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
	// Elector decides whether this replica schedules leader-scoped jobs.
	// Defaults to AlwaysLeader.
	Elector Elector
}

// Runner owns the scheduling, dispatching and execution of jobs. A process
// has a single Runner, created by the jobs service and reachable through
// Default for in-process Enqueue.
type Runner struct {
	workers  int
	store    Store
	elector  Elector
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
	if opts.Elector == nil {
		opts.Elector = AlwaysLeader{}
	}

	r := &Runner{
		workers:  opts.Workers,
		store:    opts.Store,
		elector:  opts.Elector,
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
			r.wg.Add(1)
			go r.runLocalTicker(ctx, p)
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

	r.wg.Add(1)
	go r.runScheduler(ctx)

	for i := 0; i < r.workers; i++ {
		r.wg.Add(1)
		go r.runDispatcher(ctx)
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
	return r.elector.Close(ctx)
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
	return r.store.Enqueue(ctx, run)
}

// runLocalTicker drives an all-nodes periodic job on this replica. It never
// touches the store.
func (r *Runner) runLocalTicker(ctx context.Context, p Periodic) {
	defer r.wg.Done()

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

// runScheduler enqueues due leader-scoped periodic runs while this replica is
// the leader.
func (r *Runner) runScheduler(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.elector.IsLeader() {
				continue
			}
			due, err := r.store.DueScheduled(ctx, time.Now())
			if err != nil {
				r.log.Error().Err(err).Msg("rjobs: querying due jobs failed")
				continue
			}
			for _, d := range due {
				if _, err := r.store.Enqueue(ctx, Run{Job: d.Job}); err != nil {
					r.log.Error().Err(err).Str("job", d.Job).Msg("rjobs: enqueuing periodic run failed")
				}
			}
		}
	}
}

// runDispatcher claims runs from the store and executes them.
func (r *Runner) runDispatcher(ctx context.Context) {
	defer r.wg.Done()

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

// execRun runs a claimed durable run (on-demand or leader periodic) and
// acks/naks it.
func (r *Runner) execRun(ctx context.Context, run Run) {
	log := r.log.With().Str("job", run.Job).Str("run", string(run.ID)).Int("attempt", run.Attempt).Logger()

	err := r.invoke(ctx, run, log)
	if err != nil {
		log.Error().Err(err).Msg("rjobs: run failed")
		if ferr := r.store.Fail(ctx, run.ID, defaultRetryAfter); ferr != nil {
			log.Error().Err(ferr).Msg("rjobs: marking run failed errored")
		}
		return
	}
	if cerr := r.store.Complete(ctx, run.ID); cerr != nil {
		log.Error().Err(cerr).Msg("rjobs: completing run errored")
	}
}

// invoke dispatches a run to the right registered job. Periodic runs (empty
// Params and a known periodic name) call the periodic Run closure; everything
// else is an on-demand job.
func (r *Runner) invoke(ctx context.Context, run Run, log zerolog.Logger) error {
	jobCtx := appctx.WithLogger(ctx, &log)

	if p, ok := r.lookupPeriodic(run.Job); ok {
		return r.guard(p, func() error { return p.Run(jobCtx) })
	}

	newFunc, ok := lookupOnDemand(run.Job)
	if !ok {
		return errors.Errorf("rjobs: run references unknown job %q", run.Job)
	}
	job, err := newFunc(jobCtx, nil)
	if err != nil {
		return errors.Wrap(err, "rjobs: building on-demand job failed")
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

// jitter returns a random non-negative duration in [0, max).
func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}
