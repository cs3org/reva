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
	"time"
)

// State is the lifecycle state of a run.
type State string

const (
	// StateQueued means the run is persisted and waiting to be claimed.
	StateQueued State = "queued"
	// StateRunning means a worker has claimed the run and Job.Run is
	// executing.
	StateRunning State = "running"
	// StateSucceeded is the terminal state of a run that completed without
	// error.
	StateSucceeded State = "succeeded"
	// StateFailed means the most recent attempt returned an error. It is NOT
	// terminal: the framework re-delivers a failed run, so the run will move
	// back to queued and be retried. A client should read StateFailed as
	// "last attempt failed, another is coming", not "given up".
	StateFailed State = "failed"
)

// Status is the observable state of a single run, addressable by its RunID.
type Status struct {
	RunID      RunID
	Job        string
	State      State
	Attempt    int
	EnqueuedAt time.Time
	// StartedAt is set when the run is first claimed; nil while queued.
	StartedAt *time.Time
	// FinishedAt is set when an attempt finishes (success or failure); nil
	// while queued or running.
	FinishedAt *time.Time
	// LastError carries the error of the most recent failed attempt.
	LastError string
	// Result is the payload returned by the job on success.
	Result Params
}

// StatusStore persists and serves the per-run status. It is a separate
// concern from the work-queue Store: the queue handles delivery, the status
// store handles observability, and a deployment may in principle back them
// with different technologies.
type StatusStore interface {
	// Put upserts the status of a run, keyed by its RunID.
	Put(ctx context.Context, s Status) error
	// Get returns the status of a run. It returns an errtypes.NotFound error
	// if the run is unknown.
	Get(ctx context.Context, id RunID) (Status, error)
	// Close releases the status store's resources.
	Close(ctx context.Context) error
}
