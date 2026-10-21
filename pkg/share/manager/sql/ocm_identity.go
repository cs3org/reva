// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package sql

// Writers lock the grant identity first, then the received rows by ID. On
// SQLite every transaction takes the write lock when it begins (BEGIN IMMEDIATE).

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/go-sql-driver/mysql"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// maxTransactionAttempts bounds the retries of a transaction that failed on
// a retryable lock conflict.
const maxTransactionAttempts = 4

// recoveryTimeout bounds the read that follows an unacknowledged commit.
const recoveryTimeout = 10 * time.Second

// retryable reports lock conflicts that roll the transaction back without
// changing anything, so running it again is safe.
func retryable(err error) bool {
	var my *mysql.MySQLError
	if errors.As(err, &my) {
		return my.Number == 1213 || my.Number == 1205 // deadlock, lock wait timeout
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked")
}

// commitCheck is what an attempt expects to be durable, captured before its
// commit, with the identifiers logged when the commit is not acknowledged.
// The transaction function rebuilds it on every attempt.
type commitCheck struct {
	shareID    uint
	grantKey   string
	generation uint64
	// observe reads, outside the failed transaction, which projection is durable.
	observe func(db *gorm.DB) (share.CommitObservation, error)
}

// transact runs fn in one transaction. fn must rebuild any result and check
// it returns on every attempt. A failure before the commit rolled the
// transaction back, so a retryable one runs it again. A failed commit is
// never run again: it returns a *share.CommitUnknownError.
func (m *mgr) transact(ctx context.Context, op string, fn func(tx *gorm.DB) error, check *commitCheck) error {
	for attempt := 1; ; attempt++ {
		committing, err := m.attempt(ctx, fn)
		if committing {
			return m.reconcileCommit(ctx, op, check)
		}
		if err == nil || !retryable(err) || attempt == maxTransactionAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt) * 10 * time.Millisecond):
		}
	}
}

// attempt runs fn once. committing is true when the commit itself failed.
func (m *mgr) attempt(ctx context.Context, fn func(tx *gorm.DB) error) (committing bool, err error) {
	tx := m.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	done := false
	defer func() {
		if !done {
			tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return false, err
	}
	done = true
	if err := tx.Commit().Error; err != nil {
		return true, err
	}
	return false, nil
}

// receivedProjection is the durable state of a received share: its row and
// protocols, without the bookkeeping times that gorm writes.
type receivedProjection struct {
	row       model.OcmReceivedShare
	protocols []model.OcmReceivedShareProtocol
}

// readReceivedProjection returns nil for an absent share.
func readReceivedProjection(tx *gorm.DB, id uint) (*receivedProjection, error) {
	var p receivedProjection
	err := tx.Where("id = ?", id).Take(&p.row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Where("ocm_received_share_id = ?", id).Order("id").Find(&p.protocols).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *receivedProjection) equal(q *receivedProjection) bool {
	if p == nil || q == nil {
		return p == q
	}
	a, b := p.row, q.row
	if a.ID != b.ID || !equalGrantKey(a.GrantKey, b.GrantKey) || a.RemoteShareID != b.RemoteShareID || a.Name != b.Name ||
		a.ItemType != b.ItemType || a.ShareWith != b.ShareWith || a.Owner != b.Owner || a.Initiator != b.Initiator ||
		a.Ctime != b.Ctime || a.Mtime != b.Mtime || !equalNullTime(a.Expiration, b.Expiration) ||
		a.RecipientType != b.RecipientType || a.State != b.State || a.Alias != b.Alias || a.Hidden != b.Hidden ||
		a.TransferGeneration != b.TransferGeneration || a.DeletedAt.Valid != b.DeletedAt.Valid ||
		len(p.protocols) != len(q.protocols) {
		return false
	}
	for i := range p.protocols {
		x, y := p.protocols[i], q.protocols[i]
		if x.ID != y.ID || x.Type != y.Type || x.Uri != y.Uri || x.SharedSecret != y.SharedSecret ||
			x.Permissions != y.Permissions || x.AccessTypes != y.AccessTypes || !bytes.Equal(x.Payload, y.Payload) ||
			!bytes.Equal(x.Requirements, y.Requirements) || !bytes.Equal(x.Targets, y.Targets) ||
			x.AppName != y.AppName || x.AppIconHint != y.AppIconHint || !bytes.Equal(x.MediaTypes, y.MediaTypes) {
			return false
		}
	}
	return true
}

func equalGrantKey(a, b *string) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func equalNullTime(a, b datatypes.NullTime) bool {
	return a.Valid == b.Valid && (!a.Valid || a.V.Equal(b.V))
}

// observeReceived reads one received share on a fresh snapshot and compares
// it with the projections captured by the failed attempt. A nil expected
// projection means the attempt changed nothing; a nil previous one means
// the share did not exist.
func observeReceived(db *gorm.DB, id uint, expected, previous *receivedProjection) (share.CommitObservation, error) {
	var got *receivedProjection
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		got, err = readReceivedProjection(tx, id)
		return err
	}); err != nil {
		return "", err
	}
	switch {
	case expected != nil && got.equal(expected):
		return share.CommitObservedRequested, nil
	case got.equal(previous):
		return share.CommitObservedPrevious, nil
	}
	return share.CommitObservedOther, nil
}

