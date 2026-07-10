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

package notifications

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
	bucketStatusOpen     = "open"
	bucketStatusFlushing = "flushing"
	itemStatusPending    = "pending"
	itemStatusFlushed    = "flushed"
	defaultMaxItems      = 100
)

type accumulationBucket struct {
	DedupKey      string `gorm:"primaryKey;size:512"`
	FirstSeen     time.Time
	LatestSeen    time.Time
	FlushAfter    time.Time `gorm:"index"`
	WindowSeconds int
	MaxItems      int
	ItemCount     int
	Status        string     `gorm:"size:32;index"`
	LeaseOwner    *string    `gorm:"size:255;index"`
	LeaseUntil    *time.Time `gorm:"index"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (accumulationBucket) TableName() string {
	return "notification_accumulation_buckets"
}

type accumulationItem struct {
	ID         uint           `gorm:"primaryKey"`
	ItemID     string         `gorm:"size:64;uniqueIndex"`
	DedupKey   string         `gorm:"size:512;index"`
	Status     string         `gorm:"size:32;index"`
	ReceivedAt time.Time      `gorm:"index"`
	Envelope   datatypes.JSON `gorm:"type:json"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (accumulationItem) TableName() string {
	return "notification_accumulation_items"
}

// GORMStore implements AccumulationStore using a SQL database through GORM.
type GORMStore struct {
	db *gorm.DB
}

// NewGORMStore migrates the accumulator schema and returns a store.
func NewGORMStore(db *gorm.DB) (*GORMStore, error) {
	if db == nil {
		return nil, errors.New("gorm db is required")
	}
	if err := db.AutoMigrate(&accumulationBucket{}, &accumulationItem{}); err != nil {
		return nil, errors.Wrap(err, "notifications: migrating accumulator schema failed")
	}
	return &GORMStore{db: db}, nil
}

// Add persists one accumulated notification item and updates its bucket.
func (s *GORMStore) Add(ctx context.Context, envelope model.Envelope, now time.Time) (*model.Bucket, error) {
	windowSeconds := envelope.Accumulation.WindowSeconds
	maxItems := envelope.Accumulation.MaxItems
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}

	var out accumulationBucket
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item := accumulationItem{
			ItemID:     envelope.ID,
			DedupKey:   envelope.DedupKey,
			Status:     itemStatusPending,
			ReceivedAt: now,
			Envelope:   datatypes.JSON(data),
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return tx.First(&out, "dedup_key = ?", envelope.DedupKey).Error
		}

		bucket := accumulationBucket{
			DedupKey:      envelope.DedupKey,
			FirstSeen:     now,
			LatestSeen:    now,
			FlushAfter:    now.Add(time.Duration(windowSeconds) * time.Second),
			WindowSeconds: windowSeconds,
			MaxItems:      maxItems,
			ItemCount:     1,
			Status:        bucketStatusOpen,
		}

		res = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "dedup_key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"latest_seen": now,
				"item_count":  gorm.Expr("item_count + ?", 1),
			}),
		}).Create(&bucket)
		if res.Error != nil {
			return res.Error
		}

		return tx.First(&out, "dedup_key = ?", envelope.DedupKey).Error
	})
	if err != nil {
		return nil, errors.Wrap(err, "notifications: adding accumulated item failed")
	}

	return bucketFromModel(out), nil
}

// AcquireLease attempts to acquire or refresh a bucket lease.
func (s *GORMStore) AcquireLease(ctx context.Context, dedupKey, owner string, leaseUntil, now time.Time) (bool, error) {
	res := s.db.WithContext(ctx).Model(&accumulationBucket{}).
		Where("dedup_key = ? AND status = ? AND (lease_until IS NULL OR lease_until < ? OR lease_owner = ?)", dedupKey, bucketStatusOpen, now, owner).
		Updates(map[string]any{
			"lease_owner": owner,
			"lease_until": leaseUntil,
		})
	if res.Error != nil {
		return false, errors.Wrap(res.Error, "notifications: acquiring accumulator lease failed")
	}
	return res.RowsAffected == 1, nil
}

// LockDueForFlush transitions a due bucket owned by this worker to flushing.
func (s *GORMStore) LockDueForFlush(ctx context.Context, dedupKey, owner string, now time.Time) (bool, error) {
	res := s.db.WithContext(ctx).Model(&accumulationBucket{}).
		Where("dedup_key = ? AND status = ? AND lease_owner = ? AND (flush_after <= ? OR item_count >= max_items)", dedupKey, bucketStatusOpen, owner, now).
		Update("status", bucketStatusFlushing)
	if res.Error != nil {
		return false, errors.Wrap(res.Error, "notifications: locking due accumulator bucket failed")
	}
	return res.RowsAffected == 1, nil
}

