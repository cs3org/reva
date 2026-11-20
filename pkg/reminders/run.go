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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/google/uuid"
)

const batchSize = 500

// Runner runs one kind.
type Runner struct {
	// Name is the name of the kind in the configuration and in the ledger.
	Name   string
	Kind   Reminder
	Ledger *Ledger
	DryRun bool
	// Auth returns ctx with the identity that the run acts as. The jobs runner
	// gives a run a context without one.
	Auth func(ctx context.Context) (context.Context, error)
	// Publish sends one notification.
	Publish func(ctx context.Context, event string, to Recipient, data map[string]any) error
	Now     func() time.Time
}

// Report is what a run did.
type Report struct {
	Scanned    int   `json:"scanned"`
	Due        int   `json:"due"`
	Recipients int   `json:"recipients"`
	Published  int   `json:"published"`
	Closed     int64 `json:"closed"`
	Failures   int   `json:"failures"`
	DryRun     bool  `json:"dry_run"`
}

// Due is an item that a run sends, with the level already sent in its
// episode.
type Due struct {
	Item
	LevelSent int
}

// Run scans the kind, claims the items that are due, sends one notification
// for each recipient and closes the episodes that the scan did not see. It
// keeps the report as the last run of the kind.
func (r *Runner) Run(ctx context.Context) (Report, error) {
	start := r.Now().UTC()
	report, err := r.run(ctx, start, r.DryRun, nil)
	rec := RunRecord{Kind: r.Name, StartedAt: start, FinishedAt: r.Now().UTC()}
	rec.Report, _ = json.Marshal(report)
	if err != nil {
		rec.Error = err.Error()[:min(len(err.Error()), 1024)]
	}
	if rerr := r.Ledger.RecordRun(context.WithoutCancel(ctx), rec); rerr != nil {
		appctx.GetLogger(ctx).Error().Err(rerr).Str("kind", r.Name).Msg("reminders: recording the run")
	}
	return report, err
}

// Check scans the kind like a dry run and returns at most limit of the items
// that are due.
func (r *Runner) Check(ctx context.Context, limit int) (Report, []Due, error) {
	var due []Due
	report, err := r.run(ctx, r.Now().UTC(), true, func(d Due) {
		if len(due) < limit {
			due = append(due, d)
		}
	})
	return report, due, err
}

func (r *Runner) run(ctx context.Context, start time.Time, dry bool, onDue func(Due)) (Report, error) {
	log := appctx.GetLogger(ctx)
	report := Report{DryRun: dry}
	if r.Auth != nil {
		var err error
		if ctx, err = r.Auth(ctx); err != nil {
			return report, err
		}
	}
	run := uuid.NewString()
	if !dry {
		defer func() {
			if err := r.Ledger.Release(context.WithoutCancel(ctx), r.Name, run, nil); err != nil {
				log.Error().Err(err).Str("kind", r.Name).Msg("reminders: releasing the claims of the run")
			}
		}()
	}

	var failures []error
	complete := true
	recipients := map[Recipient]bool{}
	batch := make([]Item, 0, batchSize)
	flush := func() error {
		defer func() { batch = batch[:0] }()
		if len(batch) == 0 {
			return nil
		}
		if !dry {
			if err := r.Ledger.Observe(ctx, r.Name, batch, start); err != nil {
				return err
			}
			n, err := r.Ledger.Claim(ctx, r.Name, run, batch, start)
			report.Due += n
			return err
		}
		subjects := make([]string, 0, len(batch))
		for _, it := range batch {
			subjects = append(subjects, it.Subject)
		}
		sent, err := r.Ledger.LevelsSent(ctx, r.Name, subjects)
		if err != nil {
			return err
		}
		for _, it := range batch {
			if it.Level <= sent[it.Subject] {
				continue
			}
			report.Due++
			recipients[it.Recipient] = true
			log.Info().Str("kind", r.Name).Str("subject", it.Subject).Int("level", it.Level).Str("recipient", it.Recipient.Name).Msg("reminders: dry run, would send")
			if onDue != nil {
				onDue(Due{Item: it, LevelSent: sent[it.Subject]})
			}
		}
		return nil
	}
	for it, err := range r.Kind.Scan(ctx) {
		if err != nil {
			complete = false
			failures = append(failures, err)
			continue
		}
		report.Scanned++
		batch = append(batch, it)
		if len(batch) == batchSize {
			if err := flush(); err != nil {
				return report, err
			}
		}
	}
	if err := flush(); err != nil {
		return report, err
	}

	if dry {
		report.Recipients = len(recipients)
	} else {
		for to, err := range r.Ledger.Recipients(ctx, r.Name, run) {
			if err != nil {
				return report, err
			}
			report.Recipients++
			if err := r.notify(ctx, run, to, start); err != nil {
				failures = append(failures, fmt.Errorf("notifying %s: %w", to.Name, err))
				if err := r.Ledger.Release(ctx, r.Name, run, &to); err != nil {
					return report, err
				}
				continue
			}
			report.Published++
		}
		if complete {
			var err error
			if report.Closed, err = r.Ledger.Close(ctx, r.Name, start); err != nil {
				return report, err
			}
		}
	}

	report.Failures = len(failures)
	log.Info().Str("kind", r.Name).Interface("report", report).Msg("reminders: run done")
	return report, errors.Join(failures...)
}

func (r *Runner) notify(ctx context.Context, run string, to Recipient, now time.Time) error {
	var readErr error
	items := func(yield func(Item) bool) {
		for it, err := range r.Ledger.Claimed(ctx, r.Name, run, to) {
			if err != nil {
				readErr = err
				return
			}
			if !yield(it) {
				return
			}
		}
	}
	data, err := r.Kind.Notice(ctx, to, items)
	if err := cmp.Or(readErr, err); err != nil {
		return err
	}
	if err := r.Publish(ctx, r.Kind.Event(), to, data); err != nil {
		return err
	}
	return r.Ledger.MarkSent(ctx, r.Name, run, to, r.Kind.Event(), now)
}
