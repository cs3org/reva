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

package rjobs

import (
	"context"
	"time"
)

// RunID is the identifier of a single durable run.
type RunID string

// Run is one durable unit of work waiting in (or claimed from) the store. It
// covers both on-demand runs and the runs enqueued by the scheduler on behalf
// of leader-scoped periodic jobs.
type Run struct {
	// ID is assigned by the store on Enqueue.
	ID RunID
	// Job is the registered name of the job to run.
	Job string
	// Params is the on-demand payload. Empty for periodic runs.
	Params Params
	// IdempotencyKey, when set, collapses duplicate enqueues into a single
	// run. Two enqueues with the same key yield the same run.
	IdempotencyKey string
	// Attempt is the 1-based delivery attempt for this run.
	Attempt int
	// EnqueuedAt is when the run was first persisted.
	EnqueuedAt time.Time
}

// ScheduledRun describes a periodic job whose next fire time has come due. It
// is returned by Store.DueScheduled, which atomically advances the stored
// next-fire so a concurrent scheduler cannot enqueue the same tick twice.
type ScheduledRun struct {
	// Job is the registered name of the periodic job.
	Job string
	// Next is the next-fire time the store advanced to.
	Next time.Time
}

// Store is the durable backend for leader-scoped periodic jobs and on-demand
// jobs. It is the source of truth for single-firing: DueScheduled atomically
// advances a job's next-fire, so correctness does not depend on the elector.
//
// Claimed runs are leased with a visibility timeout; if a worker dies without
// calling Complete or Fail, the lease expires and the run becomes claimable
// again, giving at-least-once delivery.
type Store interface {
	// Enqueue persists a ready-to-run job instance and returns its id. If the
	// run carries an IdempotencyKey that already has a live run, the existing
	// run's id is returned and no new run is created.
	Enqueue(ctx context.Context, run Run) (RunID, error)
	// Claim atomically leases the next ready run to the caller. It blocks
	// until a run is available or ctx is cancelled.
	Claim(ctx context.Context) (Run, error)
	// Complete acknowledges a run as successfully processed.
	Complete(ctx context.Context, id RunID) error
	// Fail marks a run as failed and schedules it to become claimable again
	// after retryAfter.
	Fail(ctx context.Context, id RunID, retryAfter time.Duration) error
	// Heartbeat extends the lease of an in-flight run, so a job that runs
	// longer than the visibility timeout is not redelivered while it is still
	// making progress. The runner calls it every HeartbeatInterval until the
	// job returns.
	Heartbeat(ctx context.Context, id RunID) error
	// HeartbeatInterval is how often the runner should call Heartbeat for an
	// in-flight run, chosen so the lease never lapses between beats.
	HeartbeatInterval() time.Duration
	// RegisterScheduled records a leader-scoped periodic job's schedule so
	// DueScheduled can track its next-fire across restarts. An existing
	// next-fire is preserved across restarts, except when the configured
	// interval changed, in which case the new interval and the given next-fire
	// are adopted.
	RegisterScheduled(ctx context.Context, job string, schedule Schedule, next time.Time) error
	// DueScheduled returns the periodic jobs whose next-fire is at or before
	// now, atomically advancing each one's stored next-fire by its interval. A
	// job whose previous run is still in flight (see MarkScheduledRunning) is
	// not returned, so a run that takes longer than its interval does not pile
	// up; its schedule resumes once the run is cleared.
	DueScheduled(ctx context.Context, now time.Time) ([]ScheduledRun, error)
	// MarkScheduledRunning records that a leader-scoped periodic job has a run
	// in flight, so DueScheduled skips it until the run is cleared.
	MarkScheduledRunning(ctx context.Context, job string) error
	// ClearScheduledRunning clears the in-flight mark for a leader-scoped
	// periodic job once its run finishes, letting its schedule resume.
	ClearScheduledRunning(ctx context.Context, job string) error
	// Close releases the store's resources.
	Close(ctx context.Context) error
}
