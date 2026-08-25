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
// one client authorization per pending enrolment and runs the atomic state
// transitions (approve, consume) that the flow needs. It does not authenticate
// anyone; its only job is to track an authorization until an appauth credential
// is minted.
package loginflow

import (
	"context"
	"crypto/sha256"
	"strings"
	"time"
)

// HashToken returns SHA256(token). Client authorizations are stored and looked
// up by this hash; the raw login and poll tokens are never persisted.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// ClientDescription turns a raw User-Agent into a short, human-readable label
// for the confirmation page and the connected-clients list, e.g.
// "Nextcloud Sync Client v3.13.0 (on Linux)". It recognises the Nextcloud
// desktop sync client (which sends a "mirall/<version>" token) and falls back to
// the raw string for anything it does not know.
func ClientDescription(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return "Unknown client"
	}

	os := ""
	if l := strings.Index(ua, "("); l >= 0 {
		if r := strings.Index(ua[l+1:], ")"); r >= 0 {
			os = strings.TrimSpace(ua[l+1 : l+1+r])
		}
	}

	version := ""
	if v, ok := uaToken(ua, "mirall/"); ok {
		version = v
	} else if v, ok := uaToken(ua, "Nextcloud-Desktop/"); ok {
		version = v
	}

	if version != "" {
		label := "Nextcloud Sync Client v" + version
		if os != "" {
			label += " (on " + os + ")"
		}
		return label
	}

	return ua
}

// uaToken returns the whitespace-delimited value that follows prefix in s.
func uaToken(s, prefix string) (string, bool) {
	_, rest, ok := strings.Cut(s, prefix)
	if !ok {
		return "", false
	}
	if f := strings.Fields(rest); len(f) > 0 {
		return f[0], true
	}
	return "", false
}

// ClientAuthorization is one pending or approved enrolment attempt by a client.
type ClientAuthorization struct {
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

// Approved reports whether the authorization has been granted.
func (ca *ClientAuthorization) Approved() bool { return ca.ApprovedAt != nil }

// Expired reports whether the authorization has passed its PENDING lifetime.
func (ca *ClientAuthorization) Expired() bool { return time.Now().After(ca.ExpiresAt) }

// Manager stores client authorizations and runs their state transitions.
type Manager interface {
	// Create inserts a new PENDING authorization.
	Create(ctx context.Context, ca *ClientAuthorization) error
	// GetByLogin returns the authorization for a login hash, including expired
	// ones so the caller can tell "gone" from "unknown". Returns
	// errtypes.NotFound if no live (non-deleted) row exists.
	GetByLogin(ctx context.Context, loginHash []byte) (*ClientAuthorization, error)
	// GetByPoll is GetByLogin keyed by poll hash.
	GetByPoll(ctx context.Context, pollHash []byte) (*ClientAuthorization, error)
	// Approve records the user's consent, once: who granted, and what they named
	// the device. Returns errtypes.Conflict if the authorization was already
	// granted, already denied, or has expired.
	Approve(ctx context.Context, loginHash []byte, userID, username, deviceName string) error
	// Consume hands out an approved authorization once and only once, so two
	// polls can never mint two app passwords from one approval. The winner gets
	// the user to mint for. Returns errtypes.NotFound if the authorization was
	// never approved, is already consumed, or has expired.
	Consume(ctx context.Context, pollHash []byte) (*ClientAuthorization, error)
	// Deny soft-deletes a PENDING authorization. It is a no-op if no PENDING row
	// exists.
	Deny(ctx context.Context, loginHash []byte) error
}
