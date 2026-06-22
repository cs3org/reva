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
	"math/rand"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
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
}

// Runner owns the scheduling, dispatching and execution of jobs. A process
// has a single Runner, created by the jobs service and reachable through
// Default for in-process Enqueue.
type Runner struct {
	workers  int
	store    Store
	status   StatusStore
	log      zerolog.Logger
	periodic []Periodic

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// running guards against overlapping runs for jobs whose Overlap policy is
	// Skip.
	runningMu sync.Mutex
	running   map[string]bool
}

// NewRunner builds a Runner from the registered jobs and the given options.
func NewRunner(ctx context.Context, opts Options) (*Runner, error) {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}

	r := &Runner{
		workers:  opts.Workers,
		store:    opts.Store,
		status:   opts.Status,
		log:      *appctx.GetLogger(ctx),
		periodic: registeredPeriodic(),
		running:  make(map[string]bool),
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

// Status returns the current status of a previously enqueued run. It returns
// an errtypes.NotFound error if the run is unknown.
func (r *Runner) Status(ctx context.Context, id RunID) (Status, error) {
	if r.status == nil {
		return Status{}, errors.New("rjobs: no status store configured")
	}
	return r.status.Get(ctx, id)
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

	stopBeat := r.startHeartbeat(ctx, run.ID, log)
	defer stopBeat()

	r.recordStatus(ctx, run, StateRunning, nil, nil, log)

	result, err := r.invoke(ctx, run, log)
	if err != nil {
		log.Error().Err(err).Msg("rjobs: run failed")
		r.recordStatus(ctx, run, StateFailed, nil, err, log)
		if ferr := r.store.Fail(ctx, run.ID, defaultRetryAfter); ferr != nil {
			log.Error().Err(ferr).Msg("rjobs: marking run failed errored")
		}
		return
	}

	r.recordStatus(ctx, run, StateSucceeded, result, nil, log)
	if cerr := r.store.Complete(ctx, run.ID); cerr != nil {
		log.Error().Err(cerr).Msg("rjobs: completing run errored")
	}
}

// startHeartbeat keeps a claimed run's lease alive by calling the store's
// Heartbeat on a ticker until the returned stop function is called. It lets a
// job run for arbitrarily long (minutes or days) without being redelivered.
func (r *Runner) startHeartbeat(ctx context.Context, id RunID, log zerolog.Logger) (stop func()) {
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
				if err := r.store.Heartbeat(ctx, id); err != nil {
					log.Warn().Err(err).Msg("rjobs: heartbeat failed")
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
func (r *Runner) recordStatus(ctx context.Context, run Run, state State, result Params, runErr error, log zerolog.Logger) {
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
	switch state {
	case StateRunning:
		st.StartedAt = &now
	case StateSucceeded, StateFailed:
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
	job, err := newFunc(jobCtx, nil)
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
