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
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// defaultRetryAfter is how long a failed on-demand or leader run waits before
// it becomes claimable again.
const defaultRetryAfter = 30 * time.Second

// schedulerTick is how often the scheduler loop checks for due periodic jobs.
const schedulerTick = 10 * time.Second

// Options configures a Runner.
type Options struct {
	// Workers is the number of concurrent workers draining the durable queue.
	Workers int
	// Store is the durable backend. It may be nil, in which case only
	// all-nodes periodic jobs run (no leader-scoped or on-demand work).
	Store Store
	// Status records and serves per-run status. It is required whenever Store
	// is set, since every durable run gets a status record.
	Status StatusStore
	// OnDemandConfig holds the configuration of the on-demand jobs, keyed by
	// job name. When a run is dispatched the entry for its job is handed to the
	// job's constructor; a job with no entry is built with a nil map and falls
	// back to its own defaults.
	OnDemandConfig map[string]map[string]any
}

// Runner owns the scheduling, dispatching and execution of jobs. A process
// has a single Runner, created by the jobs service and reachable through
// Default for in-process Enqueue.
type Runner struct {
	workers        int
	store          Store
	status         StatusStore
	log            zerolog.Logger
	periodic       []Periodic
	onDemandConfig map[string]map[string]any

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// running guards against overlapping runs for jobs whose Overlap policy is
	// Skip.
	runningMu sync.Mutex
	running   map[string]bool

	// cancels holds the cancellation handle of every run currently executing on
	// this process, so a cancel request can interrupt the right run. A run is
	// added when execRun starts it and removed when it returns.
	cancelsMu sync.Mutex
	cancels   map[RunID]*runHandle
}

// runHandle is the per-run cancellation handle held while a run executes on
// this process. cancel stops the run's context; cancelled records that the stop
// was an explicit cancellation and not a shutdown, so execRun finalises the run
// as cancelled instead of retrying it. once keeps tripping it idempotent across
// the several places that may request it (a direct local cancel, the cancel
// broadcast, the backstop poll).
type runHandle struct {
	job       string
	started   time.Time
	cancel    context.CancelFunc
	cancelled atomic.Bool
	once      sync.Once
}

// trip marks the run cancelled and cancels its context. Only the first call has
// an effect.
func (h *runHandle) trip() {
	h.once.Do(func() {
		h.cancelled.Store(true)
		h.cancel()
	})
}

// NewRunner builds a Runner from the registered jobs and the given options.
func NewRunner(ctx context.Context, opts Options) (*Runner, error) {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}

	r := &Runner{
		workers:        opts.Workers,
		store:          opts.Store,
		status:         opts.Status,
		log:            *appctx.GetLogger(ctx),
		periodic:       registeredPeriodic(),
		onDemandConfig: opts.OnDemandConfig,
		running:        make(map[string]bool),
		cancels:        make(map[RunID]*runHandle),
	}

	// leader-scoped and on-demand work both need a store.
	if r.store == nil {
		for _, p := range r.periodic {
			if p.Scope == ScopeLeader {
				return nil, errors.Errorf("rjobs: periodic job %q is leader-scoped but no store is configured", p.Name)
			}
		}
	}

	// a durable backend implies a status store: every queued/claimed run gets
	// a status record.
	if r.store != nil && r.status == nil {
		return nil, errors.New("rjobs: a status store is required when a durable store is configured")
	}

	return r, nil
}

