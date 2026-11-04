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

package reminders

import (
	"context"
	"fmt"
	"time"

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/reminders"
)

const defaultLimit = 100

// Invocations implements invoke.Invokable.
func (s *svc) Invocations() []invoke.InvocationSpec { return s.set.Invocations() }

// Invoke implements invoke.Invokable.
func (s *svc) Invoke(ctx context.Context, name string, args map[string]any) (invoke.Result, error) {
	return s.set.Invoke(ctx, name, args)
}

type filterArgs struct {
	Kind      string    `arg:"kind" help:"only this kind"`
	Subject   string    `arg:"subject" help:"only this subject"`
	Recipient string    `arg:"recipient" help:"only this recipient"`
	Since     time.Time `arg:"since" help:"history: only after this time"`
	N         int       `arg:"n" help:"at most this many, default 100"`
}

type checkArgs struct {
	Kind string `arg:"kind,required" help:"the kind"`
	N    int    `arg:"n" help:"show at most this many items, default 100"`
}

type forgetArgs struct {
	Kind    string `arg:"kind,required" help:"the kind"`
	Subject string `arg:"subject,required" help:"the subject"`
}

func (s *svc) invocations() *invoke.Set {
	set := invoke.NewSet()
	set.Add("kinds", "The configured kinds: schedule, open episodes, reminders sent and last run.").
		Handle(invoke.Typed(s.kinds))
	set.Add("state", "The open episodes.").
		Handle(invoke.Typed(func(ctx context.Context, a filterArgs) (invoke.Result, error) {
			episodes, err := s.ledger.Episodes(ctx, a.filter())
			return invoke.Result{"episodes": episodes}, err
		}))
	set.Add("history", "The reminders sent, the most recent first.").
		Handle(invoke.Typed(func(ctx context.Context, a filterArgs) (invoke.Result, error) {
			sent, err := s.ledger.History(ctx, a.filter())
			return invoke.Result{"sent": sent}, err
		}))
	set.Add("check", "Scan a kind now and show what a run would send. Changes nothing.").
		Handle(invoke.Typed(func(ctx context.Context, a checkArgs) (invoke.Result, error) {
			r, ok := s.runners[a.Kind]
			if !ok {
				return nil, fmt.Errorf("kind %q is not configured", a.Kind)
			}
			report, due, err := r.Check(ctx, limit(a.N))
			items := make([]map[string]any, 0, len(due))
			for _, d := range due {
				items = append(items, map[string]any{"subject": d.Subject, "level": d.Level, "level_sent": d.LevelSent, "recipient": d.Recipient.Name, "recipient_group": d.Recipient.Group})
			}
			res := invoke.Result{"report": report, "due": items}
			if err != nil {
				res["error"] = err.Error()
			}
			return res, nil
		}))
	set.Add("forget", "Delete an episode, so that its subject is told again from the lowest level.").
		Mutating().
		Handle(invoke.Typed(func(ctx context.Context, a forgetArgs) (invoke.Result, error) {
			n, err := s.ledger.Forget(ctx, a.Kind, a.Subject)
			return invoke.Result{"forgotten": n}, err
		}))
	return set
}

func (s *svc) kinds(ctx context.Context, _ struct{}) (invoke.Result, error) {
	runs, err := s.ledger.Runs(ctx)
	if err != nil {
		return nil, err
	}
	last := map[string]reminders.RunRecord{}
	for _, r := range runs {
		last[r.Kind] = r
	}
	out := make([]map[string]any, 0, len(s.names))
	for _, name := range s.names {
		sum, err := s.ledger.Summary(ctx, name, time.Now())
		if err != nil {
			return nil, err
		}
		k := map[string]any{"name": name, "event": s.runners[name].Kind.Event(), "schedule": s.configs[name].Schedule, "dry_run": s.configs[name].DryRun, "summary": sum}
		if r, ok := last[name]; ok {
			k["last_run"] = r
		}
		out = append(out, k)
	}
	return invoke.Result{"kinds": out}, nil
}

func (a filterArgs) filter() reminders.Filter {
	return reminders.Filter{Kind: a.Kind, Subject: a.Subject, Recipient: a.Recipient, Since: a.Since, Limit: limit(a.N)}
}

func limit(n int) int {
	if n > 0 {
		return n
	}
	return defaultLimit
}
