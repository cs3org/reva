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

package admin

import (
	"context"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	gojwt "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testService(t *testing.T) (*svc, token.Manager) {
	t.Helper()
	tm, err := jwt.New(map[string]any{"secret": "testsecret", "expires": int64(900)})
	if err != nil {
		t.Fatalf("jwt.New: %v", err)
	}
	return &svc{adminGroup: "reva_admins", adminTTL: 15 * time.Minute, tokenManager: tm}, tm
}

func userToken(t *testing.T, tm token.Manager, u *userpb.User) string {
	t.Helper()
	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope: %v", err)
	}
	tkn, err := tm.MintToken(context.Background(), u, scopes)
	if err != nil {
		t.Fatalf("MintToken: %v", err)
	}
	return tkn
}

// TestRequestAdminGroupGating asserts that only members of admin_group can step
// up, and that the minted token is admin-only (scope {admin}, no user key).
func TestRequestAdminGroupGating(t *testing.T) {
	s, tm := testService(t)

	member := &userpb.User{Id: &userpb.UserId{OpaqueId: "u1", Idp: "idp"}, Username: "alice", Groups: []string{"reva_admins"}}
	ctx := appctx.ContextSetToken(context.Background(), userToken(t, tm, member))
	res, err := s.RequestAdmin(ctx, &adminpb.RequestAdminRequest{})
	if err != nil {
		t.Fatalf("member RequestAdmin: unexpected error %v", err)
	}
	if res.Token == "" {
		t.Fatal("member RequestAdmin: empty admin token")
	}

	// The minted token must be admin-only.
	_, scopes, err := tm.DismantleToken(context.Background(), res.Token)
	if err != nil {
		t.Fatalf("DismantleToken: %v", err)
	}
	if _, ok := scopes["admin"]; !ok {
		t.Errorf("minted token has no admin scope: %v", scopes)
	}
	if _, ok := scopes["user"]; ok {
		t.Errorf("minted token must not carry a user scope: %v", scopes)
	}
}

func TestRequestAdminDeniesNonMember(t *testing.T) {
	s, tm := testService(t)

	outsider := &userpb.User{Id: &userpb.UserId{OpaqueId: "u2", Idp: "idp"}, Username: "bob", Groups: []string{"users"}}
	ctx := appctx.ContextSetToken(context.Background(), userToken(t, tm, outsider))
	_, err := s.RequestAdmin(ctx, &adminpb.RequestAdminRequest{})
	if err == nil {
		t.Fatal("non-member RequestAdmin: expected error, got nil")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("non-member RequestAdmin: expected PermissionDenied, got %v", status.Code(err))
	}
}

// TestCheckAdmin asserts that CheckAdmin reports group membership without
// minting anything, for members and non-members alike.
func TestCheckAdmin(t *testing.T) {
	s, tm := testService(t)

	for _, tc := range []struct {
		groups []string
		want   bool
	}{
		{[]string{"users", "reva_admins"}, true},
		{[]string{"users"}, false},
	} {
		u := &userpb.User{Id: &userpb.UserId{OpaqueId: "u3", Idp: "idp"}, Username: "carol", Groups: tc.groups}
		ctx := appctx.ContextSetToken(context.Background(), userToken(t, tm, u))
		res, err := s.CheckAdmin(ctx, &adminpb.CheckAdminRequest{})
		if err != nil {
			t.Fatalf("CheckAdmin(%v): unexpected error %v", tc.groups, err)
		}
		if res.Admin != tc.want {
			t.Errorf("CheckAdmin(%v) = %v, want %v", tc.groups, res.Admin, tc.want)
		}
	}
}

// TestTokenLifetimes asserts that impersonation tokens outlive admin tokens: an
// impersonated session runs transfers like any other, which a 15-minute token
// would cut off. Without impersonation_ttl it lives as long as a token from
// signing in; with it, as long as it says; and the token manager's own
// expires, when set, wins over both.
func TestTokenLifetimes(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		tms  map[string]map[string]any
		want time.Duration
	}{
		{"admin ttl", 15 * time.Minute, nil, 15 * time.Minute},
		{"manager default", 0, nil, 24 * time.Hour},
		{"impersonation ttl", 12 * time.Hour, nil, 12 * time.Hour},
		{"manager expires wins", 12 * time.Hour, map[string]map[string]any{"jwt": {"expires": int64(3600)}}, time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := config{TokenManager: "jwt", TokenManagers: tc.tms}
			if c.TokenManagers == nil {
				c.TokenManagers = map[string]map[string]any{"jwt": {}}
			}
			c.TokenManagers["jwt"]["secret"] = "testsecret"
			tm, err := newTokenManager(c, tc.ttl)
			if err != nil {
				t.Fatalf("newTokenManager: %v", err)
			}
			u := &userpb.User{Id: &userpb.UserId{OpaqueId: "u4", Idp: "idp"}, Username: "dave"}
			tkn, err := tm.MintToken(context.Background(), u, nil)
			if err != nil {
				t.Fatalf("MintToken: %v", err)
			}
			var claims gojwt.RegisteredClaims
			if _, _, err := gojwt.NewParser().ParseUnverified(tkn, &claims); err != nil {
				t.Fatalf("parsing token: %v", err)
			}
			got := time.Until(claims.ExpiresAt.Time)
			if got < tc.want-time.Minute || got > tc.want {
				t.Errorf("token lives %s, want %s", got.Round(time.Minute), tc.want)
			}
		})
	}
}
