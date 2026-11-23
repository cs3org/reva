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
	dbsql "database/sql"
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
		a.ItemType != b.ItemType || a.ShareWith != b.ShareWith || a.ShareWithIdp != b.ShareWithIdp ||
		a.ShareWithTenant != b.ShareWithTenant || a.Owner != b.Owner || a.Initiator != b.Initiator ||
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
			x.AppIconHint != y.AppIconHint || !bytes.Equal(x.Apps, y.Apps) {
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

// observeTx runs every query of one observation in a single read-only
// transaction. MySQL gets an explicit REPEATABLE READ snapshot whatever the
// session default; on SQLite the immediate transaction keeps writers out.
func observeTx(db *gorm.DB, read func(tx *gorm.DB) error) error {
	if db.Dialector.Name() == "mysql" {
		return db.Transaction(read, &dbsql.TxOptions{Isolation: dbsql.LevelRepeatableRead, ReadOnly: true})
	}
	return db.Transaction(read)
}

// observeReceived reads one received share on a fresh snapshot and compares
// it with the projections captured by the failed attempt. A nil expected
// projection means the attempt changed nothing; a nil previous one means
// the share did not exist.
func observeReceived(db *gorm.DB, id uint, expected, previous *receivedProjection) (share.CommitObservation, error) {
	var got *receivedProjection
	if err := observeTx(db, func(tx *gorm.DB) error {
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

// lockReceivedGrant locks the same projection rows as the local lifecycle
// writers. Recheck the exact stored identity independently of SQL collation.
func lockReceivedGrant(tx *gorm.DB, grant *share.GrantWithdrawal) ([]model.OcmReceivedShare, string, error) {
	if grant == nil {
		return nil, "", share.ErrInvalidGrantIdentity
	}
	origin, err := share.CanonicalGrantOrigin(grant.Origin)
	if err != nil {
		return nil, "", err
	}
	key, err := share.GrantKey(origin, grant.RemoteShareID)
	if err != nil {
		return nil, "", err
	}
	switch grant.ResourceType {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER,
		ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
	default:
		return nil, "", share.ErrInvalidGrantIdentity
	}
	recipient := convertFromCS3OCMShareType(grant.RecipientType)
	if grant.RecipientType != ocm.RecipientType_RECIPIENT_TYPE_INVALID && recipient < 0 {
		return nil, "", share.ErrInvalidGrantIdentity
	}
	var rows []model.OcmReceivedShare
	if err := forUpdate(tx).Where("grant_key = ?", key).Order("id").Find(&rows).Error; err != nil {
		return nil, "", err
	}
	for _, row := range rows {
		storedOrigin, ok := grantOrigin(row.Initiator)
		if !ok || row.GrantKey == nil || *row.GrantKey != key ||
			storedOrigin != origin || row.RemoteShareID != grant.RemoteShareID || row.ShareWith == "" {
			return nil, "", share.ErrGrantIdentityMismatch
		}
		if row.ItemType != convertFromCS3ResourceType(grant.ResourceType) ||
			(grant.RecipientType != ocm.RecipientType_RECIPIENT_TYPE_INVALID && row.RecipientType != recipient) {
			return nil, key, nil
		}
	}
	return rows, key, nil
}

// WithdrawReceivedGrant physically deletes the exact projections and their protocols.
func (m *mgr) WithdrawReceivedGrant(ctx context.Context, grant *share.GrantWithdrawal) (*share.GrantWithdrawalReceipt, error) {
	var receipt share.GrantWithdrawalReceipt
	var check commitCheck
	err := m.transact(ctx, "withdraw_grant", func(tx *gorm.DB) error {
		receipt = share.GrantWithdrawalReceipt{}
		check = commitCheck{}
		rows, key, err := lockReceivedGrant(tx, grant)
		if err != nil || len(rows) == 0 {
			return err
		}
		ids := make([]uint, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if err := tx.Unscoped().Where("ocm_received_share_id IN ?", ids).Delete(&model.OcmReceivedShareProtocol{}).Error; err != nil {
			return err
		}
		result := tx.Unscoped().Where("id IN ?", ids).Delete(&model.OcmReceivedShare{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return share.ErrShareNotFound
		}
		check = commitCheck{grantKey: key, observe: func(db *gorm.DB) (share.CommitObservation, error) {
			return observeDeleted(db, ids)
		}}
		receipt.Changed = true
		return nil
	}, &check)
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func observeDeleted(db *gorm.DB, ids []uint) (share.CommitObservation, error) {
	var rows, children int64
	err := observeTx(db, func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(&model.OcmReceivedShare{}).Where("id IN ?", ids).Count(&rows).Error; err != nil {
			return err
		}
		return tx.Unscoped().Model(&model.OcmReceivedShareProtocol{}).Where("ocm_received_share_id IN ?", ids).Count(&children).Error
	})
	if err != nil {
		return share.CommitObservedUnread, err
	}
	if rows == 0 && children == 0 {
		return share.CommitObservedRequested, nil
	}
	return share.CommitObservedOther, nil
}

// UpdateReceivedGrantPermissions replaces only existing protocol permission
// masks. Nil leaves that protocol unchanged. Mapping wire permissions is
// outside this storage primitive.
func (m *mgr) UpdateReceivedGrantPermissions(ctx context.Context, grant *share.GrantWithdrawal, webdav, webapp *model.OcmPermissions) (bool, error) {
	if webdav == nil && webapp == nil {
		return false, share.ErrInvalidGrantIdentity
	}
	var changed bool
	var check commitCheck
	err := m.transact(ctx, "update_received_grant_permissions", func(tx *gorm.DB) error {
		changed = false
		check = commitCheck{}
		rows, key, err := lockReceivedGrant(tx, grant)
		if err != nil || len(rows) == 0 {
			return err
		}
		expected := make([]*receivedProjection, 0, len(rows))
		for _, row := range rows {
			projection, err := readReceivedProjection(tx, row.ID)
			if err != nil {
				return err
			}
			if projection == nil {
				return share.ErrShareNotFound
			}
			if err := replaceReceivedPermissions(tx, projection, webdav, webapp, &changed); err != nil {
				return err
			}
			expected = append(expected, projection)
		}
		if changed {
			check = commitCheck{grantKey: key, observe: func(db *gorm.DB) (share.CommitObservation, error) {
				return observePermissionUpdate(db, expected)
			}}
		}
		return nil
	}, &check)
	return changed && err == nil, err
}

func replaceReceivedPermissions(tx *gorm.DB, p *receivedProjection, webdav, webapp *model.OcmPermissions, changed *bool) error {
	davFound, appFound := webdav == nil, webapp == nil
	for i := range p.protocols {
		protocol := &p.protocols[i]
		var want *model.OcmPermissions
		switch protocol.Type {
		case model.WebDAVProtocol:
			davFound, want = true, webdav
		case model.WebappProtocol:
			appFound, want = true, webapp
		}
		if want == nil || protocol.Permissions == *want {
			continue
		}
		result := tx.Model(protocol).Update("permissions", *want)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return share.ErrGrantIdentityMismatch
		}
		protocol.Permissions = *want
		*changed = true
	}
	if !davFound || !appFound {
		return share.ErrGrantIdentityMismatch
	}
	return nil
}

func observePermissionUpdate(db *gorm.DB, expected []*receivedProjection) (share.CommitObservation, error) {
	result := share.CommitObservedRequested
	err := observeTx(db, func(tx *gorm.DB) error {
		for _, want := range expected {
			got, err := readReceivedProjection(tx, want.row.ID)
			if err != nil {
				return err
			}
			if !got.equal(want) {
				result = share.CommitObservedOther
				break
			}
		}
		return nil
	})
	return result, err
}