// Start launches the runner loops. It returns immediately; the loops run until
// Stop is called.
func (r *Runner) Start() {
	ctx, cancel := context.WithCancel(appctx.WithLogger(context.Background(), &r.log))
	r.cancel = cancel

	// all-nodes periodic jobs run as local tickers, regardless of the store.
	for _, p := range r.periodic {
		if p.Scope == ScopeAllNodes {
			r.wg.Go(func() { r.runLocalTicker(ctx, p) })
		}
	}

	if r.store == nil {
		r.log.Info().Int("local_jobs", len(r.periodic)).Msg("rjobs: started without a store, only all-nodes jobs run")
		return
	}

	// subscribe to cluster-wide cancel broadcasts so a cancel issued on another
	// process can stop a run executing here. It is optional: without it cancels
	// still land via the backstop poll, just not instantly.
	if bus, ok := r.store.(ControlBus); ok {
		if err := bus.SubscribeCancel(ctx, r.tripLocal); err != nil {
			r.log.Error().Err(err).Msg("rjobs: subscribing to cancel broadcasts failed, cancels fall back to the backstop poll")
		}
	}

	// register leader-scoped schedules so the scheduler can track them.
	for _, p := range r.periodic {
		if p.Scope != ScopeLeader {
			continue
		}
		sched, _ := ParseSchedule(p.Schedule) // validated at registration
		next := time.Now().Add(sched.Interval())
		if p.RunOnStart {
			next = time.Now()
		}
		if err := r.store.RegisterScheduled(ctx, p.Name, sched, next); err != nil {
			r.log.Error().Err(err).Str("job", p.Name).Msg("rjobs: registering schedule failed")
		}
	}

	r.wg.Go(func() { r.runScheduler(ctx) })

	for i := 0; i < r.workers; i++ {
		r.wg.Go(func() { r.runDispatcher(ctx) })
	}

	r.log.Info().Int("workers", r.workers).Msg("rjobs: runner started")
}

