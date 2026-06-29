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

package sql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rjobs"
)

func newTestStore(t *testing.T) rjobs.StatusStore {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "jobs.db")
	s, err := New(context.Background(), config.Database{Engine: "sqlite", DBName: dbFile})
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

func TestPutAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.Put(ctx, rjobs.Status{
		RunID:      "run-1",
		Job:        "example.pingpong",
		State:      rjobs.StateQueued,
		Attempt:    1,
		EnqueuedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	// upsert: move to succeeded with a result.
	started := now.Add(time.Second)
	finished := now.Add(2 * time.Second)
	if err := s.Put(ctx, rjobs.Status{
		RunID:      "run-1",
		Job:        "example.pingpong",
		State:      rjobs.StateSucceeded,
		Attempt:    1,
		EnqueuedAt: now,
		StartedAt:  &started,
		FinishedAt: &finished,
		Result:     rjobs.Params{"pong": "pong: hi"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != rjobs.StateSucceeded {
		t.Errorf("state = %q, want succeeded", got.State)
	}
	if got.Result["pong"] != "pong: hi" {
		t.Errorf("result = %v, want pong: hi", got.Result["pong"])
	}
	if got.StartedAt == nil || got.FinishedAt == nil {
		t.Error("started/finished timestamps should be set")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(context.Background(), "missing")
	if _, ok := err.(errtypes.NotFound); !ok {
		t.Errorf("expected NotFound, got %T: %v", err, err)
	}
}

func TestListByOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	alice := "alice"
	bob := "bob"

	put := func(id, owner, job string, state rjobs.State, enqueued time.Time) {
		t.Helper()
		if err := s.Put(ctx, rjobs.Status{
			RunID:      rjobs.RunID(id),
			Job:        job,
			State:      state,
			Attempt:    1,
			EnqueuedAt: enqueued,
			Owner:      owner,
		}); err != nil {
			t.Fatal(err)
		}
	}

	put("a1", alice, "export", rjobs.StateQueued, now)
	put("a2", alice, "report", rjobs.StateSucceeded, now.Add(time.Second))
	put("b1", bob, "export", rjobs.StateQueued, now)
	put("internal", "", "cleanup", rjobs.StateRunning, now)

	// all of alice's runs, most recently enqueued first.
	got, err := s.List(ctx, rjobs.ListFilter{Owner: alice})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("alice runs = %d, want 2", len(got))
	}
	if got[0].RunID != "a2" || got[1].RunID != "a1" {
		t.Errorf("order = %q,%q, want a2,a1", got[0].RunID, got[1].RunID)
	}
	if got[0].Owner != "alice" {
		t.Errorf("owner not round-tripped: %q", got[0].Owner)
	}

	// narrowing by state keeps only the matching run.
	got, err = s.List(ctx, rjobs.ListFilter{Owner: alice, States: []rjobs.State{rjobs.StateQueued}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "a1" {
		t.Fatalf("queued alice runs = %v, want [a1]", got)
	}

	// another user's listing is scoped to their own runs only.
	got, err = s.List(ctx, rjobs.ListFilter{Owner: bob})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "b1" {
		t.Fatalf("bob runs = %v, want [b1]", got)
	}

	// the internal-only filter returns just the no-owner runs.
	got, err = s.List(ctx, rjobs.ListFilter{Internal: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "internal" {
		t.Fatalf("internal runs = %v, want [internal]", got)
	}
}

func TestReserve(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	alice := "alice"
	bob := "bob"
	queued := func(id, owner string) rjobs.Status {
		return rjobs.Status{RunID: rjobs.RunID(id), Job: "export", State: rjobs.StateQueued, Attempt: 1, EnqueuedAt: now, Owner: owner}
	}

	// the first reservation of a key wins.
	if _, reserved, err := s.Reserve(ctx, queued("r1", alice), "export:1"); err != nil || !reserved {
		t.Fatalf("first reservation should win: reserved=%v err=%v", reserved, err)
	}

	// a second one for the same (owner, key) collapses onto the holder.
	existing, reserved, err := s.Reserve(ctx, queued("r2", alice), "export:1")
	if err != nil {
		t.Fatal(err)
	}
	if reserved {
		t.Fatal("second reservation must not win while the key is held")
	}
	if existing.RunID != "r1" {
		t.Errorf("holder = %q, want r1", existing.RunID)
	}

	// a different user holding the same key is independent.
	if _, reserved, err := s.Reserve(ctx, queued("r3", bob), "export:1"); err != nil || !reserved {
		t.Fatalf("a different owner should reserve the same key: reserved=%v err=%v", reserved, err)
	}

	// once released, the key is free for a new run.
	if err := s.Release(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, reserved, err := s.Reserve(ctx, queued("r4", alice), "export:1"); err != nil || !reserved {
		t.Fatalf("a released key should be reservable again: reserved=%v err=%v", reserved, err)
	}
}
