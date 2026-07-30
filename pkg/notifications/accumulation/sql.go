// Copyright 2018-2026 CERN
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

package accumulation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/pkg/errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	statusOpen      = "open"
	statusFlushing  = "flushing"
	statusPending   = "pending"
	statusFlushed   = "flushed"
	defaultMaxItems = 100
)

// PendingEvent is one accepted envelope awaiting accumulation. One event is one
// row. It is grouped with the other events sharing its dedup key by the Group
// that owns them.
type PendingEvent struct {
	gorm.Model
	EventID  string `gorm:"uniqueIndex;size:64"`
	DedupKey string `gorm:"index;size:512"`
	Status   string `gorm:"size:32;index"`
	Envelope datatypes.JSON
}

// Group is the per dedup-key coordination state for a set of pending events:
// when it is due to flush, how many events it holds, and which box holds the
// lease. Exactly one box owns the lease at a time, so exactly one box flushes
// the group even when its events were consumed by different boxes.
type Group struct {
	gorm.Model
	DedupKey      string `gorm:"uniqueIndex;size:512"`
	FirstSeen     time.Time
	LatestSeen    time.Time
	FlushAfter    time.Time `gorm:"index"`
	WindowSeconds int
	MaxItems      int
	ItemCount     int
	Status        string     `gorm:"size:32;index"`
	LeaseOwner    *string    `gorm:"size:255;index"`
	LeaseUntil    *time.Time `gorm:"index"`
}

// SQLStore is a Store backed by a SQL database through GORM.
type SQLStore struct {
	db *gorm.DB
}

var _ Store = (*SQLStore)(nil)

// NewSQLStore migrates the schema and returns a store.
func NewSQLStore(db *gorm.DB) (*SQLStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if err := db.AutoMigrate(&Group{}, &PendingEvent{}); err != nil {
		return nil, errors.Wrap(err, "accumulation: migrating schema failed")
	}
	return &SQLStore{db: db}, nil
}

// Add stores one accepted event and updates its group. It is idempotent per
// event ID, so a redelivered event does not inflate the count.
func (s *SQLStore) Add(ctx context.Context, envelope model.Envelope, now time.Time) (*Group, error) {
	maxItems := envelope.Accumulation.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}

	var group Group
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		event := PendingEvent{
			EventID:  envelope.ID,
			DedupKey: envelope.DedupKey,
			Status:   statusPending,
			Envelope: datatypes.JSON(data),
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Redelivered event: leave the group untouched.
			return tx.First(&group, "dedup_key = ?", envelope.DedupKey).Error
		}

		created := Group{
			DedupKey:      envelope.DedupKey,
			FirstSeen:     now,
			LatestSeen:    now,
			FlushAfter:    now.Add(time.Duration(envelope.Accumulation.WindowSeconds) * time.Second),
			WindowSeconds: envelope.Accumulation.WindowSeconds,
			MaxItems:      maxItems,
			ItemCount:     1,
			Status:        statusOpen,
		}
		res = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "dedup_key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"latest_seen": now,
				"item_count":  gorm.Expr("item_count + ?", 1),
			}),
		}).Create(&created)
		if res.Error != nil {
			return res.Error
		}

		return tx.First(&group, "dedup_key = ?", envelope.DedupKey).Error
	})
	if err != nil {
		return nil, errors.Wrap(err, "accumulation: adding event failed")
	}
	return &group, nil
}

// AcquireLease acquires or refreshes the flush lease on an open group.
func (s *SQLStore) AcquireLease(ctx context.Context, dedupKey, owner string, leaseUntil, now time.Time) (bool, error) {
	res := s.db.WithContext(ctx).Model(&Group{}).
		Where("dedup_key = ?", dedupKey).
		Where("status = ?", statusOpen).
		Where(s.db.Where("lease_until IS NULL").Or("lease_until < ?", now).Or("lease_owner = ?", owner)).
		Updates(map[string]any{"lease_owner": owner, "lease_until": leaseUntil})
	if res.Error != nil {
		return false, errors.Wrap(res.Error, "accumulation: acquiring lease failed")
	}
	return res.RowsAffected == 1, nil
}