// Stop cancels the loops and waits for in-flight work to drain, bounded by
// ctx.
func (r *Runner) Stop(ctx context.Context) error {
	if r.cancel != nil {
		r.cancel()
	}

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		r.log.Warn().Msg("rjobs: shutdown deadline reached with work still in flight")
	}

	if r.store != nil {
		if err := r.store.Close(ctx); err != nil {
			return err
		}
	}
	if r.status != nil {
		if err := r.status.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Enqueue schedules an on-demand job to run as soon as a worker is free. It is
// durable: it returns once the run is persisted, so the run survives a restart
// of this replica. This is the in-process trigger API; a future RPC service
// can wrap it without the core changing.
func (r *Runner) Enqueue(ctx context.Context, name string, p Params, opts ...EnqueueOption) (RunID, error) {
	if r.store == nil {
		return "", errors.New("rjobs: cannot enqueue, no store configured")
	}
	if _, ok := lookupOnDemand(name); !ok {
		return "", errors.Errorf("rjobs: no on-demand job %q registered", name)
	}

	run := Run{Job: name, Params: p}
	for _, o := range opts {
		o(&run)
	}

	if run.DedupKey != "" {
		return r.enqueueUnique(ctx, run)
	}

	id, err := r.store.Enqueue(ctx, run)
	if err != nil {
		return "", err
	}

	if err := r.status.Put(ctx, Status{
		RunID:      id,
		Job:        name,
		State:      StateQueued,
		Attempt:    1,
		EnqueuedAt: time.Now(),
		Owner:      run.Owner,
	}); err != nil {
		// the run is already durably queued; a failed status write must not
		// fail the enqueue, only lose observability for this run.
		r.log.Error().Err(err).Str("run", string(id)).Msg("rjobs: recording queued status failed")
	}
	return id, nil
}

// enqueueUnique enqueues a run that carries a Unique key. It reserves the key in
// the status store before publishing, so two concurrent enqueues for the same
// (owner, key) cannot both start a run: the reservation, backed by a unique
// index, is the gate. On conflict it collapses onto the in-flight run (default)
// or rejects, per the run's policy.
func (r *Runner) enqueueUnique(ctx context.Context, run Run) (RunID, error) {
	run.ID = RunID(uuid.New().String())
	run.Attempt = 1
	run.EnqueuedAt = time.Now()

	queued := Status{
		RunID:      run.ID,
		Job:        run.Job,
		State:      StateQueued,
		Attempt:    1,
		EnqueuedAt: run.EnqueuedAt,
		Owner:      run.Owner,
	}

	existing, reserved, err := r.status.Reserve(ctx, queued, run.DedupKey)
	if err != nil {
		return "", err
	}
	if !reserved {
		if run.dedupReject {
			return "", errtypes.AlreadyExists(fmt.Sprintf("run %s already active for key %q", existing.RunID, run.DedupKey))
		}
		return existing.RunID, nil
	}

	if _, err := r.store.Enqueue(ctx, run); err != nil {
		// the key is reserved but nothing reached the queue. Release it and mark
		// the orphaned run failed, so the key is not stuck and no phantom queued
		// run lingers.
		if rerr := r.status.Release(ctx, run.ID); rerr != nil {
			r.log.Error().Err(rerr).Str("run", string(run.ID)).Msg("rjobs: releasing reservation after failed enqueue errored")
		}
		now := time.Now()
		failed := queued
		failed.State = StateFailed
		failed.FinishedAt = &now
		failed.LastError = err.Error()
		if perr := r.status.Put(ctx, failed); perr != nil {
			r.log.Error().Err(perr).Str("run", string(run.ID)).Msg("rjobs: recording failed status after failed enqueue errored")
		}
		return "", err
	}
	return run.ID, nil
}

// TriggerNow enqueues an immediate, out-of-band run of a leader-scoped periodic
// job, on top of its schedule: the regular cadence is left untouched, so this is
// an extra run, not a reschedule. It respects the job's single-flight guard, so
// it is rejected with an error if a run of the job is already in flight. Like
// Enqueue it is in-process; on-demand and all-nodes jobs cannot be triggered.
func (r *Runner) TriggerNow(ctx context.Context, job string) error {
	if r.store == nil {
		return errors.New("rjobs: cannot trigger, no store configured")
	}
	if !r.isLeaderJob(job) {
		return errors.Errorf("rjobs: %q is not a registered leader periodic job", job)
	}

	acquired, err := r.store.TryMarkScheduledRunning(ctx, job)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.Errorf("rjobs: a run of %q is already in flight", job)
	}

	if _, err := r.store.Enqueue(ctx, Run{Job: job}); err != nil {
		// release the gate we just took so the job is not stuck marked-running.
		if cerr := r.store.ClearScheduledRunning(ctx, job); cerr != nil {
			r.log.Error().Err(cerr).Str("job", job).Msg("rjobs: releasing schedule mark after a failed trigger errored")
		}
		return errors.Wrap(err, "rjobs: enqueuing triggered run failed")
	}
	r.log.Info().Str("job", job).Msg("rjobs: triggered an immediate run")
	return nil
}

// Status returns the current status of a previously enqueued run. It returns
// an errtypes.NotFound error if the run is unknown.
func (r *Runner) Status(ctx context.Context, id RunID) (Status, error) {
	if r.status == nil {
		return Status{}, errors.New("rjobs: no status store configured")
	}
	return r.status.Get(ctx, id)
}

// ListByOwner returns the runs created for the given user, most recently
// enqueued first. It is the read side of WithOwner: a UI lists a user's jobs
// with it. The filter narrows the result further (by state, job or page); its
// Owner and Internal fields are ignored — the owner argument wins and internal
// runs are never included.
func (r *Runner) ListByOwner(ctx context.Context, owner string, f ListFilter) ([]Status, error) {
	if r.status == nil {
		return nil, errors.New("rjobs: no status store configured")
	}
	f.Owner = owner
	f.Internal = false
	return r.status.List(ctx, f)
}

