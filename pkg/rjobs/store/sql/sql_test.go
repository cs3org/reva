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
