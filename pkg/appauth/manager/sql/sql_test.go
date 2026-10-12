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
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/sql/model"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/google/uuid"
)

func newTestManager(t *testing.T) (appauth.Manager, context.Context, *userpb.UserId) {
	t.Helper()
	return newManagerAt(t, filepath.Join(t.TempDir(), "appauth.db"), nil)
}

func newManagerAt(t *testing.T, db string, conf map[string]any) (appauth.Manager, context.Context, *userpb.UserId) {
	t.Helper()
	m := map[string]any{"db_engine": "sqlite", "db_name": db}
	maps.Copy(m, conf)
	mgr, err := New(context.Background(), m)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	uid := &userpb.UserId{OpaqueId: "jdoe", Idp: "example.org", Type: userpb.UserType_USER_TYPE_PRIMARY}
	return mgr, userContext(uid), uid
}

func userContext(uid *userpb.UserId) context.Context {
	return appctx.ContextSetUser(context.Background(), &userpb.User{Id: uid, Username: uid.OpaqueId})
}

func appAuthScope(t *testing.T, client string, perms []string, roots ...string) map[string]*authpb.Scope {
	t.Helper()
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{Client: client, Permissions: perms, Roots: roots}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return scopes
}

func inDays(d int) *typespb.Timestamp {
	return &typespb.Timestamp{Seconds: uint64(time.Now().AddDate(0, 0, d).Unix())}
}

func TestGenerateGetInvalidate(t *testing.T) {
	m, ctx, uid := newTestManager(t)

	pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientSync, []string{scope.PermAll}), "laptop|client-1", nil)
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

// The scope is built from the columns, so what the store returns is what was
// asked for, with the id and the expiry the store assigned.
func TestScopeFromColumns(t *testing.T) {
	m, ctx, uid := newTestManager(t)
	exp := inDays(30)
	pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientCLI, scope.ReadOnlyPermissions()), "laptop", exp)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, got := range []*apppb.AppPassword{pw, mustGet(t, m, ctx, uid, pw.Password)} {
		ap, ok := scope.GetAppAuthScope(got.TokenScope)
		if !ok || len(got.TokenScope) != 1 {
			t.Fatalf("scope %v", got.TokenScope)
		}
		if _, err := uuid.Parse(ap.PasswordID); err != nil {
			t.Errorf("password id %q: %v", ap.PasswordID, err)
		}
		if ap.Client != scope.ClientCLI || !slices.Equal(ap.Permissions, scope.ReadOnlyPermissions()) ||
			len(ap.Roots) != 0 || ap.ExpiresAt != int64(exp.Seconds) {
			t.Errorf("scope %+v", ap)
		}
		if got.TokenScope["appauth"].Role != authpb.Role_ROLE_VIEWER {
			t.Errorf("role %v", got.TokenScope["appauth"].Role)
		}
	}
}

func TestLookupBySecretAlone(t *testing.T) {
	m, ctx, uid := newTestManager(t)
	pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()), "script", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	got, err := m.GetAppPassword(context.Background(), nil, pw.Password)
	if err != nil {
		t.Fatalf("get without user: %v", err)
	}
	if got.GetUser().GetOpaqueId() != uid.OpaqueId {
		t.Fatalf("owner %v, want %s", got.GetUser(), uid.OpaqueId)
	}
	if _, err := m.GetAppPassword(context.Background(), nil, "wrong"); !isNotFound(err) {
		t.Fatalf("want not found for a wrong secret, got %v", err)
	}
	other := &userpb.UserId{OpaqueId: "mallory"}
	if _, err := m.GetAppPassword(context.Background(), other, pw.Password); !isNotFound(err) {
		t.Fatalf("want not found for another user, got %v", err)
	}
}

