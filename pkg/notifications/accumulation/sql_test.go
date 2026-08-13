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
	"path/filepath"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *SQLStore {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "accumulation.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	store, err := NewSQLStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestSQLStoreAddIsIdempotentPerEnvelopeID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
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

	group, err := store.Add(ctx, envelope, now)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if group.ItemCount != 1 {
		t.Fatalf("first item count = %d, want 1", group.ItemCount)
	}

	group, err = store.Add(ctx, envelope, now.Add(time.Second))
	if err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if group.ItemCount != 1 {
		t.Fatalf("duplicate item count = %d, want 1", group.ItemCount)
	}
}

func TestSQLStoreAddAccumulatesDistinctEvents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	now := time.Now()
	base := model.Envelope{
		Type:     model.TypeAccumulated,
		DedupKey: "share-1",
		Accumulation: model.AccumulationPolicy{
			WindowSeconds: 60,
			MaxItems:      100,
		},
	}

	first := base
	first.ID = "not-1"
	if _, err := store.Add(ctx, first, now); err != nil {
		t.Fatalf("first add: %v", err)
	}
	second := base
	second.ID = "not-2"
	group, err := store.Add(ctx, second, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if group.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", group.ItemCount)
	}

	events, ids, err := store.PendingItems(ctx, base.DedupKey)
	if err != nil {
		t.Fatalf("pending items: %v", err)
	}
	if len(events) != 2 || len(ids) != 2 {
		t.Fatalf("pending = %d events / %d ids, want 2 each", len(events), len(ids))
	}
}

func TestSQLStoreKeepsRecipientsAcrossStorage(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	now := time.Now()
	envelope := model.Envelope{
		ID:       "not-1",
		Type:     model.TypeAccumulated,
		DedupKey: "share-1",
		Recipients: []*userpb.User{
			{
				Id:       &userpb.UserId{Idp: "cernbox.cern.ch", OpaqueId: "bob", Type: userpb.UserType_USER_TYPE_PRIMARY},
				Username: "bob",
				Mail:     "bob@example.org",
			},
			{Mail: "extra@example.org"},
		},
		Accumulation: model.AccumulationPolicy{
			WindowSeconds: 60,
			MaxItems:      100,
		},
	}

	if _, err := store.Add(ctx, envelope, now); err != nil {
		t.Fatalf("add: %v", err)
	}

	events, _, err := store.PendingItems(ctx, envelope.DedupKey)
	if err != nil {
		t.Fatalf("pending items: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("pending events = %d, want 1", len(events))
	}

	// The email handler reads the address off the recipient when the
	// accumulation flushes, so the recipients have to survive storage.
	got := events[0].Recipients
	if len(got) != len(envelope.Recipients) {
		t.Fatalf("recipients = %v, want %v", got, envelope.Recipients)
	}
	for i, want := range envelope.Recipients {
		if !proto.Equal(got[i], want) {
			t.Fatalf("recipient %d = %v, want %v", i, got[i], want)
		}
	}
}

func TestSQLStoreListsExpiredLeaseCandidate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
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
		t.Fatalf("candidates = %+v, want the expired group", candidates)
	}
}
