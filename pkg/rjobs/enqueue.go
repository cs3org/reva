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

// WithIdempotencyKey collapses duplicate enqueues carrying the same key into a
// single run. Use it to make a user action (e.g. clicking "export" twice)
// enqueue at most one run.
func WithIdempotencyKey(key string) EnqueueOption {
	return func(r *Run) {
		r.IdempotencyKey = key
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