func TestExpiry(t *testing.T) {
	m, ctx, uid := newTestManager(t)
	read := scope.ReadOnlyPermissions()

	// Mandatory for every kind but sync: the default applies.
	for _, kind := range []string{scope.ClientCLI, scope.ClientWebDAV} {
		pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, kind, read), kind, nil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		want := time.Now().AddDate(0, 0, 90).Unix()
		if got := int64(pw.GetExpiration().GetSeconds()); got < want-5 || got > want {
			t.Errorf("%s: expires at %d, want the 90 days default %d", kind, got, want)
		}
	}
	sync, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientSync, []string{scope.PermAll}), "sync", nil)
	if err != nil || sync.GetExpiration() != nil {
		t.Fatalf("a sync app password needs no expiry: %v, %v", sync.GetExpiration(), err)
	}

	for name, exp := range map[string]*typespb.Timestamp{"past": inDays(-1), "beyond the maximum": inDays(366)} {
		if _, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientWebDAV, read), name, exp); !isBadRequest(err) {
			t.Errorf("%s: want a bad request, got %v", name, err)
		}
	}

	// The store refuses an expired app password.
	pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientWebDAV, read), "short", inDays(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.(*mgr).db.Model(&model.AppPassword{}).Where("user_id = ? AND label = ?", uid.OpaqueId, "short").
		Update("expiration", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetAppPassword(ctx, nil, pw.Password); !isNotFound(err) {
		t.Fatalf("want not found for an expired app password, got %v", err)
	}

	// Configurable.
	m2, ctx2, _ := newManagerAt(t, filepath.Join(t.TempDir(), "appauth.db"), map[string]any{"default_expiry_days": 7, "max_expiry_days": 30})
	pw, err = m2.GenerateAppPassword(ctx2, appAuthScope(t, scope.ClientCLI, read), "cli", nil)
	if err != nil || int64(pw.GetExpiration().GetSeconds()) > time.Now().AddDate(0, 0, 7).Unix() {
		t.Fatalf("want the configured default: %v, %v", pw.GetExpiration(), err)
	}
	if _, err := m2.GenerateAppPassword(ctx2, appAuthScope(t, scope.ClientCLI, read), "cli", inDays(31)); !isBadRequest(err) {
		t.Fatalf("want the configured maximum, got %v", err)
	}
}

func TestPolicy(t *testing.T) {
	db := filepath.Join(t.TempDir(), "appauth.db")
	m, _, _ := newManagerAt(t, db, nil)
	custom, _, _ := newManagerAt(t, db, map[string]any{"policy": map[string]any{"user_types": map[string]any{
		"lightweight": []string{scope.ClientCLI},
		"service":     []string{scope.ClientWebDAV},
	}}})
	read := scope.ReadOnlyPermissions()

	cases := []struct {
		mgr    appauth.Manager
		typ    userpb.UserType
		client string
		want   bool
	}{
		{m, userpb.UserType_USER_TYPE_PRIMARY, scope.ClientCLI, true},
		{m, userpb.UserType_USER_TYPE_SECONDARY, scope.ClientCLI, true},
		{m, userpb.UserType_USER_TYPE_SERVICE, scope.ClientWebDAV, true},
		{m, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientSync, false},
		{m, userpb.UserType_USER_TYPE_GUEST, scope.ClientWebDAV, false},
		{m, userpb.UserType_USER_TYPE_FEDERATED, scope.ClientCLI, false},
		{m, userpb.UserType_USER_TYPE_INVALID, scope.ClientSync, false},
		{custom, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientCLI, true},
		{custom, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientSync, false},
		{custom, userpb.UserType_USER_TYPE_SERVICE, scope.ClientCLI, false},
		// Types left out of the configuration keep their defaults.
		{custom, userpb.UserType_USER_TYPE_PRIMARY, scope.ClientSync, true},
	}
	for _, c := range cases {
		ctx := userContext(&userpb.UserId{OpaqueId: "u", Type: c.typ})
		perms := read
		if c.client == scope.ClientSync {
			perms = []string{scope.PermAll}
		}
		_, err := c.mgr.GenerateAppPassword(ctx, appAuthScope(t, c.client, perms), "p", nil)
		if got := err == nil; got != c.want {
			t.Errorf("%v creating %s: allowed = %v (%v), want %v", c.typ, c.client, got, err, c.want)
		}
		if !c.want {
			if _, ok := err.(errtypes.PermissionDenied); !ok {
				t.Errorf("%v creating %s: want permission denied, got %v", c.typ, c.client, err)
			}
		}
	}
}

func TestRejectsInvalidScopes(t *testing.T) {
	m, ctx, _ := newTestManager(t)
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	both, err := scope.AddOwnerScope(appAuthScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]map[string]*authpb.Scope{
		"owner scope":           owner,
		"owner scope alongside": both,
		"unknown client":        appAuthScope(t, "printer", scope.ReadOnlyPermissions()),
		"no permission":         appAuthScope(t, scope.ClientWebDAV, nil),
		"unknown permission":    appAuthScope(t, scope.ClientWebDAV, []string{"files:steal"}),
		"wildcard and more":     appAuthScope(t, scope.ClientSync, []string{scope.PermAll, scope.PermFilesList}),
		"relative root":         appAuthScope(t, scope.ClientCLI, scope.ReadOnlyPermissions(), "thesis"),
		"roots":                 appAuthScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions(), "/eos/user/j/jdoe/thesis"),
	}
	for name, scopes := range cases {
		if _, err := m.GenerateAppPassword(ctx, scopes, name, nil); !isBadRequest(err) {
			t.Errorf("%s: want a bad request, got %v", name, err)
		}
	}
}