// PendingItems returns pending envelopes for a dedup key in receive order.
func (s *GORMStore) PendingItems(ctx context.Context, dedupKey string) ([]model.Envelope, []string, error) {
	var rows []accumulationItem
	if err := s.db.WithContext(ctx).
		Where("dedup_key = ? AND status = ?", dedupKey, itemStatusPending).
		Order("received_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, nil, errors.Wrap(err, "notifications: listing pending accumulator items failed")
	}

	envelopes := make([]model.Envelope, 0, len(rows))
	itemIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		var envelope model.Envelope
		if err := json.Unmarshal(row.Envelope, &envelope); err != nil {
			return nil, nil, err
		}
		envelopes = append(envelopes, envelope)
		itemIDs = append(itemIDs, row.ItemID)
	}
	return envelopes, itemIDs, nil
}

// MarkFlushed marks the flushed items and either removes the empty bucket or
// reopens it for items that arrived while the previous batch was flushing.
func (s *GORMStore) MarkFlushed(ctx context.Context, dedupKey string, itemIDs []string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(itemIDs) > 0 {
			if err := tx.Model(&accumulationItem{}).
				Where("dedup_key = ? AND item_id IN ?", dedupKey, itemIDs).
				Update("status", itemStatusFlushed).Error; err != nil {
				return err
			}
		}

		var bucket accumulationBucket
		if err := tx.First(&bucket, "dedup_key = ?", dedupKey).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		var pending []accumulationItem
		if err := tx.Where("dedup_key = ? AND status = ?", dedupKey, itemStatusPending).
			Order("received_at ASC, id ASC").
			Find(&pending).Error; err != nil {
			return err
		}

		if len(pending) == 0 {
			if err := tx.Where("dedup_key = ?", dedupKey).Delete(&accumulationBucket{}).Error; err != nil {
				return err
			}
			return tx.Where("dedup_key = ? AND status = ?", dedupKey, itemStatusFlushed).Delete(&accumulationItem{}).Error
		}

		firstSeen := pending[0].ReceivedAt
		latestSeen := pending[len(pending)-1].ReceivedAt
		flushAfter := firstSeen.Add(time.Duration(bucket.WindowSeconds) * time.Second)

		return tx.Model(&accumulationBucket{}).
			Where("dedup_key = ?", dedupKey).
			Updates(map[string]any{
				"first_seen":  firstSeen,
				"latest_seen": latestSeen,
				"flush_after": flushAfter,
				"item_count":  len(pending),
				"status":      bucketStatusOpen,
				"lease_owner": nil,
				"lease_until": nil,
			}).Error
	})
}

// ReleaseLease releases a lease held by owner and reopens the bucket.
func (s *GORMStore) ReleaseLease(ctx context.Context, dedupKey, owner string) error {
	res := s.db.WithContext(ctx).Model(&accumulationBucket{}).
		Where("dedup_key = ? AND lease_owner = ?", dedupKey, owner).
		Updates(map[string]any{
			"status":      bucketStatusOpen,
			"lease_owner": nil,
			"lease_until": nil,
		})
	return errors.Wrap(res.Error, "notifications: releasing accumulator lease failed")
}

// ListCandidates lists buckets that might need a worker to own or recover them.
func (s *GORMStore) ListCandidates(ctx context.Context, now time.Time, limit int) ([]*model.Bucket, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows []accumulationBucket
	if err := s.db.WithContext(ctx).
		Where("status = ? AND (flush_after <= ? OR item_count >= max_items OR lease_until IS NULL OR lease_until < ?)", bucketStatusOpen, now, now).
		Order("flush_after ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "notifications: listing accumulator candidates failed")
	}

	buckets := make([]*model.Bucket, 0, len(rows))
	for _, row := range rows {
		buckets = append(buckets, bucketFromModel(row))
	}
	return buckets, nil
}

func bucketFromModel(row accumulationBucket) *model.Bucket {
	bucket := &model.Bucket{
		DedupKey:      row.DedupKey,
		FirstSeen:     row.FirstSeen,
		LatestSeen:    row.LatestSeen,
		FlushAfter:    row.FlushAfter,
		WindowSeconds: row.WindowSeconds,
		MaxItems:      row.MaxItems,
		ItemCount:     row.ItemCount,
		Status:        row.Status,
	}
	if row.LeaseOwner != nil {
		bucket.LeaseOwner = *row.LeaseOwner
	}
	if row.LeaseUntil != nil {
		bucket.LeaseUntil = *row.LeaseUntil
	}
	return bucket
}
