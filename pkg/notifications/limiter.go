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

package notifications

import (
	"context"
	"sync"
	"time"
)

// RateLimiter limits notification submissions per submitting user.
type RateLimiter interface {
	Allow(ctx context.Context, submittingUser string) error
}

// NoopRateLimiter accepts all submissions.
type NoopRateLimiter struct{}

// Allow implements RateLimiter.
func (NoopRateLimiter) Allow(context.Context, string) error {
	return nil
}

// RateLimitError is returned when a submitting user exceeds the configured rate.
type RateLimitError struct {
	SubmittingUser string
	RetryAfter     time.Duration
}

// Error implements error.
func (e *RateLimitError) Error() string {
	return "notification rate limit exceeded"
}

type fixedWindowState struct {
	windowStart time.Time
	count       int
}

// FixedWindowRateLimiter is a simple in-memory per-user fixed-window limiter.
type FixedWindowRateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu     sync.Mutex
	states map[string]fixedWindowState
}

// NewFixedWindowRateLimiter creates a per-user fixed-window limiter.
func NewFixedWindowRateLimiter(limit int, window time.Duration) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		limit:  limit,
		window: window,
		now:    time.Now,
		states: make(map[string]fixedWindowState),
	}
}

// Allow implements RateLimiter.
func (l *FixedWindowRateLimiter) Allow(_ context.Context, submittingUser string) error {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return nil
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.states[submittingUserKey(submittingUser)]
	if state.windowStart.IsZero() || now.Sub(state.windowStart) >= l.window {
		l.states[submittingUserKey(submittingUser)] = fixedWindowState{
			windowStart: now,
			count:       1,
		}
		return nil
	}

	if state.count >= l.limit {
		return &RateLimitError{
			SubmittingUser: submittingUser,
			RetryAfter:     l.window - now.Sub(state.windowStart),
		}
	}

	state.count++
	l.states[submittingUserKey(submittingUser)] = state
	return nil
}

func submittingUserKey(submittingUser string) string {
	if submittingUser == "" {
		return "<anonymous>"
	}
	return submittingUser
}