func mustGet(t *testing.T, m appauth.Manager, ctx context.Context, uid *userpb.UserId, secret string) *apppb.AppPassword {
	t.Helper()
	pw, err := m.GetAppPassword(ctx, uid, secret)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return pw
}

func isNotFound(err error) bool {
	_, ok := err.(errtypes.NotFound)
	return ok
}

func isBadRequest(err error) bool {
	_, ok := err.(errtypes.BadRequest)
	return ok
}

// An administrator creates for any user, without the user type policy or the
// maximum expiry, and is recorded as the creator.
func TestCreatedByAdministrator(t *testing.T) {
	m, _, _ := newTestManager(t)
	guest := &userpb.UserId{OpaqueId: "guest", Type: userpb.UserType_USER_TYPE_GUEST}
	admin := &userpb.UserId{OpaqueId: "root-admin"}
	ctx := userContext(guest)

	if _, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientCLI, scope.ReadOnlyPermissions()), "denied", nil); err == nil {
		t.Fatal("a guest must not create app passwords")
	}
	pw, err := m.GenerateAppPassword(appauth.ContextWithAdministrator(ctx, admin), appAuthScope(t, scope.ClientCLI, scope.ReadOnlyPermissions()), "by admin", inDays(500))
	if err != nil {
		t.Fatalf("an administrator creates for any user: %v", err)
	}
	if pw.GetUser().GetOpaqueId() != "guest" {
		t.Fatalf("owner %v", pw.GetUser())
	}
	own, err := m.GenerateAppPassword(userContext(&userpb.UserId{OpaqueId: "guest", Type: userpb.UserType_USER_TYPE_PRIMARY}), appAuthScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()), "own", nil)
	if err != nil {
		t.Fatal(err)
	}

	creators, err := m.(appauth.CreatorLister).ListAppPasswordCreators(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byAdmin, _ := scope.GetAppAuthScope(pw.TokenScope)
	byOwner, _ := scope.GetAppAuthScope(own.TokenScope)
	if len(creators) != 2 || creators[byAdmin.PasswordID] != "root-admin" || creators[byOwner.PasswordID] != "guest" {
		t.Fatalf("creators %v", creators)
	}
}

func TestInvalidateByPasswordID(t *testing.T) {
	m, ctx, uid := newTestManager(t)
	pw, err := m.GenerateAppPassword(ctx, appAuthScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()), "script", nil)
	if err != nil {
		t.Fatal(err)
	}
	ap, _ := scope.GetAppAuthScope(pw.TokenScope)

	other := userContext(&userpb.UserId{OpaqueId: "mallory", Type: userpb.UserType_USER_TYPE_PRIMARY})
	if err := m.InvalidateAppPassword(other, ap.PasswordID); !isNotFound(err) {
		t.Fatalf("another user must not revoke it, got %v", err)
	}
	if err := m.InvalidateAppPassword(ctx, ap.PasswordID); err != nil {
		t.Fatalf("invalidate by password id: %v", err)
	}
	if _, err := m.GetAppPassword(ctx, uid, pw.Password); !isNotFound(err) {
		t.Fatalf("revoked app password still found: %v", err)
	}
}