func (m *mgr) reconcileCommit(ctx context.Context, op string, check *commitCheck) error {
	observed := share.CommitObservedUnread
	if check != nil && check.observe != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout)
		defer cancel()
		if o, err := check.observe(m.db.WithContext(rctx)); err == nil {
			observed = o
		}
	}
	e := appctx.GetLogger(ctx).Error().Str("operation", op).Str("observed", string(observed))
	if check != nil {
		if check.shareID != 0 {
			e = e.Uint("share_id", check.shareID)
		}
		if check.grantKey != "" {
			e = e.Str("grant_key", check.grantKey)
		}
		if check.generation != 0 {
			e = e.Uint64("transfer_generation", check.generation)
		}
	}
	e.Msg("OCM share commit was not acknowledged; no receipt is returned")
	return &share.CommitUnknownError{Observed: observed}
}

// grantOrigin returns the canonical origin of a received share: the host of
// its original sender, never of its owner. The raw address is validated
// before any normalization could hide a scheme or another invalid part.
func grantOrigin(creator string) (string, bool) {
	_, host, err := providerdomain.ParseOCMAddress(creator)
	if err != nil {
		return "", false
	}
	origin, err := share.CanonicalGrantOrigin(host)
	if err != nil {
		return "", false
	}
	return origin, true
}

// lockIdentityByKey locks an existing identity, or returns nil if absent.
// The stored fields must match the key's inputs when they are known.
func lockIdentityByKey(tx *gorm.DB, key, origin, remoteShareID string) (*model.OcmGrantIdentity, error) {
	var ident model.OcmGrantIdentity
	err := forUpdate(tx).Where("grant_key = ?", key).Take(&ident).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if origin != "" && (ident.Origin != origin || ident.RemoteShareID != remoteShareID) {
		return nil, share.ErrGrantIdentityMismatch
	}
	if want, err := share.GrantKey(ident.Origin, ident.RemoteShareID); err != nil || want != ident.GrantKey {
		return nil, share.ErrGrantIdentityMismatch
	}
	return &ident, nil
}

// ensureIdentity inserts the identity if absent and locks it.
func ensureIdentity(tx *gorm.DB, origin, remoteShareID string) (*model.OcmGrantIdentity, error) {
	key, err := share.GrantKey(origin, remoteShareID)
	if err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.OcmGrantIdentity{
		GrantKey:      key,
		Origin:        origin,
		RemoteShareID: remoteShareID,
	}).Error; err != nil {
		return nil, err
	}
	ident, err := lockIdentityByKey(tx, key, origin, remoteShareID)
	if err != nil {
		return nil, err
	}
	if ident == nil {
		return nil, share.ErrGrantIdentityMismatch
	}
	return ident, nil
}

// lockGuardedReceived locks and verifies the identity of a received share.
// Every stored share is bound, so a share without a binding fails closed.
func lockGuardedReceived(tx *gorm.DB, row *model.OcmReceivedShare) (*model.OcmGrantIdentity, error) {
	if row.GrantKey == nil || *row.GrantKey == "" {
		return nil, share.ErrGrantIdentityMismatch
	}
	origin, ok := grantOrigin(row.Initiator)
	if !ok {
		return nil, share.ErrGrantIdentityMismatch
	}
	ident, err := lockIdentityByKey(tx, *row.GrantKey, origin, row.RemoteShareID)
	if err != nil {
		return nil, err
	}
	if ident == nil {
		return nil, share.ErrGrantIdentityMismatch
	}
	return ident, nil
}

func withdrawn(ident *model.OcmGrantIdentity) bool {
	return ident != nil && ident.WithdrawnAt.Valid
}

