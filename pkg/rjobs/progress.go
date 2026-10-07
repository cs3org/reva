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
	"maps"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// defaultProgressInterval is how often the progress of a running run is
// persisted, when it changed.
const defaultProgressInterval = 10 * time.Second

// Progress is a snapshot of how far a run has got. Done and Total count one
// unit of the job's choice; Total is 0 while unknown.
type Progress struct {
	Phase   string `json:"phase,omitempty"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total,omitempty"`
	Unit    string `json:"unit,omitempty"` // e.g. "bytes"
	Details Params `json:"details,omitempty"`
}

// Reporter records the progress of the run it was handed to.
// Report never blocks, and the latest snapshot wins.
type Reporter interface {
	Report(Progress)
}

// ProgressJob is an optional interface: the runner calls RunWithProgress
// instead of Run on a job that implements it.
type ProgressJob interface {
	Job
	RunWithProgress(ctx context.Context, p Params, r Reporter) (Params, error)
}

// ProgressStore is an optional interface of a StatusStore that persists the
// progress of runs. PutProgress only touches a run that is not terminal, so a
// late write cannot replace the final snapshot.
type ProgressStore interface {
	PutProgress(ctx context.Context, id RunID, p Progress, at time.Time) error
}

// progressReporter keeps the latest snapshot of a run. seq counts the reports,
// so the writer persists only when something changed.
type progressReporter struct {
	mu   sync.Mutex
	snap Progress
	at   time.Time
	seq  uint64
}

func (r *progressReporter) Report(p Progress) {
	p.Details = maps.Clone(p.Details)
	r.mu.Lock()
	r.snap = p
	r.at = time.Now()
	r.seq++
	r.mu.Unlock()
}

// latest returns the current snapshot and its sequence number; ok is false
// while nothing was reported.
func (r *progressReporter) latest() (p Progress, at time.Time, seq uint64, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snap, r.at, r.seq, r.seq > 0
}

// startProgressWriter persists the run's progress every interval while it
// changes. The returned stop function stops the writer, waits for it, and then
// writes the final snapshot, so no tick can land after it.
func (r *Runner) startProgressWriter(ctx context.Context, run Run, rep *progressReporter, log zerolog.Logger) (stop func()) {
	ps, ok := r.status.(ProgressStore)
	if !ok {
		return func() {}
	}

	var written uint64
	write := func(ctx context.Context) {
		p, at, seq, ok := rep.latest()
		if !ok || seq == written {
			return
		}
		if err := ps.PutProgress(ctx, run.ID, p, at); err != nil {
			log.Warn().Err(err).Msg("rjobs: recording progress failed")
			return
		}
		written = seq
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(r.progressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				write(ctx)
			}
		}
	})

	return func() {
		close(done)
		wg.Wait()
		// the final snapshot must survive a shutdown, so it does not use ctx.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		write(fctx)
	}
}
