// Copyright 2018-2021 CERN
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

package json

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/owncloud/reva/v2/pkg/appauth"
	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"golang.org/x/crypto/bcrypt"
)

// seedFile writes a JSON appauth file containing the given tokens keyed by
// user ID and bcrypt hash. This allows tests to inject arbitrary state
// (including expired tokens) without going through GenerateAppPassword.
func seedFile(t *testing.T, tokens map[string]map[string]*apppb.AppPassword) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "appauth.json")
	data, err := json.Marshal(tokens)
	if err != nil {
		t.Fatalf("marshal seed data: %v", err)
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	return file
}

// hashPassword returns a bcrypt hash for the given plaintext password.
func hashPassword(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 4)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	return string(h)
}

func loadManager(t *testing.T, file string, keepExpired bool) appauth.Manager {
	t.Helper()
	mgr, err := New(map[string]interface{}{
		"file":                        file,
		"token_strength":              16,
		"password_hash_cost":          4,
		"keep_expired_tokens_on_load": keepExpired,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return mgr
}

func testContext(uid string) context.Context {
	user := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: uid,
			Idp:      "test",
		},
	}
	return ctxpkg.ContextSetUser(context.Background(), user)
}

func testUserID(uid string) *userpb.UserId {
	return &userpb.UserId{OpaqueId: uid, Idp: "test"}
}

func futureExpiration() *typespb.Timestamp {
	return &typespb.Timestamp{Seconds: uint64(time.Now().Add(1 * time.Hour).Unix())}
}

// --- GetAppPassword tests ---
// These tests seed state via JSON file to isolate GetAppPassword from
// GenerateAppPassword, per review feedback.

func TestGetAppPassword_SkipsExpiredToken(t *testing.T) {
	uid := testUserID("user1")
	pw := "secret123"
	hash := hashPassword(t, pw)

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash: {
				Password:   hash,
				Label:      "expired-token",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Add(-2 * time.Hour).Unix())},
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(-1 * time.Hour).Unix())},
			},
		},
	})

	mgr := loadManager(t, file, true)
	ctx := testContext("user1")

	_, err := mgr.GetAppPassword(ctx, uid, pw)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestGetAppPassword_ValidToken(t *testing.T) {
	uid := testUserID("user1")
	pw := "secret123"
	hash := hashPassword(t, pw)

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash: {
				Password:   hash,
				Label:      "valid-token",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Unix())},
				Expiration: futureExpiration(),
			},
		},
	})

	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	result, err := mgr.GetAppPassword(ctx, uid, pw)
	if err != nil {
		t.Fatalf("GetAppPassword: %v", err)
	}
	if result.Label != "valid-token" {
		t.Errorf("expected label 'valid-token', got %q", result.Label)
	}
}

func TestGetAppPassword_NoExpirationNeverExpires(t *testing.T) {
	uid := testUserID("user1")
	pw := "secret123"
	hash := hashPassword(t, pw)

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash: {
				Password: hash,
				Label:    "no-expiry",
				User:     uid,
				Ctime:    &typespb.Timestamp{Seconds: uint64(time.Now().Unix())},
				// Expiration deliberately nil
			},
		},
	})

	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	result, err := mgr.GetAppPassword(ctx, uid, pw)
	if err != nil {
		t.Fatalf("GetAppPassword should succeed for non-expiring token: %v", err)
	}
	if result.Label != "no-expiry" {
		t.Errorf("expected label 'no-expiry', got %q", result.Label)
	}
}

func TestGetAppPassword_ZeroExpirationNeverExpires(t *testing.T) {
	uid := testUserID("user1")
	pw := "secret123"
	hash := hashPassword(t, pw)

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash: {
				Password:   hash,
				Label:      "zero-expiry",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Unix())},
				Expiration: &typespb.Timestamp{Seconds: 0},
			},
		},
	})

	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	result, err := mgr.GetAppPassword(ctx, uid, pw)
	if err != nil {
		t.Fatalf("GetAppPassword should succeed for zero-expiry token: %v", err)
	}
	if result.Label != "zero-expiry" {
		t.Errorf("expected label 'zero-expiry', got %q", result.Label)
	}
}

// --- Purge on load tests ---

