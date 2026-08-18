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
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestRecordStatusKeepsStartedAt guards against the terminal write blanking the
// start time: recordStatus upserts the whole row, so it must carry StartedAt
// forward from the running write, not just set FinishedAt.
func TestRecordStatusKeepsStartedAt(t *testing.T) {
	status := newFakeStatus()
	r := &Runner{status: status}
	run := Run{ID: "r-startedat", Job: "test.startedat", EnqueuedAt: time.Now()}
	started := time.Now().Truncate(time.Second)

	r.recordStatus(context.Background(), run, StateRunning, nil, nil, started, zerolog.Nop())
	r.recordStatus(context.Background(), run, StateSucceeded, Params{"ok": true}, nil, started, zerolog.Nop())

	got, err := status.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != StateSucceeded {
		t.Fatalf("state = %q, want succeeded", got.State)
	}
	if got.StartedAt == nil {
		t.Fatal("StartedAt was blanked on the terminal write")
	}
	if !got.StartedAt.Equal(started) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, started)
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt not set on the terminal write")
	}
}