// WithdrawReceivedGrant marks a provider grant as withdrawn and retains all
// its bound received shares as INVALID and hidden, in one transaction.
func (m *mgr) WithdrawReceivedGrant(ctx context.Context, w *share.GrantWithdrawal) (*share.GrantWithdrawalReceipt, error) {
	if w == nil {
		return nil, share.ErrInvalidGrantIdentity
	}
	origin, err := share.CanonicalGrantOrigin(w.Origin)
	if err != nil {
		return nil, err
	}
	key, err := share.GrantKey(origin, w.RemoteShareID)
	if err != nil {
		return nil, err
	}
	itemType := convertFromCS3ResourceType(w.ResourceType)

	var receipt share.GrantWithdrawalReceipt
	var check commitCheck
	err = m.transact(ctx, "withdraw_grant", func(tx *gorm.DB) error {
		receipt = share.GrantWithdrawalReceipt{}
		check = commitCheck{grantKey: key}
		var expected, previous *grantProjection
		check.observe = func(db *gorm.DB) (share.CommitObservation, error) {
			return observeGrant(db, key, expected, previous)
		}
		ident, err := lockIdentityByKey(tx, key, origin, w.RemoteShareID)
		if err != nil || ident == nil {
			// An unknown grant is not recorded: there is nothing to withdraw.
			return err
		}
		var rows []model.OcmReceivedShare
		if err := forUpdate(tx).Where("grant_key = ?", key).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if previous, err = readGrantProjection(tx, key); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			if r.ItemType != itemType {
				return nil
			}
			if w.RecipientType != ocm.RecipientType_RECIPIENT_TYPE_INVALID && r.RecipientType != convertFromCS3OCMShareType(w.RecipientType) {
				return nil
			}
		}
		if !ident.WithdrawnAt.Valid {
			now := time.Now()
			if err := tx.Model(&model.OcmGrantIdentity{}).Where("grant_key = ?", key).
				Update("withdrawn_at", datatypes.NullTime{V: now, Valid: true}).Error; err != nil {
				return err
			}
			ids := make([]uint, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			if err := tx.Model(&model.OcmReceivedShare{}).Where("id IN ?", ids).Updates(map[string]any{
				"state":  model.OcmShareStateInvalid,
				"hidden": true,
				"mtime":  now.Unix(),
			}).Error; err != nil {
				return err
			}
			receipt.Changed = true
			if expected, err = readGrantProjection(tx, key); err != nil {
				return err
			}
		}
		for _, r := range rows {
			after, err := reloadReceived(tx, r.ID)
			if err != nil {
				return err
			}
			receipt.Shares = append(receipt.Shares, after)
		}
		return nil
	}, &check)
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// grantProjection is the durable state of a grant: its ledger record and
// every bound received share.
type grantProjection struct {
	ident  model.OcmGrantIdentity
	shares []*receivedProjection
}

// readGrantProjection returns nil for an unknown grant.
func readGrantProjection(tx *gorm.DB, key string) (*grantProjection, error) {
	var g grantProjection
	err := tx.Where("grant_key = ?", key).Take(&g.ident).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []uint
	if err := tx.Model(&model.OcmReceivedShare{}).Where("grant_key = ?", key).Order("id").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		p, err := readReceivedProjection(tx, id)
		if err != nil {
			return nil, err
		}
		g.shares = append(g.shares, p)
	}
	return &g, nil
}

func (g *grantProjection) equal(h *grantProjection) bool {
	if g == nil || h == nil {
		return g == h
	}
	if g.ident.GrantKey != h.ident.GrantKey || g.ident.Origin != h.ident.Origin || g.ident.RemoteShareID != h.ident.RemoteShareID ||
		!equalNullTime(g.ident.WithdrawnAt, h.ident.WithdrawnAt) || len(g.shares) != len(h.shares) {
		return false
	}
	for i := range g.shares {
		if !g.shares[i].equal(h.shares[i]) {
			return false
		}
	}
	return true
}

// observeGrant reads a grant on a fresh snapshot and compares it with the
// projections captured by the failed attempt, like observeReceived.
func observeGrant(db *gorm.DB, key string, expected, previous *grantProjection) (share.CommitObservation, error) {
	var got *grantProjection
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		got, err = readGrantProjection(tx, key)
		return err
	}); err != nil {
		return "", err
	}
	switch {
	case expected != nil && got.equal(expected):
		return share.CommitObservedRequested, nil
	case got.equal(previous):
		return share.CommitObservedPrevious, nil
	}
	return share.CommitObservedOther, nil
}
