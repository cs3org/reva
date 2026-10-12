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
	"encoding/json"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func appPasswordScope(t *testing.T, client string, perms []string, roots ...string) map[string]*authpb.Scope {
	t.Helper()
	scopes, err := scope.AddAppPasswordScope(&scope.AppPasswordScope{Client: client, Permissions: perms, Roots: roots}, nil)
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

	pw, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientSync, []string{scope.PermAll}), "laptop|client-1", nil)
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
	pw, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientMCP, scope.ReadOnlyPermissions(), "/eos/user/j/jdoe/thesis"), "assistant", exp)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, got := range []*apppb.AppPassword{pw, mustGet(t, m, ctx, uid, pw.Password)} {
		ap, ok := scope.GetAppPasswordScope(got.TokenScope)
		if !ok || len(got.TokenScope) != 1 {
			t.Fatalf("scope %v", got.TokenScope)
		}
		if _, err := uuid.Parse(ap.PasswordID); err != nil {
			t.Errorf("password id %q: %v", ap.PasswordID, err)
		}
		if ap.Client != scope.ClientMCP || !slices.Equal(ap.Permissions, scope.ReadOnlyPermissions()) ||
			!slices.Equal(ap.Roots, []string{"/eos/user/j/jdoe/thesis"}) || ap.ExpiresAt != int64(exp.Seconds) {
			t.Errorf("scope %+v", ap)
		}
		if got.TokenScope["apppassword"].Role != authpb.Role_ROLE_VIEWER {
			t.Errorf("role %v", got.TokenScope["apppassword"].Role)
		}
	}
}

func TestLookupBySecretAlone(t *testing.T) {
	m, ctx, uid := newTestManager(t)
	pw, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()), "script", nil)
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
	for _, kind := range []string{scope.ClientCLI, scope.ClientWebDAV, scope.ClientMCP} {
		pw, err := m.GenerateAppPassword(ctx, appPasswordScope(t, kind, read), kind, nil)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		want := time.Now().AddDate(0, 0, 90).Unix()
		if got := int64(pw.GetExpiration().GetSeconds()); got < want-5 || got > want {
			t.Errorf("%s: expires at %d, want the 90 days default %d", kind, got, want)
		}
	}
	sync, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientSync, []string{scope.PermAll}), "sync", nil)
	if err != nil || sync.GetExpiration() != nil {
		t.Fatalf("a sync app password needs no expiry: %v, %v", sync.GetExpiration(), err)
	}

	for name, exp := range map[string]*typespb.Timestamp{"past": inDays(-1), "beyond the maximum": inDays(366)} {
		if _, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientWebDAV, read), name, exp); !isBadRequest(err) {
			t.Errorf("%s: want a bad request, got %v", name, err)
		}
	}

	// The store refuses an expired app password.
	pw, err := m.GenerateAppPassword(ctx, appPasswordScope(t, scope.ClientWebDAV, read), "short", inDays(1))
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
	pw, err = m2.GenerateAppPassword(ctx2, appPasswordScope(t, scope.ClientMCP, read), "mcp", nil)
	if err != nil || int64(pw.GetExpiration().GetSeconds()) > time.Now().AddDate(0, 0, 7).Unix() {
		t.Fatalf("want the configured default: %v, %v", pw.GetExpiration(), err)
	}
	if _, err := m2.GenerateAppPassword(ctx2, appPasswordScope(t, scope.ClientMCP, read), "mcp", inDays(31)); !isBadRequest(err) {
		t.Fatalf("want the configured maximum, got %v", err)
	}
}

