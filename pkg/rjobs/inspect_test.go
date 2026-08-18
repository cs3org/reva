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
	"testing"
	"time"
)

// TestInspect checks the runner's live self-report of workers, in-flight
// periodics and executing runs — the state the store does not hold.
func TestInspect(t *testing.T) {
	r := &Runner{
		workers: 3,
		running: map[string]bool{},
		cancels: map[RunID]*runHandle{},
	}

	if info := r.Inspect(); info.Workers != 3 || info.Busy != 0 || info.StoreWired || len(info.Active) != 0 {
		t.Fatalf("idle runner: %+v", info)
	}

	r.registerRun("r1", &runHandle{job: "j1", started: time.Now()})
	r.registerRun("r2", &runHandle{job: "j2", started: time.Now()})
	r.running["periodic.x"] = true
	r.running["periodic.y"] = false // not in flight

	info := r.Inspect()
	if info.Busy != 2 || len(info.Active) != 2 {
		t.Fatalf("expected 2 active runs, got %+v", info.Active)
	}
	if len(info.InFlightPeriodic) != 1 || info.InFlightPeriodic[0] != "periodic.x" {
		t.Fatalf("expected only periodic.x in flight, got %+v", info.InFlightPeriodic)
	}
}