// Cancel requests cancellation of a previously enqueued run and returns its
// updated status. Cancellation is cooperative and asynchronous: it records a
// durable cancel intent and stops the run on this process if it is running
// here. The run reaches StateCancelled once it actually stops, so callers
// observe Status to confirm. A run that ignores its context runs to completion.
// Cancelling a finished run is a no-op; it returns an errtypes.NotFound error
// if the run is unknown.
func (r *Runner) Cancel(ctx context.Context, id RunID) (Status, error) {
	if r.status == nil {
		return Status{}, errors.New("rjobs: no status store configured")
	}
	// Durable intent first: it is the source of truth and the backstop should
	// the fast local path not reach the worker running the job.
	st, err := r.status.RequestCancel(ctx, id)
	if err != nil {
		return Status{}, err
	}
	// Fast path: stop it now if it runs here, and broadcast so a worker running
	// it on another process stops without waiting for the backstop poll.
	r.tripLocal(CancelSignal{RunID: id})
	r.broadcastCancel(ctx, CancelSignal{RunID: id})
	return st, nil
}

// CancelPeriodic requests cancellation of the in-flight run of a leader-scoped
// periodic job, addressed by job name (a leader job has at most one run in
// flight). Like Cancel it is cooperative and asynchronous. The schedule itself
// is untouched, so the job keeps firing on its normal cadence; this stops only
// the current run. It returns an error if the job is not a registered leader
// job or if no run of it is currently in flight.
func (r *Runner) CancelPeriodic(ctx context.Context, job string) error {
	if r.store == nil {
		return errors.New("rjobs: cannot cancel, no store configured")
	}
	if !r.isLeaderJob(job) {
		return errors.Errorf("rjobs: %q is not a registered leader periodic job", job)
	}

	running, err := r.store.RequestCancelScheduled(ctx, job)
	if err != nil {
		return err
	}
	if !running {
		return errors.Errorf("rjobs: no run of %q is currently in flight", job)
	}

	// Fast path: stop it here if it runs on this process, and broadcast to the
	// process running it otherwise.
	r.tripLocal(CancelSignal{Job: job})
	r.broadcastCancel(ctx, CancelSignal{Job: job})
	return nil
}

// registerRun records a run's cancellation handle for the duration of its
// execution on this process.
func (r *Runner) registerRun(id RunID, h *runHandle) {
	r.cancelsMu.Lock()
	r.cancels[id] = h
	r.cancelsMu.Unlock()
}

// deregisterRun drops a run's cancellation handle once it has finished.
func (r *Runner) deregisterRun(id RunID) {
	r.cancelsMu.Lock()
	delete(r.cancels, id)
	r.cancelsMu.Unlock()
}

// tripLocal stops any run matching sig that is executing on this process: a run
// by id (on-demand) or every in-flight run of a job (periodic). It is the
// node-local half of a cancel, invoked directly and from the cancel broadcast,
// and a no-op when no matching run is running here.
func (r *Runner) tripLocal(sig CancelSignal) {
	r.cancelsMu.Lock()
	defer r.cancelsMu.Unlock()
	if sig.RunID != "" {
		if h, ok := r.cancels[sig.RunID]; ok {
			h.trip()
		}
		return
	}
	if sig.Job == "" {
		return
	}
	for _, h := range r.cancels {
		if h.job == sig.Job {
			h.trip()
		}
	}
}

// broadcastCancel best-effort publishes a cancel signal to the other processes
// when the store supports it, so a run executing elsewhere stops without waiting
// for its backstop poll. A failure only falls back to that poll, so it is
// logged, not returned.
func (r *Runner) broadcastCancel(ctx context.Context, sig CancelSignal) {
	bus, ok := r.store.(ControlBus)
	if !ok {
		return
	}
	if err := bus.PublishCancel(ctx, sig); err != nil {
		r.log.Warn().Err(err).Msg("rjobs: broadcasting cancel failed; relying on the backstop poll")
	}
}

