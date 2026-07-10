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
	"path/filepath"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestGORMStore(t *testing.T) *GORMStore {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notifications.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	store, err := NewGORMStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestGORMStoreAddIsIdempotentPerEnvelopeID(t *testing.T) {
	ctx := context.Background()
	store := newTestGORMStore(t)
	now := time.Now()
	envelope := model.Envelope{
		ID:       "not-1",
		Type:     model.TypeAccumulated,
		DedupKey: "share-1",
		Accumulation: model.AccumulationPolicy{
			WindowSeconds: 60,
			MaxItems:      100,
		},
	}

	bucket, err := store.Add(ctx, envelope, now)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if bucket.ItemCount != 1 {
		t.Fatalf("first item count = %d, want 1", bucket.ItemCount)
	}

	bucket, err = store.Add(ctx, envelope, now.Add(time.Second))
	if err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if bucket.ItemCount != 1 {
		t.Fatalf("duplicate item count = %d, want 1", bucket.ItemCount)
	}
}

func TestGORMStoreListsExpiredLeaseCandidate(t *testing.T) {
	ctx := context.Background()
	store := newTestGORMStore(t)
	now := time.Now()
	envelope := model.Envelope{
		ID:       "not-1",
		Type:     model.TypeAccumulated,
		DedupKey: "share-1",
		Accumulation: model.AccumulationPolicy{
			WindowSeconds: 60,
			MaxItems:      100,
		},
	}

	if _, err := store.Add(ctx, envelope, now); err != nil {
		t.Fatalf("add: %v", err)
	}
	ok, err := store.AcquireLease(ctx, envelope.DedupKey, "box-1", now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if !ok {
		t.Fatal("expected lease acquisition to succeed")
	}

	candidates, err := store.ListCandidates(ctx, now.Add(2*time.Minute), 10)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].DedupKey != envelope.DedupKey {
		t.Fatalf("candidates = %+v, want expired bucket", candidates)
	}
}
