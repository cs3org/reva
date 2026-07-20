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
	"sort"
	"time"
)

// JobInfo describes a registered job: on-demand jobs carry only a name; periodic
// jobs also carry their schedule, scope and overlap policy.
type JobInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`               // "periodic" | "on-demand"
	Schedule string `json:"schedule,omitempty"` // periodic only
	Scope    string `json:"scope,omitempty"`    // periodic only: "leader" | "all-nodes"
	Overlap  string `json:"overlap,omitempty"`  // periodic only: "skip" | "allow"
}

// ActiveRun is one run executing on this process right now.
type ActiveRun struct {
	RunID   RunID     `json:"run_id"`
	Job     string    `json:"job"`
	Started time.Time `json:"started"`
}

// RunnerInfo is a runner's live self-report: what it knows and what it is doing,
// held in memory and mostly absent from any store. It is the payload the Admin
// API fans out to answer "what jobs are there and what is going on".
type RunnerInfo struct {
	// Workers is the worker-pool size; Busy is how many are currently running a
	// queued (on-demand or leader) run.
	Workers int `json:"workers"`
	Busy    int `json:"busy"`
	// StoreWired reports whether a durable queue/status store is configured, i.e.
	// whether this runner does on-demand and leader work or only all-nodes
	// tickers.
	StoreWired bool `json:"store_wired"`
	// Jobs are the jobs registered on this runner.
	Jobs []JobInfo `json:"jobs"`
	// InFlightPeriodic are the periodic jobs (any scope) executing here right now,
	// by name — including all-nodes jobs that never touch the store.
	InFlightPeriodic []string `json:"in_flight_periodic"`
	// Active are the queued runs (on-demand / leader) executing here right now.
	Active []ActiveRun `json:"active"`
}

// Inspect returns a snapshot of this runner's live in-memory state, read under
// the runner's own locks. It reflects reality — a run shows here only while this
// process is actually executing it — where the durable status can lag.
func (r *Runner) Inspect() RunnerInfo {
	info := RunnerInfo{
		Workers:    r.workers,
		StoreWired: r.store != nil,
		Jobs:       registeredJobs(),
	}

	r.cancelsMu.Lock()
	for id, h := range r.cancels {
		info.Active = append(info.Active, ActiveRun{RunID: id, Job: h.job, Started: h.started})
	}
	r.cancelsMu.Unlock()
	info.Busy = len(info.Active)
	sort.Slice(info.Active, func(i, j int) bool { return info.Active[i].Started.Before(info.Active[j].Started) })

	r.runningMu.Lock()
	for name, running := range r.running {
		if running {
			info.InFlightPeriodic = append(info.InFlightPeriodic, name)
		}
	}
	r.runningMu.Unlock()
	sort.Strings(info.InFlightPeriodic)

	return info
}

// registeredJobs enumerates the process's registered jobs with their metadata.
func registeredJobs() []JobInfo {
	var out []JobInfo
	for _, p := range registeredPeriodic() {
		out = append(out, JobInfo{
			Name:     p.Name,
			Kind:     "periodic",
			Schedule: p.Schedule,
			Scope:    p.Scope.String(),
			Overlap:  overlapString(p.Overlap),
		})
	}
	for _, name := range registeredOnDemandNames() {
		out = append(out, JobInfo{Name: name, Kind: "on-demand"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func overlapString(o OverlapPolicy) string {
	if o == Allow {
		return "allow"
	}
	return "skip"
}

// ListRuns returns the runs in the durable status store matching the filter,
// across all owners (the admin's fleet-wide view; ListByOwner pins the owner).
func (r *Runner) ListRuns(ctx context.Context, f ListFilter) ([]Status, error) {
	if r.status == nil {
		return nil, errors.New("rjobs: no status store configured")
	}
	return r.status.List(ctx, f)
}
