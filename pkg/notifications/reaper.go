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
	"math/rand"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/rjobs"
)

const defaultReaperInterval = 15 * time.Minute

// ReaperJobName is the rjobs periodic job name for accumulator recovery.
const ReaperJobName = "notifications.reaper"

// Reaper periodically recovers accumulator buckets that are due, unleased, or
// have an expired lease.
type Reaper struct {
	worker   *Worker
	interval time.Duration
	limit    int
	rand     *rand.Rand
	once     sync.Once
}

// ReaperConfig configures the accumulator reaper.
type ReaperConfig struct {
	Interval time.Duration
	Limit    int
}

// NewReaper creates a reaper for the given worker.
func NewReaper(worker *Worker, conf ReaperConfig) *Reaper {
	if conf.Interval <= 0 {
		conf.Interval = defaultReaperInterval
	}
	if conf.Limit <= 0 {
		conf.Limit = 100
	}

	return &Reaper{
		worker:   worker,
		interval: conf.Interval,
		limit:    conf.Limit,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RegisterReaperJob registers the accumulator reaper with Reva's jobs
// framework. The job runs on every node because lease acquisition is already
// SQL-coordinated and each node must be able to recover work after a local
// notification worker dies.
func RegisterReaperJob(reaper *Reaper) error {
	if reaper == nil {
		return errors.New("notifications: reaper is required")
	}

	return rjobs.RegisterPeriodic(rjobs.Periodic{
		Name:       ReaperJobName,
		Schedule:   "@every " + reaper.interval.String(),
		Scope:      rjobs.ScopeAllNodes,
		RunOnStart: true,
		Overlap:    rjobs.Skip,
		Run: func(ctx context.Context) error {
			return reaper.Run(ctx)
		},
	})
}

// Run performs one reaper pass. The first pass waits for a random duration in
// [0, interval) so boxes spread their startup scans over time.
func (r *Reaper) Run(ctx context.Context) error {
	var err error
	r.once.Do(func() {
		err = r.waitInitialDelay(ctx)
	})
	if err != nil {
		return err
	}
	return r.RunOnce(ctx)
}

func (r *Reaper) waitInitialDelay(ctx context.Context) error {
	if r == nil || r.interval <= 0 {
		return nil
	}

	delay := time.Duration(r.rand.Int63n(int64(r.interval)))
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RunOnce performs one recovery scan.
func (r *Reaper) RunOnce(ctx context.Context) error {
	if r == nil || r.worker == nil || r.worker.store == nil {
		return nil
	}

	buckets, err := r.worker.store.ListCandidates(ctx, r.worker.now(), r.limit)
	if err != nil {
		return err
	}

	errs := make([]error, 0)
	for _, bucket := range buckets {
		if err := r.worker.resumeBucket(ctx, bucket); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