func TestPolicy(t *testing.T) {
	db := filepath.Join(t.TempDir(), "appauth.db")
	m, _, _ := newManagerAt(t, db, nil)
	custom, _, _ := newManagerAt(t, db, map[string]any{"policy": map[string]any{"user_types": map[string]any{
		"lightweight": []string{scope.ClientMCP},
		"service":     []string{scope.ClientWebDAV},
	}}})
	read := scope.ReadOnlyPermissions()

	cases := []struct {
		mgr    appauth.Manager
		typ    userpb.UserType
		client string
		want   bool
	}{
		{m, userpb.UserType_USER_TYPE_PRIMARY, scope.ClientMCP, true},
		{m, userpb.UserType_USER_TYPE_SECONDARY, scope.ClientCLI, true},
		{m, userpb.UserType_USER_TYPE_SERVICE, scope.ClientWebDAV, true},
		{m, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientSync, false},
		{m, userpb.UserType_USER_TYPE_GUEST, scope.ClientWebDAV, false},
		{m, userpb.UserType_USER_TYPE_FEDERATED, scope.ClientMCP, false},
		{m, userpb.UserType_USER_TYPE_INVALID, scope.ClientSync, false},
		{custom, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientMCP, true},
		{custom, userpb.UserType_USER_TYPE_LIGHTWEIGHT, scope.ClientSync, false},
		{custom, userpb.UserType_USER_TYPE_SERVICE, scope.ClientMCP, false},
		// Types left out of the configuration keep their defaults.
		{custom, userpb.UserType_USER_TYPE_PRIMARY, scope.ClientSync, true},
	}
	for _, c := range cases {
		ctx := userContext(&userpb.UserId{OpaqueId: "u", Type: c.typ})
		perms := read
		if c.client == scope.ClientSync {
			perms = []string{scope.PermAll}
		}
		_, err := c.mgr.GenerateAppPassword(ctx, appPasswordScope(t, c.client, perms), "p", nil)
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
	both, err := scope.AddOwnerScope(appPasswordScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions()))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]map[string]*authpb.Scope{
		"owner scope":           owner,
		"owner scope alongside": both,
		"unknown client":        appPasswordScope(t, "printer", scope.ReadOnlyPermissions()),
		"no permission":         appPasswordScope(t, scope.ClientWebDAV, nil),
		"unknown permission":    appPasswordScope(t, scope.ClientWebDAV, []string{"files:steal"}),
		"wildcard and more":     appPasswordScope(t, scope.ClientSync, []string{scope.PermAll, scope.PermFilesList}),
		"relative root":         appPasswordScope(t, scope.ClientMCP, scope.ReadOnlyPermissions(), "thesis"),
		"roots outside mcp":     appPasswordScope(t, scope.ClientWebDAV, scope.ReadOnlyPermissions(), "/eos/user/j/jdoe/thesis"),
	}
	for name, scopes := range cases {
		if _, err := m.GenerateAppPassword(ctx, scopes, name, nil); !isBadRequest(err) {
			t.Errorf("%s: want a bad request, got %v", name, err)
		}
	}
}

// legacyAppPassword is the table as the store wrote it before app passwords
// had a client kind.
type legacyAppPassword struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     string `gorm:"size:64;index;not null"`
	TokenHash  []byte `gorm:"uniqueIndex;size:32;not null"`
	Label      string `gorm:"size:255"`
	ScopeJSON  []byte
	Ctime      time.Time `gorm:"not null"`
	Utime      time.Time `gorm:"not null"`
	Expiration *time.Time
}

func (legacyAppPassword) TableName() string { return "app_passwords" }

func insertLegacy(t *testing.T, db *gorm.DB, user, token string) {
	t.Helper()
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	scopeJSON, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	row := &legacyAppPassword{UserID: user, TokenHash: hashToken(token), Label: "laptop|Nextcloud Sync Client|c1", ScopeJSON: scopeJSON, Ctime: now, Utime: now}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
}

// Rows written before the migration, or by an older node during a rolling
// upgrade, become sync app passwords with every permission.
func TestMigratesLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appauth.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyAppPassword{}); err != nil {
		t.Fatal(err)
	}
	insertLegacy(t, db, "jdoe", "before-the-upgrade")
	insertLegacy(t, db, "jdoe", "another-one")

	m, ctx, _ := newManagerAt(t, path, nil)
	insertLegacy(t, db, "jdoe", "from-an-older-node")

	ids := map[string]bool{}
	for _, token := range []string{"before-the-upgrade", "another-one", "from-an-older-node"} {
		pw := mustGet(t, m, context.Background(), nil, token)
		ap, ok := scope.GetAppPasswordScope(pw.TokenScope)
		if !ok || len(pw.TokenScope) != 1 {
			t.Fatalf("%s: scope %v", token, pw.TokenScope)
		}
		if ap.Client != scope.ClientSync || !slices.Equal(ap.Permissions, []string{scope.PermAll}) || len(ap.Roots) != 0 || ap.ExpiresAt != 0 {
			t.Errorf("%s: scope %+v", token, ap)
		}
		if _, err := uuid.Parse(ap.PasswordID); err != nil || ids[ap.PasswordID] {
			t.Errorf("%s: password id %q is not a fresh UUID", token, ap.PasswordID)
		}
		ids[ap.PasswordID] = true
		if pw.GetUser().GetOpaqueId() != "jdoe" || pw.Label != "laptop|Nextcloud Sync Client|c1" {
			t.Errorf("%s: %+v", token, pw)
		}
	}

	var rows []model.AppPassword
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.CreatedBy != "jdoe" || r.Client != scope.ClientSync || len(r.ScopeJSON) == 0 {
			t.Errorf("row %d: %+v", r.ID, r)
		}
	}
	if list, err := m.ListAppPasswords(ctx); err != nil || len(list) != 3 {
		t.Fatalf("list: %d, %v", len(list), err)
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