// runLocalTicker drives an all-nodes periodic job on this replica. It never
// touches the store.
func (r *Runner) runLocalTicker(ctx context.Context, p Periodic) {
	sched, _ := ParseSchedule(p.Schedule)
	log := r.log.With().Str("job", p.Name).Str("scope", p.Scope.String()).Logger()

	if p.RunOnStart {
		r.execPeriodic(ctx, p, log)
	}

	for {
		wait := sched.Interval() + jitter(p.Jitter)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			r.execPeriodic(ctx, p, log)
		}
	}
}

// runScheduler enqueues due leader-scoped periodic runs. Every jobs process
// runs this loop: the store's DueScheduled atomically advances each job's
// next-fire, so exactly one process wins a given tick and enqueues it. No
// leader election is needed.
func (r *Runner) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			due, err := r.store.DueScheduled(ctx, time.Now())
			if err != nil {
				r.log.Error().Err(err).Msg("rjobs: querying due jobs failed")
				continue
			}
			for _, d := range due {
				// The store may surface a schedule entry for a job that is no
				// longer a registered leader job: its scope changed to
				// all-nodes, or it was deleted/renamed, leaving a stale entry
				// in the store. Skip it here rather than enqueuing a run that
				// would double-execute (all-nodes) or reference a job that no
				// longer exists. The stale entry is left in place; it is
				// harmless once it is never enqueued.
				if !r.isLeaderJob(d.Job) {
					r.log.Debug().Str("job", d.Job).Msg("rjobs: skipping stale schedule entry, not a registered leader job")
					continue
				}
				if _, err := r.store.Enqueue(ctx, Run{Job: d.Job}); err != nil {
					r.log.Error().Err(err).Str("job", d.Job).Msg("rjobs: enqueuing periodic run failed")
					continue
				}
				// mark the job as in flight so the scheduler does not enqueue
				// another run until this one finishes (see execRun, which
				// clears it).
				if err := r.store.MarkScheduledRunning(ctx, d.Job); err != nil {
					r.log.Error().Err(err).Str("job", d.Job).Msg("rjobs: marking schedule running failed")
				}
			}
		}
	}
}

