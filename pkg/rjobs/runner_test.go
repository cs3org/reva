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
	"sync/atomic"
	"testing"
	"time"
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
func (stubStore) DueScheduled(context.Context, time.Time) ([]ScheduledRun, error) {
	return nil, nil
}
func (stubStore) RegisterScheduled(context.Context, string, Schedule, time.Time) error {
	return nil
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

func resetRegistry() {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.periodic = make(map[string]Periodic)
	reg.onDemand = make(map[string]NewJob)
}
