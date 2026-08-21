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

// Package loginflow holds the persistent state for the login flow. It stores
// one row per pending enrolment and runs the atomic state
// transitions (approve, consume) that the flow needs. It does not authenticate
// anyone; its only job is to track a flow until an appauth credential is minted.
package loginflow

import (
	"context"
	"crypto/sha256"
	"time"
)

// HashToken returns SHA256(token). Flows are stored and looked up by this hash;
// the raw login and poll tokens are never persisted.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Flow is one pending or approved enrolment attempt.
type Flow struct {
	LoginHash  []byte     // SHA256(logintoken)
	PollHash   []byte     // SHA256(polltoken)
	ClientID   string     // UUIDv4, surfaced to audit / management API
	UserAgent  string     // raw User-Agent from Init
	CreatedAt  time.Time  // set at Init
	ExpiresAt  time.Time  // PENDING lifetime
	ApprovedAt *time.Time // nil = PENDING; non-nil = APPROVED
	UserID     string     // set on approval, opaque user id
	Username   string     // set on approval, login name for the poll response
	DeviceName string     // set on approval, becomes the app password label
}

// Approved reports whether the flow has been granted.
func (f *Flow) Approved() bool { return f.ApprovedAt != nil }

// Expired reports whether the flow has passed its PENDING lifetime.
func (f *Flow) Expired() bool { return time.Now().After(f.ExpiresAt) }

// Manager stores flows and runs their state transitions.
type Manager interface {
	// CreateFlow inserts a new PENDING flow.
	CreateFlow(ctx context.Context, f *Flow) error
	// GetByLogin returns the flow for a login hash, including expired ones so
	// the caller can tell "gone" from "unknown". Returns errtypes.NotFound if
	// no live (non-deleted) row exists.
	GetByLogin(ctx context.Context, loginHash []byte) (*Flow, error)
	// GetByPoll is GetByLogin keyed by poll hash.
	GetByPoll(ctx context.Context, pollHash []byte) (*Flow, error)
	// Approve runs the PENDING -> APPROVED compare-and-set. It records the
	// granting user and the device name. Returns errtypes.Conflict if the row
	// was not in a PENDING, non-expired state (lost race).
	Approve(ctx context.Context, loginHash []byte, userID, username, deviceName string) error
	// Consume runs the APPROVED -> soft-deleted compare-and-set and returns the
	// consumed flow. Returns errtypes.NotFound if the row was not in an
	// APPROVED, non-expired state (already consumed or lost race).
	Consume(ctx context.Context, pollHash []byte) (*Flow, error)
	// Deny soft-deletes a PENDING flow. It is a no-op if no PENDING row exists.
	Deny(ctx context.Context, loginHash []byte) error
}