// runDispatcher claims runs from the store and executes them.
func (r *Runner) runDispatcher(ctx context.Context) {
	for {
		run, err := r.store.Claim(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.log.Error().Err(err).Msg("rjobs: claiming run failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		r.execRun(ctx, run)
	}
}

// execRun runs a claimed durable run (on-demand or leader periodic), records
// its status transitions and acks/naks it. While the job runs it heartbeats
// the store so a long run keeps its claim and is not redelivered.
func (r *Runner) execRun(ctx context.Context, run Run) {
	log := r.log.With().Str("job", run.Job).Str("run", string(run.ID)).Int("attempt", run.Attempt).Logger()

	// once this run finishes, a leader-scoped periodic job is no longer in
	// flight, so the scheduler may schedule it again.
	if r.isLeaderJob(run.Job) {
		defer func() {
			if err := r.store.ClearScheduledRunning(ctx, run.Job); err != nil {
				log.Error().Err(err).Msg("rjobs: clearing schedule running mark failed")
			}
		}()
	}

	// Per-run cancellable context, registered so a cancel request can interrupt
	// this specific run. It derives from ctx, so a shutdown still cancels it too;
	// the runHandle.cancelled flag is what tells the two apart below.
	runCtx, cancel := context.WithCancel(ctx)
	h := &runHandle{job: run.Job, started: time.Now(), cancel: cancel}
	r.registerRun(run.ID, h)
	defer r.deregisterRun(run.ID)
	defer cancel()

	// A cancellation may have arrived while the run was queued or waiting to be
	// retried. Honour it before doing any work: drop the run as cancelled
	// instead of executing it.
	if r.cancelRequested(ctx, run) {
		log.Info().Msg("rjobs: run cancelled before it started")
		r.finishCancelled(ctx, run, time.Time{}, log)
		return
	}

	stopBeat := r.startHeartbeat(ctx, run, h, log)
	defer stopBeat()

	r.recordStatus(ctx, run, StateRunning, nil, nil, h.started, log)

	result, err := r.invoke(runCtx, run, log)

	// An explicit cancellation wins over the job's return value: the run is
	// terminal and must not be retried, whether the job returned an error or
	// not. A shutdown does not set this flag, so in-flight work is still retried
	// after a restart.
	if h.cancelled.Load() {
		log.Info().Msg("rjobs: run cancelled")
		r.finishCancelled(ctx, run, h.started, log)
		return
	}

	if err != nil {
		log.Error().Err(err).Msg("rjobs: run failed")
		r.recordStatus(ctx, run, StateFailed, nil, err, h.started, log)
		if ferr := r.store.Fail(ctx, run.ID, defaultRetryAfter); ferr != nil {
			log.Error().Err(ferr).Msg("rjobs: marking run failed errored")
		}
		return
	}

	r.recordStatus(ctx, run, StateSucceeded, result, nil, h.started, log)
	if run.DedupKey != "" && r.status != nil {
		// the run is done; free its Unique key so a new run can take it.
		if err := r.status.Release(ctx, run.ID); err != nil {
			log.Error().Err(err).Msg("rjobs: releasing dedup reservation errored")
		}
	}
	if cerr := r.store.Complete(ctx, run.ID); cerr != nil {
		log.Error().Err(cerr).Msg("rjobs: completing run errored")
	}
}

// cancelRequested reports whether a cancellation has been requested for run. It
// reads the durable cancel intent kept in the status store. Periodic runs are
// not status-tracked, so this is always false for them; their cancellation is
// driven separately through the schedule store.
func (r *Runner) cancelRequested(ctx context.Context, run Run) bool {
	if _, ok := r.lookupPeriodic(run.Job); ok {
		// Periodic runs are not status-tracked; their cancel intent lives in the
		// schedule store, keyed by job name.
		req, err := r.store.ScheduledCancelRequested(ctx, run.Job)
		if err != nil {
			return false
		}
		return req
	}
	st, err := r.status.Get(ctx, run.ID)
	if err != nil {
		// A missing or unreadable status must not block execution; the run just
		// misses its cancel check this round.
		return false
	}
	return st.CancelRequested
}

// finishCancelled records a run as cancelled and acks it so it is not
// redelivered. Cancellation is terminal, unlike a failure.
func (r *Runner) finishCancelled(ctx context.Context, run Run, started time.Time, log zerolog.Logger) {
	r.recordStatus(ctx, run, StateCancelled, nil, nil, started, log)
	if err := r.store.Complete(ctx, run.ID); err != nil {
		log.Error().Err(err).Msg("rjobs: completing cancelled run errored")
	}
}

// startHeartbeat keeps a claimed run's lease alive by calling the store's
// Heartbeat on a ticker until the returned stop function is called. It lets a
// job run for arbitrarily long (minutes or days) without being redelivered.
func (r *Runner) startHeartbeat(ctx context.Context, run Run, h *runHandle, log zerolog.Logger) (stop func()) {
	interval := r.store.HeartbeatInterval()
	if interval <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.store.Heartbeat(ctx, run.ID); err != nil {
					log.Warn().Err(err).Msg("rjobs: heartbeat failed")
				}
				// Backstop for a cancel that did not reach this worker by the
				// fast path: notice the durable cancel intent and stop the run.
				// It keeps beating afterwards so the lease stays alive while the
				// job winds down.
				if !h.cancelled.Load() && r.cancelRequested(ctx, run) {
					log.Info().Msg("rjobs: cancellation observed, stopping run")
					h.trip()
				}
			}
		}
	}()

	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// recordStatus upserts a run's status. Periodic runs (those matching a
