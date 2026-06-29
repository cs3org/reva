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

import "sync"

// EnqueueOption customises an on-demand enqueue.
type EnqueueOption func(*Run)

// ConflictPolicy decides what Enqueue does when a Unique key is already held by
// an active run.
type ConflictPolicy int

const (
	// Collapse returns the existing run's id, so a duplicate request rides on
	// the run already in flight. It is the default.
	Collapse ConflictPolicy = iota
	// Reject makes Enqueue fail with an errtypes.AlreadyExists error instead of
	// reusing the in-flight run.
	Reject
)

// Unique makes the run the only active one for its (owner, key): while a run
// with this key is queued, running or retrying, a second Enqueue with the same
// key does not start another. The reservation is released once the run
// succeeds, so the key is free again afterwards; fold any finer scope (one per
// day, one per space, ...) into the key. With no policy a conflict collapses
// onto the existing run and Enqueue returns its id; pass Reject to fail
// instead. Pair it with WithOwner to make uniqueness per user.
func Unique(key string, policy ...ConflictPolicy) EnqueueOption {
	return func(r *Run) {
		r.DedupKey = key
		r.dedupReject = len(policy) > 0 && policy[0] == Reject
	}
}

// WithOwner attributes the run to a user by username, so it appears in that
// user's run listing (see Runner.ListByOwner). Without this option a run has no
// owner and counts as internal.
func WithOwner(username string) EnqueueOption {
	return func(r *Run) {
		r.Owner = username
	}
}

// defaultRunner is the process-wide runner, set by the jobs service once at
// startup. It lets any in-process reva component reach Enqueue without
// threading the runner through every call site.
var (
	defaultMu     sync.RWMutex
	defaultRunner *Runner
)

// SetDefault registers the process-wide runner. It is called by the jobs
// service.
func SetDefault(r *Runner) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultRunner = r
}

// Default returns the process-wide runner, or nil if the jobs service is not
// enabled. Callers that want to enqueue work should check for nil.
func Default() *Runner {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultRunner
}
