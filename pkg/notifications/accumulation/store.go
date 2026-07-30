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

// Package accumulation stores accepted notification events that share a
// deduplication key and coordinates, through an expiring lease, which box
// flushes them. This keeps accumulation correct in a stateless HA deployment
// where any box may consume any event.
package accumulation

import (
	"context"
	"time"

	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

// Store persists accumulated events and coordinates the flush lease across
// boxes.
type Store interface {
	// Add stores one accepted event and returns the state of its group.
	Add(ctx context.Context, envelope model.Envelope, now time.Time) (*Group, error)
	// AcquireLease acquires or refreshes the flush lease on an open group.
	AcquireLease(ctx context.Context, dedupKey, owner string, leaseUntil, now time.Time) (bool, error)
	// LockDueForFlush transitions a due group owned by this box to flushing.
	LockDueForFlush(ctx context.Context, dedupKey, owner string, now time.Time) (bool, error)
	// PendingItems returns a group's pending events in receive order with ids.
	PendingItems(ctx context.Context, dedupKey string) ([]model.Envelope, []string, error)
	// MarkFlushed marks the flushed events, removing or reopening the group.
	MarkFlushed(ctx context.Context, dedupKey string, eventIDs []string) error
	// ReleaseLease releases a lease held by owner and reopens the group.
	ReleaseLease(ctx context.Context, dedupKey, owner string) error
	// ListCandidates lists groups a box should try to own and flush.
	ListCandidates(ctx context.Context, now time.Time, limit int) ([]*Group, error)
}
