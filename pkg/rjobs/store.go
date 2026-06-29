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
	// Owner is the username the run was created for, or empty for an internal
	// run (periodic jobs, and on-demand jobs enqueued without WithOwner). It
	// travels with the run so the worker can record it in the run's status.
	Owner string
	// DedupKey, set through Unique, makes the run the only active one for its
	// (Owner, DedupKey): while it is queued, running or retrying, another
	// enqueue with the same key does not start a second run. It travels with the
	// run so the reservation can be released once the run succeeds.
	DedupKey string
	// dedupReject selects Reject over the default Collapse when DedupKey is
	// already held. It is read at enqueue time only and never leaves the
	// process, so it is deliberately unexported and not serialised.
	dedupReject bool
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
	// Enqueue persists a ready-to-run job instance and returns its id.
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
	// TryMarkScheduledRunning marks a leader-scoped periodic job as having a run
	// in flight, but only if one is not already marked, reporting whether it
	// acquired the mark. It is the single-flight gate for an out-of-band trigger:
	// the caller that acquires it may enqueue an immediate run, and
	// ClearScheduledRunning releases it when the run finishes. It returns an
	// error if the job has no registered schedule.
	TryMarkScheduledRunning(ctx context.Context, job string) (bool, error)
	// ClearScheduledRunning clears the in-flight mark for a leader-scoped
	// periodic job once its run finishes, letting its schedule resume. It also
	// clears any cancel intent recorded by RequestCancelScheduled, so a cancel
	// can never carry over to the job's next run.
	ClearScheduledRunning(ctx context.Context, job string) error
	// RequestCancelScheduled records a cancel intent for a leader-scoped periodic
	// job's in-flight run, reporting whether a run was in flight to cancel. It
	// only marks while a run is actually running, and ClearScheduledRunning
	// clears it, so the intent never leaks to a later run.
	RequestCancelScheduled(ctx context.Context, job string) (bool, error)
	// ScheduledCancelRequested reports whether a cancel has been requested for a
	// leader-scoped periodic job's in-flight run. The worker running the job
	// polls it as the backstop to the cancel broadcast.
	ScheduledCancelRequested(ctx context.Context, job string) (bool, error)
	// Close releases the store's resources.
	Close(ctx context.Context) error
}

// CancelSignal is a best-effort, cluster-wide notification that a run should be
// cancelled. Exactly one field is set: RunID targets a single on-demand run;
// Job targets the in-flight run of a periodic job, which each process matches
// against the runs it is currently executing.
type CancelSignal struct {
	RunID RunID
	Job   string
}

// ControlBus is an optional capability of a Store: a best-effort, cluster-wide
// pub/sub that delivers cancel signals to whichever process is running a given
// run, without waiting for the durable backstop poll. It is strictly an
// optimisation over the durable cancel intent: a dropped signal only delays a
// cancel to the next poll, it never loses it. A Store that does not implement
// ControlBus still cancels correctly, just not instantly across processes.
type ControlBus interface {
	// PublishCancel broadcasts a cancel signal to every subscribed process.
	PublishCancel(ctx context.Context, sig CancelSignal) error
	// SubscribeCancel registers handler for every cancel signal published in the
	// cluster, including by other processes. It returns once the subscription is
	// active.
	SubscribeCancel(ctx context.Context, handler func(CancelSignal)) error
}
