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

package sql

import (
	"context"
	"path/filepath"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func newTestManager(t *testing.T) (appauth.Manager, context.Context, *userpb.UserId) {
	t.Helper()
	m, err := New(context.Background(), map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "appauth.db"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	uid := &userpb.UserId{OpaqueId: "jdoe", Idp: "example.org"}
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: uid, Username: "jdoe"})
	return m, ctx, uid
}

func TestGenerateGetInvalidate(t *testing.T) {
	m, ctx, uid := newTestManager(t)

	ownerScope, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("owner scope: %v", err)
	}

	pw, err := m.GenerateAppPassword(ctx, ownerScope, "laptop|client-1", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if pw.Password == "" {
		t.Fatal("generate returned empty token")
	}

	// The plaintext token authenticates.
	got, err := m.GetAppPassword(ctx, uid, pw.Password)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Label != "laptop|client-1" || len(got.TokenScope) == 0 {
		t.Fatalf("unexpected app password: %+v", got)
	}

	// A wrong secret misses.
	if _, err := m.GetAppPassword(ctx, uid, "wrong"); !isNotFound(err) {
		t.Fatalf("want not found for wrong secret, got %v", err)
	}

	// List returns a hash handle, not the plaintext.
	list, err := m.ListAppPasswords(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 password, got %d", len(list))
	}
	handle := list[0].Password
	if handle == pw.Password {
		t.Fatal("list must not return the plaintext token")
	}

	// Invalidate by the handle; the token then misses.
	if err := m.InvalidateAppPassword(ctx, handle); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if _, err := m.GetAppPassword(ctx, uid, pw.Password); !isNotFound(err) {
		t.Fatalf("token should be gone, got %v", err)
	}

	// Invalidating again reports not found (idempotency handled by the caller).
	if err := m.InvalidateAppPassword(ctx, handle); !isNotFound(err) {
		t.Fatalf("second invalidate should be not found, got %v", err)
	}
}

func isNotFound(err error) bool {
	_, ok := err.(errtypes.NotFound)
	return ok
}
