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
	"context"
	"errors"
	"strings"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
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

// reconcileFunc reads, outside the failed transaction, whether the change
// of an unacknowledged commit is durable.
type reconcileFunc func(db *gorm.DB) (share.CommitObservation, error)

// transact runs fn in one transaction. fn must rebuild any result it returns
// on every attempt. A failure before the commit rolled the transaction back,
// so a retryable one runs it again. A failed commit is never run again: it
// returns a *share.CommitUnknownError after reconcile, if any.
func (m *mgr) transact(ctx context.Context, op string, fn func(tx *gorm.DB) error, reconcile reconcileFunc) error {
	for attempt := 1; ; attempt++ {
		committing, err := m.attempt(ctx, fn)
		if committing {
			return m.reconcileCommit(ctx, op, reconcile)
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

// observeReceived reads a received share after an unacknowledged commit and
// compares it with the requested and the previous values.
func observeReceived(db *gorm.DB, id uint, requested, previous func(model.OcmReceivedShare) bool) (share.CommitObservation, error) {
	var row model.OcmReceivedShare
	if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
		return "", err
	}
	switch {
	case requested(row):
		return share.CommitObservedRequested, nil
	case previous(row):
		return share.CommitObservedPrevious, nil
	}
	return share.CommitObservedOther, nil
}

func (m *mgr) reconcileCommit(ctx context.Context, op string, reconcile reconcileFunc) error {
	observed := share.CommitObservedUnread
	if reconcile != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout)
		defer cancel()
		if o, err := reconcile(m.db.WithContext(rctx)); err == nil {
			observed = o
		}
	}
	appctx.GetLogger(ctx).Error().Str("operation", op).Str("observed", string(observed)).
		Msg("OCM share commit was not acknowledged; no receipt is returned")
	return &share.CommitUnknownError{Observed: observed}
}

// grantOrigin returns the canonical origin of a received share: the domain
// of its original sender, never of its owner.
func grantOrigin(creator string) (string, bool) {
	u, err := ocmd.GetUserIdFromOCMAddress(creator)
	if err != nil {
		return "", false
	}
	origin, err := share.CanonicalGrantOrigin(u.Idp)
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
// It returns nil for an unbound share.
func lockGuardedReceived(tx *gorm.DB, row *model.OcmReceivedShare) (*model.OcmGrantIdentity, error) {
	if row.GrantKey == nil {
		return nil, nil
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
	err = m.transact(ctx, "withdraw_grant", func(tx *gorm.DB) error {
		receipt = share.GrantWithdrawalReceipt{}
		ident, err := lockIdentityByKey(tx, key, origin, w.RemoteShareID)
		if err != nil || ident == nil {
			// An unknown grant is not recorded: there is nothing to withdraw.
			return err
		}
		var rows []model.OcmReceivedShare
		if err := forUpdate(tx).Where("grant_key = ?", key).Order("id").Find(&rows).Error; err != nil {
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
		}
		for _, r := range rows {
			after, err := reloadReceived(tx, r.ID)
			if err != nil {
				return err
			}
			receipt.Shares = append(receipt.Shares, after)
		}
		return nil
	}, func(db *gorm.DB) (share.CommitObservation, error) {
		var ident model.OcmGrantIdentity
		if err := db.Where("grant_key = ?", key).Take(&ident).Error; err != nil {
			return "", err
		}
		if ident.WithdrawnAt.Valid {
			return share.CommitObservedRequested, nil
		}
		return share.CommitObservedPrevious, nil
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// backfillReport counts the outcome of binding historical received shares.
type backfillReport struct {
	Bound          int
	Identities     int
	MissingOrigin  int
	ConflictingMap int
}

// backfillGrantIdentities binds historical received shares to their grant.
// Shares without a verifiable origin or with conflicting type mappings stay
// unbound and cannot be withdrawn. It is idempotent.
func (m *mgr) backfillGrantIdentities(ctx context.Context) (backfillReport, error) {
	var report backfillReport
	var rows []model.OcmReceivedShare
	if err := m.db.WithContext(ctx).Where("grant_key IS NULL").Order("id").Find(&rows).Error; err != nil {
		return report, err
	}
	type grant struct{ origin, remote string }
	groups := map[grant][]model.OcmReceivedShare{}
	var order []grant
	for _, r := range rows {
		origin, ok := grantOrigin(r.Initiator)
		if !ok || r.RemoteShareID == "" || len(r.RemoteShareID) > share.MaxGrantFieldLength {
			report.MissingOrigin++
			continue
		}
		g := grant{origin, r.RemoteShareID}
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], r)
	}
	for _, g := range order {
		members := groups[g]
		consistent := true
		for _, r := range members[1:] {
			if r.ItemType != members[0].ItemType || r.RecipientType != members[0].RecipientType {
				consistent = false
			}
		}
		if !consistent {
			report.ConflictingMap += len(members)
			continue
		}
		bound, conflict := 0, false
		err := m.transact(ctx, "bind_grant_identity", func(tx *gorm.DB) error {
			bound, conflict = 0, false
			ident, err := ensureIdentity(tx, g.origin, g.remote)
			if err != nil {
				return err
			}
			// A grant already bound with another type mapping is a conflict.
			var other int64
			if err := tx.Model(&model.OcmReceivedShare{}).
				Where("grant_key = ? AND (item_type <> ? OR recipient_type <> ?)", ident.GrantKey, members[0].ItemType, members[0].RecipientType).
				Count(&other).Error; err != nil {
				return err
			}
			if other > 0 {
				conflict = true
				return nil
			}
			ids := make([]uint, 0, len(members))
			for _, r := range members {
				ids = append(ids, r.ID)
			}
			updates := map[string]any{"grant_key": ident.GrantKey}
			if withdrawn(ident) {
				// Rows written for an already withdrawn grant join its
				// retained terminal projection.
				updates["state"] = model.OcmShareStateInvalid
				updates["hidden"] = true
			}
			res := tx.Model(&model.OcmReceivedShare{}).Where("id IN ? AND grant_key IS NULL", ids).Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			bound = int(res.RowsAffected)
			return nil
		}, nil)
		if err != nil {
			return report, err
		}
		if conflict {
			report.ConflictingMap += len(members)
			continue
		}
		if bound == 0 {
			// Another instance bound these rows meanwhile.
			continue
		}
		report.Identities++
		report.Bound += bound
	}
	if report.MissingOrigin > 0 || report.ConflictingMap > 0 {
		appctx.GetLogger(ctx).Warn().
			Int("missing_origin", report.MissingOrigin).
			Int("conflicting_mapping", report.ConflictingMap).
			Msg("received OCM shares left without grant identity; they cannot be withdrawn by notification")
	}
	return report, nil
}
