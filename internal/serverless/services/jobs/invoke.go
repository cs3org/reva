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

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/rjobs"
)

// The jobs service is an invoke.Invokable: the Admin API's typed jobs RPCs reach
// it over the control channel through these invocations. They are the
// admin↔runner transport, not a user-facing surface.

func (s *svc) Invocations() []invoke.InvocationSpec { return s.set.Invocations() }

func (s *svc) Invoke(ctx context.Context, name string, args map[string]any) (invoke.Result, error) {
	return s.set.Invoke(ctx, name, args)
}

// buildInvokeSet declares the jobs invocations. Handlers read s.runner at call
// time (it is built in Start), so a call before the runner is ready fails
// cleanly rather than panicking.
func (s *svc) buildInvokeSet() *invoke.Set {
	set := invoke.NewSet()

	set.Add("inspect", "Report this runner's live in-memory state (registered jobs, what is executing, worker pool).").
		Handle(func(_ context.Context, _ invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			return toResult(r.Inspect())
		})

	set.Add("runs", "List durable run history from the store, filtered.").
		OptArg("job", "restrict to this job").
		OptArg("owner", "restrict to this owner").
		OptArg("states", "comma-separated states").
		OptArg("internal", "only internal (ownerless) runs").
		OptArg("limit", "max runs").
		OptArg("offset", "skip this many").
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			runs, err := r.ListRuns(ctx, runFilter(a))
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(runs))
			for _, st := range runs {
				out = append(out, statusMap(st))
			}
			return invoke.Result{"runs": out}, nil
		})

	set.Add("status", "Report one run's full persisted status.").
		Arg("run_id", "the run id").
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			st, err := r.Status(ctx, rjobs.RunID(a.String("run_id")))
			if err != nil {
				return nil, err
			}
			return statusMap(st), nil
		})

	set.Add("enqueue", "Enqueue an on-demand run; returns its run id.").
		Arg("job", "the on-demand job name").
		OptArg("params", "JSON object of parameters").
		OptArg("owner", "attribute the run to this user").
		Mutating().
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			var params rjobs.Params
			if raw := a.String("params"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &params); err != nil {
					return nil, errors.New("jobs: params must be a JSON object")
				}
			}
			var opts []rjobs.EnqueueOption
			if owner := a.String("owner"); owner != "" {
				opts = append(opts, rjobs.WithOwner(owner))
			}
			id, err := r.Enqueue(ctx, a.String("job"), params, opts...)
			if err != nil {
				return nil, err
			}
			return invoke.Result{"run_id": string(id)}, nil
		})

	set.Add("trigger", "Run a leader-scoped periodic job now.").
		Arg("job", "the periodic job name").
		Mutating().
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			if err := r.TriggerNow(ctx, a.String("job")); err != nil {
				return nil, err
			}
			return invoke.Result{"triggered": a.String("job")}, nil
		})

	set.Add("cancel", "Cancel a run by id; returns its updated status.").
		Arg("run_id", "the run id").
		Mutating().
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			st, err := r.Cancel(ctx, rjobs.RunID(a.String("run_id")))
			if err != nil {
				return nil, err
			}
			return statusMap(st), nil
		})

	set.Add("stop", "Cancel a periodic job's in-flight run.").
		Arg("job", "the periodic job name").
		Mutating().
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			if err := r.CancelPeriodic(ctx, a.String("job")); err != nil {
				return nil, err
			}
			return invoke.Result{"stopped": a.String("job")}, nil
		})

	return set
}

// ready returns the runner or a clear error if it has not started (or failed to
// build its store).
func (s *svc) ready() (*rjobs.Runner, error) {
	if s.runner == nil {
		return nil, errors.New("jobs: runner not ready")
	}
	return s.runner, nil
}

// runFilter maps invocation args to a store list filter.
func runFilter(a invoke.Args) rjobs.ListFilter {
	f := rjobs.ListFilter{
		Job:      a.String("job"),
		Owner:    a.String("owner"),
		Internal: a.Bool("internal"),
		Limit:    a.Int("limit"),
		Offset:   a.Int("offset"),
	}
	if states := a.String("states"); states != "" {
		for st := range strings.SplitSeq(states, ",") {
			if st = strings.TrimSpace(st); st != "" {
				f.States = append(f.States, rjobs.State(st))
			}
		}
	}
	return f
}

// toResult renders any JSON-taggable value as an invoke.Result (a map), so the
// control channel serialises it in the shape the Admin API parses.
func toResult(v any) (invoke.Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m invoke.Result
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// statusMap renders a run status with the stable snake_case keys the Admin API
// expects (rjobs.Status carries no JSON tags of its own).
func statusMap(s rjobs.Status) map[string]any {
	m := map[string]any{
		"run_id":           string(s.RunID),
		"job":              s.Job,
		"state":            string(s.State),
		"attempt":          s.Attempt,
		"owner":            s.Owner,
		"enqueued_at":      s.EnqueuedAt.UTC().Format(time.RFC3339),
		"last_error":       s.LastError,
		"cancel_requested": s.CancelRequested,
	}
	if s.StartedAt != nil {
		m["started_at"] = s.StartedAt.UTC().Format(time.RFC3339)
	}
	if s.FinishedAt != nil {
		m["finished_at"] = s.FinishedAt.UTC().Format(time.RFC3339)
	}
	if s.Result != nil {
		m["result"] = s.Result
	}
	return m
}