func TestPurgeExpiredTokensOnLoad(t *testing.T) {
	uid := testUserID("user1")
	hash1 := hashPassword(t, "expired-pw")
	hash2 := hashPassword(t, "valid-pw")

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash1: {
				Password:   hash1,
				Label:      "expired",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Add(-2 * time.Hour).Unix())},
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(-1 * time.Hour).Unix())},
			},
			hash2: {
				Password:   hash2,
				Label:      "valid",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Unix())},
				Expiration: futureExpiration(),
			},
		},
	})

	// Load with purge enabled (default).
	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	tokens, _ := mgr.ListAppPasswords(ctx)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token after purge, got %d", len(tokens))
	}
	if tokens[0].Label != "valid" {
		t.Errorf("expected remaining token to be 'valid', got %q", tokens[0].Label)
	}
}

func TestKeepExpiredTokensOnLoad(t *testing.T) {
	uid := testUserID("user1")
	hash1 := hashPassword(t, "expired-pw")
	hash2 := hashPassword(t, "valid-pw")

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash1: {
				Password:   hash1,
				Label:      "expired",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Add(-2 * time.Hour).Unix())},
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(-1 * time.Hour).Unix())},
			},
			hash2: {
				Password:   hash2,
				Label:      "valid",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Unix())},
				Expiration: futureExpiration(),
			},
		},
	})

	// Load with keep_expired_tokens_on_load = true.
	mgr := loadManager(t, file, true)
	ctx := testContext("user1")

	tokens, _ := mgr.ListAppPasswords(ctx)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens (expired kept), got %d", len(tokens))
	}
}

// --- GenerateAppPassword tests ---

func TestGenerateAppPassword_RejectsExpiredToken(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "appauth.json")
	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	_, err := mgr.GenerateAppPassword(ctx, nil, "should-fail", &typespb.Timestamp{
		Seconds: uint64(time.Now().Add(-1 * time.Hour).Unix()),
	})
	if err == nil {
		t.Fatal("expected error when creating already-expired token, got nil")
	}
}

func TestGenerateAppPassword_ValidToken(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "appauth.json")
	mgr := loadManager(t, file, false)
	ctx := testContext("user1")

	appPass, err := mgr.GenerateAppPassword(ctx, nil, "new-token", futureExpiration())
	if err != nil {
		t.Fatalf("GenerateAppPassword: %v", err)
	}
	if appPass.Label != "new-token" {
		t.Errorf("expected label 'new-token', got %q", appPass.Label)
	}
	// The returned password should be the plaintext token, not the hash.
	if appPass.Password == "" {
		t.Error("expected non-empty plaintext password")
	}
}

func TestGenerateAppPassword_PurgesExpiredForUser(t *testing.T) {
	uid := testUserID("user1")
	hash := hashPassword(t, "old-expired-pw")

	file := seedFile(t, map[string]map[string]*apppb.AppPassword{
		uid.String(): {
			hash: {
				Password:   hash,
				Label:      "expired",
				User:       uid,
				Ctime:      &typespb.Timestamp{Seconds: uint64(time.Now().Add(-2 * time.Hour).Unix())},
				Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(-1 * time.Hour).Unix())},
			},
		},
	})

	mgr := loadManager(t, file, true) // keep expired so seed isn't purged on load
	ctx := testContext("user1")

	// Generating a new valid token should purge the expired one.
	_, err := mgr.GenerateAppPassword(ctx, nil, "new-token", futureExpiration())
	if err != nil {
		t.Fatalf("GenerateAppPassword: %v", err)
	}

	tokens, _ := mgr.ListAppPasswords(ctx)
	for _, tok := range tokens {
		if tok.Label == "expired" {
			t.Error("expired token should have been purged by GenerateAppPassword")
		}
	}
	found := false
	for _, tok := range tokens {
		if tok.Label == "new-token" {
			found = true
		}
	}
	if !found {
		t.Error("new-token should be present after GenerateAppPassword")
	}
}

// --- isExpired unit test ---

func TestIsExpired(t *testing.T) {
	nowSec := uint64(time.Now().Unix())

	tests := []struct {
		name     string
		pw       *apppb.AppPassword
		expected bool
	}{
		{
			name:     "nil expiration",
			pw:       &apppb.AppPassword{},
			expected: false,
		},
		{
			name:     "zero seconds",
			pw:       &apppb.AppPassword{Expiration: &typespb.Timestamp{Seconds: 0}},
			expected: false,
		},
		{
			name:     "future expiration",
			pw:       &apppb.AppPassword{Expiration: &typespb.Timestamp{Seconds: nowSec + 3600}},
			expected: false,
		},
		{
			name:     "past expiration",
			pw:       &apppb.AppPassword{Expiration: &typespb.Timestamp{Seconds: nowSec - 3600}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpired(tt.pw, nowSec); got != tt.expected {
				t.Errorf("isExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

