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
func (stubStore) RegisterScheduled(context.Context, string, Schedule, time.Time) error {
	return nil
}
func (stubStore) MarkScheduledRunning(context.Context, string) error  { return nil }
func (stubStore) ClearScheduledRunning(context.Context, string) error { return nil }
func (stubStore) Close(context.Context) error                         { return nil }

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
	}
	f.rec[s.RunID] = s
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
	if s.State == StateSucceeded || s.State == StateCancelled {
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
func (s *oneRunStore) Fail(context.Context, RunID, time.Duration) error {
	s.failed.Store(true)
	return nil
}
func (s *oneRunStore) Heartbeat(context.Context, RunID) error { return nil }
func (s *oneRunStore) HeartbeatInterval() time.Duration       { return 20 * time.Millisecond }
func (s *oneRunStore) DueScheduled(context.Context, time.Time) ([]ScheduledRun, error) {
	return nil, nil
}
func (s *oneRunStore) RegisterScheduled(context.Context, string, Schedule, time.Time) error {
	return nil
}
func (s *oneRunStore) MarkScheduledRunning(context.Context, string) error  { return nil }
func (s *oneRunStore) ClearScheduledRunning(context.Context, string) error { return nil }
func (s *oneRunStore) Close(context.Context) error                         { return nil }

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
