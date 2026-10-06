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
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestAllNodesRunOnStart(t *testing.T) {
	resetRegistry()

	var runs int32
	done := make(chan struct{}, 1)
	err := RegisterPeriodic(Periodic{
		Name:       "test.warm",
		Schedule:   "@every 1h",
		Scope:      ScopeAllNodes,
		RunOnStart: true,
		Run: func(ctx context.Context) error {
			if atomic.AddInt32(&runs, 1) == 1 {
				done <- struct{}{}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewRunner(context.Background(), Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnStart job did not run")
	}
}

func TestLeaderJobNeedsStore(t *testing.T) {
	resetRegistry()

	if err := RegisterPeriodic(Periodic{
		Name:     "test.cleanup",
		Schedule: "@every 1h",
		Scope:    ScopeLeader,
		Run:      func(ctx context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := NewRunner(context.Background(), Options{}); err == nil {
		t.Fatal("expected error: leader job without a store")
	}
}

func TestStoreRequiresStatusStore(t *testing.T) {
	resetRegistry()

	if _, err := NewRunner(context.Background(), Options{Store: stubStore{}}); err == nil {
		t.Fatal("expected error: store configured without a status store")
	}
}

// stubStore is a minimal Store used only to exercise runner construction.
type stubStore struct{}

func (stubStore) Enqueue(context.Context, Run) (RunID, error)      { return "", nil }
func (stubStore) Claim(context.Context) (Run, error)               { return Run{}, nil }
func (stubStore) Complete(context.Context, RunID) error            { return nil }
func (stubStore) Fail(context.Context, RunID, time.Duration) error { return nil }
func (stubStore) Heartbeat(context.Context, RunID) error           { return nil }
func (stubStore) HeartbeatInterval() time.Duration                 { return 0 }
func (stubStore) DueScheduled(context.Context, time.Time) ([]ScheduledRun, error) {
	return nil, nil
}
func (stubStore) RegisterScheduled(context.Context, string, Schedule, time.Time, bool) error {
	return nil
}
func (stubStore) MarkScheduledRunning(context.Context, string) error  { return nil }
func (stubStore) ClearScheduledRunning(context.Context, string) error { return nil }
func (stubStore) TryMarkScheduledRunning(context.Context, string) (bool, error) {
	return false, nil
}
func (stubStore) RequestCancelScheduled(context.Context, string) (bool, error) {
	return false, nil
}
func (stubStore) ScheduledCancelRequested(context.Context, string) (bool, error) {
	return false, nil
}
func (stubStore) Close(context.Context) error { return nil }

func TestGuardSkipsOverlap(t *testing.T) {
	r := &Runner{running: make(map[string]bool)}
	p := Periodic{Name: "j", Overlap: Skip}

	block := make(chan struct{})
	go func() {
		_ = r.guard(p, func() error {
			<-block
			return nil
		})
	}()

	// wait for the first run to mark itself running
	for {
		r.runningMu.Lock()
		running := r.running["j"]
		r.runningMu.Unlock()
		if running {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if err := r.guard(p, func() error { return nil }); err != errSkippedOverlap {
		t.Fatalf("expected overlap skip, got %v", err)
	}
	close(block)
}

func TestIsLeaderJob(t *testing.T) {
	r := &Runner{
		periodic: []Periodic{
			{Name: "cleanup", Scope: ScopeLeader},
			{Name: "warm", Scope: ScopeAllNodes},
		},
	}

	if !r.isLeaderJob("cleanup") {
		t.Error("a registered leader job should be reported as such")
	}
	// a job that flipped to all-nodes must not be treated as leader, so a
	// stale schedule entry for it is skipped instead of double-running.
	if r.isLeaderJob("warm") {
		t.Error("an all-nodes job must not be reported as a leader job")
	}
	// a job no longer registered at all (deleted/renamed) is not a leader job.
	if r.isLeaderJob("gone") {
		t.Error("an unregistered job must not be reported as a leader job")
	}
}

// funcJob adapts a plain function to the Job interface for tests.
type funcJob func(ctx context.Context, p Params) (Params, error)

func (f funcJob) Run(ctx context.Context, p Params) (Params, error) { return f(ctx, p) }

func TestInvokePassesOnDemandConfig(t *testing.T) {
	resetRegistry()

	got := make(map[string]map[string]any)
	register := func(name string) {
		err := RegisterOnDemand(name, func(ctx context.Context, m map[string]any) (Job, error) {
			got[name] = m
			return funcJob(func(ctx context.Context, p Params) (Params, error) { return nil, nil }), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	register("test.configured")
	register("test.bare")

	r, err := NewRunner(context.Background(), Options{
		OnDemandConfig: map[string]map[string]any{
			"test.configured": {"greeting": "hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.invoke(context.Background(), Run{Job: "test.configured"}, nil, r.log); err != nil {
		t.Fatal(err)
	}
	if _, err := r.invoke(context.Background(), Run{Job: "test.bare"}, nil, r.log); err != nil {
		t.Fatal(err)
	}

	if g := got["test.configured"]["greeting"]; g != "hello" {
		t.Errorf("configured job did not receive its config section: got %v", got["test.configured"])
	}
	// a job with no config section is built with a nil map, not an empty one,
	// so it can tell "unset" apart and rely on its own defaults.
	if got["test.bare"] != nil {
		t.Errorf("a job without a config section should receive nil, got %v", got["test.bare"])
	}
}

func resetRegistry() {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.periodic = make(map[string]Periodic)
	reg.onDemand = make(map[string]NewJob)
}

// jobFunc adapts a function to the Job interface for tests.
type jobFunc func(context.Context, Params) (Params, error)

func (f jobFunc) Run(ctx context.Context, p Params) (Params, error) { return f(ctx, p) }

// fakeStatus is an in-memory StatusStore. Like the real store, a Put never
// clears the cancel intent: only RequestCancel owns it.
type fakeStatus struct {
	mu  sync.Mutex
	rec map[RunID]Status
}

func newFakeStatus() *fakeStatus { return &fakeStatus{rec: make(map[RunID]Status)} }

func (f *fakeStatus) Put(_ context.Context, s Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cur, ok := f.rec[s.RunID]; ok {
		s.CancelRequested = cur.CancelRequested
		s.Progress, s.ProgressAt = cur.Progress, cur.ProgressAt
	}
	f.rec[s.RunID] = s
	return nil
}

// PutProgress, like the real store, only touches a run that is not terminal.
func (f *fakeStatus) PutProgress(_ context.Context, id RunID, p Progress, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rec[id]
	if !ok || s.State.Terminal() {
		return nil
	}
	s.Progress, s.ProgressAt = &p, &at
	f.rec[id] = s
	return nil
}

func (f *fakeStatus) Get(_ context.Context, id RunID) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rec[id]
	if !ok {
		return Status{}, errtypes.NotFound(string(id))
	}
	return s, nil
}

func (f *fakeStatus) RequestCancel(_ context.Context, id RunID) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rec[id]
	if !ok {
		return Status{}, errtypes.NotFound(string(id))
	}
	if s.State.Terminal() {
		return s, nil
	}
	s.CancelRequested = true
	s.State = StateCancelling
	f.rec[id] = s
	return s, nil
}

// List, Reserve and Release are part of the StatusStore interface but are not
// exercised by the cancellation tests; minimal implementations keep the fake
// satisfying the interface.
func (f *fakeStatus) List(context.Context, ListFilter) ([]Status, error) { return nil, nil }

func (f *fakeStatus) Reserve(_ context.Context, s Status, _ string) (Status, bool, error) {
	if err := f.Put(context.Background(), s); err != nil {
		return Status{}, false, err
	}
	return s, true, nil
}

func (f *fakeStatus) Release(context.Context, RunID) error { return nil }

func (f *fakeStatus) Close(context.Context) error { return nil }

// oneRunStore hands out exactly one run, then blocks Claim until shutdown. It
// records whether the run was Completed (acked) or Failed (retried).
type oneRunStore struct {
	run       Run
	claimed   atomic.Bool
	completed atomic.Bool
	failed    atomic.Bool
	failDelay atomic.Int64
}

func (s *oneRunStore) Enqueue(_ context.Context, r Run) (RunID, error) { return r.ID, nil }

func (s *oneRunStore) Claim(ctx context.Context) (Run, error) {
	if s.claimed.CompareAndSwap(false, true) {
		return s.run, nil
	}
	<-ctx.Done()
	return Run{}, ctx.Err()
}

func (s *oneRunStore) Complete(context.Context, RunID) error { s.completed.Store(true); return nil }
func (s *oneRunStore) Fail(_ context.Context, _ RunID, d time.Duration) error {
	s.failDelay.Store(int64(d))
	s.failed.Store(true)
	return nil
}
func (s *oneRunStore) Heartbeat(context.Context, RunID) error { return nil }
func (s *oneRunStore) HeartbeatInterval() time.Duration       { return 20 * time.Millisecond }
func (s *oneRunStore) DueScheduled(context.Context, time.Time) ([]ScheduledRun, error) {
	return nil, nil
}
func (s *oneRunStore) RegisterScheduled(context.Context, string, Schedule, time.Time, bool) error {
	return nil
}
func (s *oneRunStore) MarkScheduledRunning(context.Context, string) error  { return nil }
func (s *oneRunStore) ClearScheduledRunning(context.Context, string) error { return nil }
func (s *oneRunStore) TryMarkScheduledRunning(context.Context, string) (bool, error) {
	return false, nil
}
func (s *oneRunStore) RequestCancelScheduled(context.Context, string) (bool, error) {
	return false, nil
}
func (s *oneRunStore) ScheduledCancelRequested(context.Context, string) (bool, error) {
	return false, nil
}
func (s *oneRunStore) Close(context.Context) error { return nil }

func TestCancelStopsRunningJob(t *testing.T) {
	resetRegistry()

	started := make(chan struct{})
	if err := RegisterOnDemand("test.cancelme", func(context.Context, map[string]any) (Job, error) {
		return jobFunc(func(ctx context.Context, _ Params) (Params, error) {
			close(started)
			<-ctx.Done() // cooperative: stop as soon as the run is cancelled
			return nil, ctx.Err()
		}), nil
	}); err != nil {
		t.Fatal(err)
	}

	status := newFakeStatus()
	store := &oneRunStore{run: Run{ID: "run-1", Job: "test.cancelme", Attempt: 1}}
	_ = status.Put(context.Background(), Status{RunID: "run-1", Job: "test.cancelme", State: StateQueued})

	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}

	if _, err := r.Cancel(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	for {
		got, _ := status.Get(context.Background(), "run-1")
		if got.State == StateCancelled {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("run did not reach cancelled, last state %q", got.State)
		case <-time.After(5 * time.Millisecond):
		}
	}

	if !store.completed.Load() {
		t.Error("a cancelled run must be acked via Complete")
	}
	if store.failed.Load() {
		t.Error("a cancelled run must not be failed/retried")
	}
}

// busStore is a oneRunStore that also implements ControlBus, delivering a
// published cancel straight to the subscribed handler (loopback), so a test can
// simulate a cancel issued on another process.
type busStore struct {
	oneRunStore
	mu      sync.Mutex
	handler func(CancelSignal)
}

func (s *busStore) PublishCancel(_ context.Context, sig CancelSignal) error {
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	if h != nil {
		h(sig)
	}
	return nil
}

func (s *busStore) SubscribeCancel(_ context.Context, handler func(CancelSignal)) error {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
	return nil
}

func TestCancelBroadcastStopsRemoteRun(t *testing.T) {
	resetRegistry()

	started := make(chan struct{})
	if err := RegisterOnDemand("test.remote", func(context.Context, map[string]any) (Job, error) {
		return jobFunc(func(ctx context.Context, _ Params) (Params, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}), nil
	}); err != nil {
		t.Fatal(err)
	}

	status := newFakeStatus()
	store := &busStore{oneRunStore: oneRunStore{run: Run{ID: "run-1", Job: "test.remote", Attempt: 1}}}
	_ = status.Put(context.Background(), Status{RunID: "run-1", Job: "test.remote", State: StateQueued})

	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}

	// Simulate a cancel issued on another process: it reaches us only via the
	// bus, not through this runner's Cancel.
	if err := store.PublishCancel(context.Background(), CancelSignal{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	for {
		got, _ := status.Get(context.Background(), "run-1")
		if got.State == StateCancelled {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("run did not reach cancelled, last state %q", got.State)
		case <-time.After(5 * time.Millisecond):
		}
	}
	if !store.completed.Load() {
		t.Error("a broadcast-cancelled run must be acked via Complete")
	}
}

// triggerStore records what TriggerNow does: whether it took the single-flight
// gate and what it enqueued.
type triggerStore struct {
	stubStore
	mu       sync.Mutex
	acquire  bool
	enqueued []string
}

func (s *triggerStore) TryMarkScheduledRunning(context.Context, string) (bool, error) {
	return s.acquire, nil
}

func (s *triggerStore) Enqueue(_ context.Context, run Run) (RunID, error) {
	s.mu.Lock()
	s.enqueued = append(s.enqueued, run.Job)
	s.mu.Unlock()
	return "rid", nil
}

func TestTriggerNow(t *testing.T) {
	resetRegistry()
	if err := RegisterPeriodic(Periodic{
		Name: "test.cleanup", Schedule: "@every 1h", Scope: ScopeLeader,
		Run: func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}

	store := &triggerStore{acquire: true}
	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: newFakeStatus()})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.TriggerNow(context.Background(), "test.cleanup"); err != nil {
		t.Fatal(err)
	}
	if len(store.enqueued) != 1 || store.enqueued[0] != "test.cleanup" {
		t.Fatalf("expected one enqueue of test.cleanup, got %v", store.enqueued)
	}

	// With a run already in flight the gate is not acquired: trigger is rejected
	// and enqueues nothing.
	store.acquire = false
	if err := r.TriggerNow(context.Background(), "test.cleanup"); err == nil {
		t.Error("expected an error triggering a job already in flight")
	}
	if len(store.enqueued) != 1 {
		t.Errorf("a rejected trigger must not enqueue, got %v", store.enqueued)
	}

	// On-demand and unregistered jobs cannot be triggered.
	if err := r.TriggerNow(context.Background(), "nope"); err == nil {
		t.Error("expected an error triggering an unregistered job")
	}
}

// periodicStore delivers one leader-periodic run and tracks the schedule's
// in-flight and cancel state by job name, like the real KV-backed store.
type periodicStore struct {
	run       Run
	claimed   atomic.Bool
	completed atomic.Bool
	mu        sync.Mutex
	running   bool
	cancelReq bool
}

func (s *periodicStore) Enqueue(_ context.Context, r Run) (RunID, error) { return r.ID, nil }

func (s *periodicStore) Claim(ctx context.Context) (Run, error) {
	if s.claimed.CompareAndSwap(false, true) {
		s.mu.Lock()
		s.running = true
		s.mu.Unlock()
		return s.run, nil
	}
	<-ctx.Done()
	return Run{}, ctx.Err()
}

func (s *periodicStore) Complete(context.Context, RunID) error            { s.completed.Store(true); return nil }
func (s *periodicStore) Fail(context.Context, RunID, time.Duration) error { return nil }
func (s *periodicStore) Heartbeat(context.Context, RunID) error           { return nil }
func (s *periodicStore) HeartbeatInterval() time.Duration                 { return 20 * time.Millisecond }
func (s *periodicStore) DueScheduled(context.Context, time.Time) ([]ScheduledRun, error) {
	return nil, nil
}
func (s *periodicStore) RegisterScheduled(context.Context, string, Schedule, time.Time, bool) error {
	return nil
}
func (s *periodicStore) MarkScheduledRunning(context.Context, string) error { return nil }
func (s *periodicStore) TryMarkScheduledRunning(context.Context, string) (bool, error) {
	return false, nil
}
func (s *periodicStore) ClearScheduledRunning(context.Context, string) error {
	s.mu.Lock()
	s.running = false
	s.cancelReq = false
	s.mu.Unlock()
	return nil
}

func (s *periodicStore) RequestCancelScheduled(context.Context, string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return false, nil
	}
	s.cancelReq = true
	return true, nil
}

func (s *periodicStore) ScheduledCancelRequested(context.Context, string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelReq, nil
}

func (s *periodicStore) inFlight() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *periodicStore) Close(context.Context) error { return nil }

func TestCancelPeriodicStopsInFlightRun(t *testing.T) {
	resetRegistry()

	started := make(chan struct{})
	if err := RegisterPeriodic(Periodic{
		Name: "test.periodic", Schedule: "@every 1h", Scope: ScopeLeader,
		Run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done() // cooperative: stop when the run is cancelled
			return ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}

	store := &periodicStore{run: Run{ID: "run-1", Job: "test.periodic", Attempt: 1}}
	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: newFakeStatus()})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("periodic job did not start")
	}

	if err := r.CancelPeriodic(context.Background(), "test.periodic"); err != nil {
		t.Fatal(err)
	}

	// Wait until the run has fully wound down: the in-flight mark is cleared by
	// the last deferred step of execRun, so this also implies the run was acked.
	deadline := time.After(2 * time.Second)
	for store.inFlight() {
		select {
		case <-deadline:
			t.Fatal("cancelled periodic run did not wind down")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if !store.completed.Load() {
		t.Fatal("cancelled periodic run was not acked via Complete")
	}

	// With nothing in flight, a second cancel is an error, not a silent no-op.
	if err := r.CancelPeriodic(context.Background(), "test.periodic"); err == nil {
		t.Error("expected an error cancelling a job with no run in flight")
	}
}

// knownStore answers KnownJobs from a fixed set and counts the lookups.
type knownStore struct {
	triggerStore
	jobs    map[string]bool
	lookups atomic.Int32
}

func (s *knownStore) OnDemandJobKnown(_ context.Context, job string) (bool, error) {
	s.lookups.Add(1)
	return s.jobs[job], nil
}

func TestEnqueueJobRegisteredElsewhere(t *testing.T) {
	resetRegistry()

	store := &knownStore{jobs: map[string]bool{"remote.copy": true}}
	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: newFakeStatus()})
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := r.Enqueue(context.Background(), "remote.copy", nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := store.lookups.Load(); n != 1 {
		t.Errorf("a known job should be cached, got %d lookups", n)
	}

	_, err = r.Enqueue(context.Background(), "remote.typo", nil)
	if _, ok := err.(errtypes.IsNotFound); !ok {
		t.Errorf("expected NotFound for a job no runner registered, got %v", err)
	}
}

// progressJob reports a few snapshots and returns.
type progressJob struct{}

func (progressJob) Run(context.Context, Params) (Params, error) { return nil, nil }

func (progressJob) RunWithProgress(_ context.Context, _ Params, r Reporter) (Params, error) {
	details := Params{}
	for i := range 3 {
		details["file"] = i
		r.Report(Progress{Phase: "copy", Done: int64(i + 1), Total: 3, Details: details})
	}
	details["file"] = "changed after report"
	return nil, nil
}

func TestProgressFinalSnapshot(t *testing.T) {
	resetRegistry()
	if err := RegisterOnDemand("test.progress", func(context.Context, map[string]any) (Job, error) {
		return progressJob{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	status := newFakeStatus()
	store := &oneRunStore{run: Run{ID: "run-1", Job: "test.progress", Attempt: 1}}
	_ = status.Put(context.Background(), Status{RunID: "run-1", Job: "test.progress", State: StateQueued})

	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: status, ProgressInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	deadline := time.After(2 * time.Second)
	for !store.completed.Load() {
		select {
		case <-deadline:
			t.Fatal("run did not complete")
		case <-time.After(5 * time.Millisecond):
		}
	}

	got, _ := status.Get(context.Background(), "run-1")
	if got.State != StateSucceeded {
		t.Fatalf("state = %q, want succeeded", got.State)
	}
	// the final snapshot is written before the terminal status, even though the
	// interval never ticked.
	if got.Progress == nil || got.Progress.Done != 3 {
		t.Fatalf("final progress not recorded: %+v", got.Progress)
	}
	if got.Progress.Details["file"] != 2 {
		t.Errorf("reported details must be copied, got %v", got.Progress.Details)
	}
}

// runOnce runs a single on-demand run of job through a runner and returns its
// final status once the store acked or naked it.
func runOnce(t *testing.T, job Job, run Run) (Status, *oneRunStore) {
	t.Helper()
	resetRegistry()
	if err := RegisterOnDemand(run.Job, func(context.Context, map[string]any) (Job, error) { return job, nil }); err != nil {
		t.Fatal(err)
	}
	status := newFakeStatus()
	store := &oneRunStore{run: run}
	_ = status.Put(context.Background(), Status{RunID: run.ID, Job: run.Job, State: StateQueued})

	r, err := NewRunner(context.Background(), Options{Workers: 1, Store: store, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	r.Start()
	defer r.Stop(context.Background())

	deadline := time.After(2 * time.Second)
	for !store.completed.Load() && !store.failed.Load() {
		select {
		case <-deadline:
			t.Fatal("run did not finish")
		case <-time.After(5 * time.Millisecond):
		}
	}
	st, _ := status.Get(context.Background(), run.ID)
	return st, store
}

func TestRetryAfter(t *testing.T) {
	job := jobFunc(func(context.Context, Params) (Params, error) {
		return nil, RetryAfter(5*time.Second, errors.New("busy"))
	})
	st, store := runOnce(t, job, Run{ID: "run-1", Job: "test.retry", Attempt: 2})
	if !store.failed.Load() || time.Duration(store.failDelay.Load()) != 5*time.Second {
		t.Errorf("expected a nak after 5s, got failed=%v delay=%v", store.failed.Load(), time.Duration(store.failDelay.Load()))
	}
	if st.State != StateQueued || st.LastError != "busy" || st.Attempt != 2 {
		t.Errorf("unexpected status %+v", st)
	}
}

func TestPermanent(t *testing.T) {
	job := jobFunc(func(context.Context, Params) (Params, error) {
		return nil, Permanent(errors.New("source is gone"))
	})
	st, store := runOnce(t, job, Run{ID: "run-1", Job: "test.permanent", Attempt: 1})
	if !store.completed.Load() || store.failed.Load() {
		t.Error("an aborted run must be acked, not retried")
	}
	if st.State != StateAborted || st.FinishedAt == nil || st.LastError != "source is gone" {
		t.Errorf("unexpected status %+v", st)
	}
}
