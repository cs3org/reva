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
	"errors"
	"fmt"
	"sync"
	"time"
)

const defaultMaxRenderedItems = 10

// AccumulationStore persists accumulated notifications and coordinates leases.
type AccumulationStore interface {
	Add(ctx context.Context, envelope Envelope, now time.Time) (*Bucket, error)
	AcquireLease(ctx context.Context, dedupKey, owner string, leaseUntil, now time.Time) (bool, error)
	LockDueForFlush(ctx context.Context, dedupKey, owner string, now time.Time) (bool, error)
	PendingItems(ctx context.Context, dedupKey string) ([]Envelope, []string, error)
	MarkFlushed(ctx context.Context, dedupKey string, itemIDs []string) error
	ReleaseLease(ctx context.Context, dedupKey, owner string) error
	ListCandidates(ctx context.Context, now time.Time, limit int) ([]*Bucket, error)
}

// Worker handles notification envelopes consumed from NATS.
type Worker struct {
	store      AccumulationStore
	dispatcher *Dispatcher
	ownerID    string

	leaseDuration    time.Duration
	maxRenderedItems int
	now              func() time.Time

	mu     sync.Mutex
	timers map[string]*time.Timer
}

// WorkerConfig configures a notification worker.
type WorkerConfig struct {
	OwnerID          string
	LeaseDuration    time.Duration
	MaxRenderedItems int
}

// NewWorker creates a notification worker.
func NewWorker(store AccumulationStore, dispatcher *Dispatcher, conf WorkerConfig) (*Worker, error) {
	if dispatcher == nil {
		return nil, errors.New("notification dispatcher is required")
	}
	if conf.OwnerID == "" {
		return nil, errors.New("worker owner id is required")
	}
	if conf.LeaseDuration <= 0 {
		conf.LeaseDuration = 5 * time.Minute
	}
	if conf.MaxRenderedItems <= 0 {
		conf.MaxRenderedItems = defaultMaxRenderedItems
	}

	return &Worker{
		store:            store,
		dispatcher:       dispatcher,
		ownerID:          conf.OwnerID,
		leaseDuration:    conf.LeaseDuration,
		maxRenderedItems: conf.MaxRenderedItems,
		now:              time.Now,
		timers:           make(map[string]*time.Timer),
	}, nil
}

// Handle handles one notification envelope.
func (w *Worker) Handle(ctx context.Context, envelope Envelope) error {
	switch envelope.Type {
	case TypeDirect:
		return w.dispatcher.Dispatch(ctx, envelope)
	case TypeAccumulated:
		if w.store == nil {
			return errors.New("accumulation store is required for accumulated notifications")
		}

		bucket, err := w.store.Add(ctx, envelope, w.now())
		if err != nil {
			return err
		}
		return w.resumeBucket(ctx, bucket)
	default:
		return fmt.Errorf("unsupported notification type %q", envelope.Type)
	}
}

func (w *Worker) resumeBucket(ctx context.Context, bucket *Bucket) error {
	now := w.now()
	leaseUntil := bucket.FlushAfter.Add(w.leaseDuration)
	if leaseUntil.Before(now.Add(w.leaseDuration)) {
		leaseUntil = now.Add(w.leaseDuration)
	}

	acquired, err := w.store.AcquireLease(ctx, bucket.DedupKey, w.ownerID, leaseUntil, now)
	if err != nil || !acquired {
		return err
	}

	if bucket.ItemCount >= bucket.MaxItems || !bucket.FlushAfter.After(now) {
		return w.Flush(ctx, bucket.DedupKey)
	}

	w.scheduleFlush(ctx, bucket.DedupKey, time.Until(bucket.FlushAfter))
	return nil
}

// Flush dispatches the currently pending items for a dedup key if this worker
// owns the lease and the bucket is due.
func (w *Worker) Flush(ctx context.Context, dedupKey string) error {
	now := w.now()
	locked, err := w.store.LockDueForFlush(ctx, dedupKey, w.ownerID, now)
	if err != nil || !locked {
		return err
	}

	items, itemIDs, err := w.store.PendingItems(ctx, dedupKey)
	if err != nil {
		_ = w.store.ReleaseLease(ctx, dedupKey, w.ownerID)
		return err
	}
	if len(items) == 0 {
		return w.store.MarkFlushed(ctx, dedupKey, nil)
	}

	envelope := w.accumulate(items)
	if err := w.dispatcher.Dispatch(ctx, envelope); err != nil {
		_ = w.store.ReleaseLease(ctx, dedupKey, w.ownerID)
		return err
	}

	return w.store.MarkFlushed(ctx, dedupKey, itemIDs)
}

func (w *Worker) accumulate(items []Envelope) Envelope {
	envelope := items[0]
	moreCount := max(len(items)-w.maxRenderedItems, 0)
	renderedItems := make([]map[string]any, 0, min(len(items), w.maxRenderedItems))

	for _, item := range items[:min(len(items), w.maxRenderedItems)] {
		renderedItems = append(renderedItems, item.TemplateData)
	}

	envelope.TemplateData = map[string]any{
		"_count":     len(items),
		"_items":     renderedItems,
		"_moreCount": moreCount,
	}
	return envelope
}

func (w *Worker) scheduleFlush(ctx context.Context, dedupKey string, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if timer, ok := w.timers[dedupKey]; ok {
		timer.Stop()
	}

	w.timers[dedupKey] = time.AfterFunc(delay, func() {
		_ = w.Flush(ctx, dedupKey)
	})
}