// registered periodic job) are not tracked: their observability comes from the
// scheduler, not the per-run status store.
func (r *Runner) recordStatus(ctx context.Context, run Run, state State, result Params, runErr error, started time.Time, log zerolog.Logger) {
	if r.status == nil {
		return
	}
	if _, ok := r.lookupPeriodic(run.Job); ok {
		return
	}

	now := time.Now()
	st := Status{
		RunID:      run.ID,
		Job:        run.Job,
		State:      state,
		Attempt:    run.Attempt,
		EnqueuedAt: run.EnqueuedAt,
		Result:     result,
		Owner:      run.Owner,
	}
	// StartedAt travels with every write once the run has begun: recordStatus
	// upserts the whole row, so a terminal write that omitted it would blank the
	// start time recorded at StateRunning.
	if !started.IsZero() {
		s := started
		st.StartedAt = &s
	}
	switch state {
	case StateSucceeded, StateFailed, StateCancelled:
		st.FinishedAt = &now
	}
	if runErr != nil {
		st.LastError = runErr.Error()
	}

	if err := r.status.Put(ctx, st); err != nil {
		log.Error().Err(err).Str("state", string(state)).Msg("rjobs: recording status failed")
	}
}

// invoke dispatches a run to the right registered job. Periodic runs (empty
// Params and a known periodic name) call the periodic Run closure; everything
// else is an on-demand job. The returned Params are the on-demand job's result
// (always nil for periodic runs).
func (r *Runner) invoke(ctx context.Context, run Run, log zerolog.Logger) (Params, error) {
	jobCtx := appctx.WithLogger(ctx, &log)

	if p, ok := r.lookupPeriodic(run.Job); ok {
		return nil, r.guard(p, func() error { return p.Run(jobCtx) })
	}

	newFunc, ok := lookupOnDemand(run.Job)
	if !ok {
		return nil, errors.Errorf("rjobs: run references unknown job %q", run.Job)
	}
	job, err := newFunc(jobCtx, r.onDemandConfig[run.Job])
	if err != nil {
		return nil, errors.Wrap(err, "rjobs: building on-demand job failed")
	}
	return job.Run(jobCtx, run.Params)
}

// execPeriodic runs a periodic Run closure inline (used by the local ticker).
func (r *Runner) execPeriodic(ctx context.Context, p Periodic, log zerolog.Logger) {
	start := time.Now()
	err := r.guard(p, func() error { return p.Run(appctx.WithLogger(ctx, &log)) })
	switch {
	case errors.Is(err, errSkippedOverlap):
		log.Debug().Msg("rjobs: skipped, previous run still in flight")
	case err != nil:
		log.Error().Err(err).Dur("took", time.Since(start)).Msg("rjobs: periodic run failed")
	default:
		log.Debug().Dur("took", time.Since(start)).Msg("rjobs: periodic run done")
	}
}

var errSkippedOverlap = errors.New("rjobs: run skipped due to overlap policy")

// guard enforces the overlap policy for a periodic job.
func (r *Runner) guard(p Periodic, fn func() error) error {
	if p.Overlap == Allow {
		return fn()
	}

	r.runningMu.Lock()
	if r.running[p.Name] {
		r.runningMu.Unlock()
		return errSkippedOverlap
	}
	r.running[p.Name] = true
	r.runningMu.Unlock()

	defer func() {
		r.runningMu.Lock()
		r.running[p.Name] = false
		r.runningMu.Unlock()
	}()

	return fn()
}

func (r *Runner) lookupPeriodic(name string) (Periodic, bool) {
	for _, p := range r.periodic {
		if p.Name == name {
			return p, true
		}
	}
	return Periodic{}, false
}

// isLeaderJob reports whether name is currently registered as a leader-scoped
// periodic job. The scheduler uses it to ignore stale schedule entries left by
// a job that changed scope or was removed.
func (r *Runner) isLeaderJob(name string) bool {
	p, ok := r.lookupPeriodic(name)
	return ok && p.Scope == ScopeLeader
}

// jitter returns a random non-negative duration in [0, max).
func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}
