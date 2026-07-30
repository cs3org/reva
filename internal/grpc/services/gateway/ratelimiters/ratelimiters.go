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

// Package ratelimiters holds the rate limiters the gateway applies to the
// PublishEvent flow, keyed by the submitting user.
package ratelimiters

import (
	"context"
	"time"
)

// Limiter limits notification submissions per submitting user.
type Limiter interface {
	Allow(ctx context.Context, submittingUser string) error
}

// Noop accepts all submissions.
type Noop struct{}

// Allow implements Limiter.
func (Noop) Allow(context.Context, string) error {
	return nil
}

// LimitError is returned when a submitting user exceeds the configured rate.
type LimitError struct {
	SubmittingUser string
	RetryAfter     time.Duration
}

// Error implements error.
func (e *LimitError) Error() string {
	return "rate limit exceeded"
}