// LockDueForFlush transitions a due group owned by this box to flushing.
func (s *SQLStore) LockDueForFlush(ctx context.Context, dedupKey, owner string, now time.Time) (bool, error) {
	res := s.db.WithContext(ctx).Model(&Group{}).
		Where("dedup_key = ?", dedupKey).
		Where("status = ?", statusOpen).
		Where("lease_owner = ?", owner).
		Where(s.db.Where("flush_after <= ?", now).Or("item_count >= max_items")).
		Update("status", statusFlushing)
	if res.Error != nil {
		return false, errors.Wrap(res.Error, "accumulation: locking due group failed")
	}
	return res.RowsAffected == 1, nil
}

// PendingItems returns the pending events for a dedup key in receive order.
func (s *SQLStore) PendingItems(ctx context.Context, dedupKey string) ([]model.Envelope, []string, error) {
	var rows []PendingEvent
	if err := s.db.WithContext(ctx).
		Where("dedup_key = ?", dedupKey).
		Where("status = ?", statusPending).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, nil, errors.Wrap(err, "accumulation: listing pending events failed")
	}

	envelopes := make([]model.Envelope, 0, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		var envelope model.Envelope
		if err := json.Unmarshal(row.Envelope, &envelope); err != nil {
			return nil, nil, errors.Wrap(err, "accumulation: decoding event failed")
		}
		envelopes = append(envelopes, envelope)
		ids = append(ids, row.EventID)
	}
	return envelopes, ids, nil
}

// MarkFlushed marks the flushed events and either removes the empty group or
// reopens it for events that arrived while the batch was flushing.
func (s *SQLStore) MarkFlushed(ctx context.Context, dedupKey string, eventIDs []string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(eventIDs) > 0 {
			if err := tx.Model(&PendingEvent{}).
				Where("dedup_key = ?", dedupKey).
				Where("event_id IN ?", eventIDs).
				Update("status", statusFlushed).Error; err != nil {
				return err
			}
		}

		var group Group
		if err := tx.First(&group, "dedup_key = ?", dedupKey).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var pending []PendingEvent
		if err := tx.Where("dedup_key = ?", dedupKey).
			Where("status = ?", statusPending).
			Order("created_at ASC, id ASC").
			Find(&pending).Error; err != nil {
			return err
		}

		if len(pending) == 0 {
			if err := tx.Unscoped().Where("dedup_key = ?", dedupKey).Delete(&Group{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().
				Where("dedup_key = ?", dedupKey).
				Where("status = ?", statusFlushed).
				Delete(&PendingEvent{}).Error
		}

		firstSeen := pending[0].CreatedAt
		latestSeen := pending[len(pending)-1].CreatedAt
		return tx.Model(&Group{}).
			Where("dedup_key = ?", dedupKey).
			Updates(map[string]any{
				"first_seen":  firstSeen,
				"latest_seen": latestSeen,
				"flush_after": firstSeen.Add(time.Duration(group.WindowSeconds) * time.Second),
				"item_count":  len(pending),
				"status":      statusOpen,
				"lease_owner": nil,
				"lease_until": nil,
			}).Error
	})
}

// ReleaseLease releases a lease held by owner and reopens the group.
func (s *SQLStore) ReleaseLease(ctx context.Context, dedupKey, owner string) error {
	res := s.db.WithContext(ctx).Model(&Group{}).
		Where("dedup_key = ?", dedupKey).
		Where("lease_owner = ?", owner).
		Updates(map[string]any{"status": statusOpen, "lease_owner": nil, "lease_until": nil})
	return errors.Wrap(res.Error, "accumulation: releasing lease failed")
}

// ListCandidates lists groups that a box should try to own and flush: due,
// unleased, or with an expired lease.
func (s *SQLStore) ListCandidates(ctx context.Context, now time.Time, limit int) ([]*Group, error) {
	if limit <= 0 {
		limit = 100
	}

	var groups []*Group
	if err := s.db.WithContext(ctx).
		Where("status = ?", statusOpen).
		Where(s.db.
			Where("flush_after <= ?", now).
			Or("item_count >= max_items").
			Or("lease_until IS NULL").
			Or("lease_until < ?", now)).
		Order("flush_after ASC").
		Limit(limit).
		Find(&groups).Error; err != nil {
		return nil, errors.Wrap(err, "accumulation: listing candidates failed")
	}
	return groups, nil
}
