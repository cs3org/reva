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

type runsArgs struct {
	Job      string `arg:"job" help:"restrict to this job"`
	Owner    string `arg:"owner" help:"restrict to this owner"`
	States   string `arg:"states" help:"comma-separated states"`
	Internal bool   `arg:"internal" help:"only internal (ownerless) runs"`
	Limit    int    `arg:"limit" help:"max runs"`
	Offset   int    `arg:"offset" help:"skip this many"`
}

type runArgs struct {
	RunID rjobs.RunID `arg:"run_id,required" help:"the run id"`
}

type enqueueArgs struct {
	Job    string `arg:"job,required" help:"the on-demand job name"`
	Params string `arg:"params" help:"JSON object of parameters"`
	Owner  string `arg:"owner" help:"attribute the run to this user"`
}

type periodicArgs struct {
	Job string `arg:"job,required" help:"the periodic job name"`
}

// buildInvokeSet declares the jobs invocations. Handlers read s.runner at call
// time (it is built in Start), so a call before the runner is ready fails
// cleanly rather than panicking.
func (s *svc) buildInvokeSet() *invoke.Set {
	set := invoke.NewSet()

	set.Add("inspect", "Report this runner's live in-memory state (registered jobs, what is executing, worker pool).").
		Handle(invoke.Typed(func(_ context.Context, _ struct{}) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			return toResult(r.Inspect())
		}))

	set.Add("runs", "List durable run history from the store, filtered.").
		Handle(invoke.Typed(func(ctx context.Context, a runsArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			f := rjobs.ListFilter{Job: a.Job, Owner: a.Owner, Internal: a.Internal, Limit: a.Limit, Offset: a.Offset}
			for st := range strings.SplitSeq(a.States, ",") {
				if st = strings.TrimSpace(st); st != "" {
					f.States = append(f.States, rjobs.State(st))
				}
			}
			runs, err := r.ListRuns(ctx, f)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(runs))
			for _, st := range runs {
				out = append(out, statusMap(st))
			}
			return invoke.Result{"runs": out}, nil
		}))

	set.Add("status", "Report one run's full persisted status.").
		Handle(invoke.Typed(func(ctx context.Context, a runArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			st, err := r.Status(ctx, a.RunID)
			if err != nil {
				return nil, err
			}
			return statusMap(st), nil
		}))

	set.Add("enqueue", "Enqueue an on-demand run; returns its run id.").
		Mutating().
		Handle(invoke.Typed(func(ctx context.Context, a enqueueArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			var params rjobs.Params
			if a.Params != "" {
				if err := json.Unmarshal([]byte(a.Params), &params); err != nil {
					return nil, errors.New("jobs: params must be a JSON object")
				}
			}
			var opts []rjobs.EnqueueOption
			if a.Owner != "" {
				opts = append(opts, rjobs.WithOwner(a.Owner))
			}
			id, err := r.Enqueue(ctx, a.Job, params, opts...)
			if err != nil {
				return nil, err
			}
			return invoke.Result{"run_id": string(id)}, nil
		}))

	set.Add("trigger", "Run a leader-scoped periodic job now.").
		Mutating().
		Handle(invoke.Typed(func(ctx context.Context, a periodicArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			if err := r.TriggerNow(ctx, a.Job); err != nil {
				return nil, err
			}
			return invoke.Result{"triggered": a.Job}, nil
		}))

	set.Add("cancel", "Cancel a run by id; returns its updated status.").
		Mutating().
		Handle(invoke.Typed(func(ctx context.Context, a runArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			st, err := r.Cancel(ctx, a.RunID)
			if err != nil {
				return nil, err
			}
			return statusMap(st), nil
		}))

	set.Add("stop", "Cancel a periodic job's in-flight run.").
		Mutating().
		Handle(invoke.Typed(func(ctx context.Context, a periodicArgs) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			if err := r.CancelPeriodic(ctx, a.Job); err != nil {
				return nil, err
			}
			return invoke.Result{"stopped": a.Job}, nil
		}))

	set.Add("forget", "Drop the queue state of an on-demand job no runner registers anymore.").
		Arg("job", "the job name").
		Mutating().
		Handle(func(ctx context.Context, a invoke.Args) (invoke.Result, error) {
			r, err := s.ready()
			if err != nil {
				return nil, err
			}
			if err := r.ForgetJob(ctx, a.String("job")); err != nil {
				return nil, err
			}
			return invoke.Result{"forgotten": a.String("job")}, nil
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
