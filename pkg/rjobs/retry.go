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
	"errors"
	"time"
)

// retryAfterError asks the runner to retry the run after a given delay.
type retryAfterError struct {
	after time.Duration
	cause error
}

func (e *retryAfterError) Error() string {
	if e.cause == nil {
		return "rjobs: retry after " + e.after.String()
	}
	return e.cause.Error()
}

func (e *retryAfterError) Unwrap() error { return e.cause }

// RetryAfter returns an error that makes the runner retry the run after d
// (0 retries at once) instead of after the default delay. The run is recorded
// as queued, with cause as its last error, since it is waiting, not broken.
func RetryAfter(d time.Duration, cause error) error {
	return &retryAfterError{after: max(d, 0), cause: cause}
}

// permanentError marks a failure that a retry cannot fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent returns an error that ends the run for good: it is recorded as
// aborted and never retried. A nil err stays nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// retryDelay reports the delay a RetryAfter error asks for.
func retryDelay(err error) (time.Duration, bool) {
	if ra, ok := errors.AsType[*retryAfterError](err); ok {
		return ra.after, true
	}
	return 0, false
}

// isPermanent reports whether err was wrapped by Permanent.
func isPermanent(err error) bool {
	_, ok := errors.AsType[*permanentError](err)
	return ok
}
