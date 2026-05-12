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

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc/metadata"
)

func TestCtxWithUserInfoStoresScopes(t *testing.T) {
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "example.org"}}
	scopes, err := authscope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/files/test.txt", nil)
	req.Header.Set("User-Agent", "dav-client")

	ctx := ctxWithUserInfo(context.Background(), req, user, "token-123", scopes)

	gotUser, ok := appctx.ContextGetUser(ctx)
	if !ok || gotUser.GetId().GetOpaqueId() != "einstein" {
		t.Fatalf("ContextGetUser() = %+v, %t, want einstein", gotUser, ok)
	}
	gotToken, ok := appctx.ContextGetToken(ctx)
	if !ok || gotToken != "token-123" {
		t.Fatalf("ContextGetToken() = %q, %t, want token-123", gotToken, ok)
	}
	gotScopes, ok := appctx.ContextGetScopes(ctx)
	if !ok || !reflect.DeepEqual(gotScopes, scopes) {
		t.Fatalf("ContextGetScopes() = %+v, %t, want %+v", gotScopes, ok, scopes)
	}

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata in context")
	}
	if got := md.Get(appctx.TokenHeader); len(got) != 1 || got[0] != "token-123" {
		t.Fatalf("outgoing %s metadata = %v, want [token-123]", appctx.TokenHeader, got)
	}
	if got := md.Get(appctx.UserAgentHeader); len(got) != 1 || got[0] != "dav-client" {
		t.Fatalf("outgoing %s metadata = %v, want [dav-client]", appctx.UserAgentHeader, got)
	}
}

func TestIsTokenValidReturnsScopes(t *testing.T) {
	tokenManager, err := jwt.New(map[string]any{
		"secret":  "test-secret-auth",
		"expires": int64(3600),
	})
	if err != nil {
		t.Fatalf("jwt.New returned error: %v", err)
	}

	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "example.org"}}
	scopes, err := authscope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope returned error: %v", err)
	}

	token, err := tokenManager.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/ocm/share123", nil)
	gotUser, gotScopes, ok := isTokenValid(req, tokenManager, token)
	if !ok {
		t.Fatal("isTokenValid() returned false")
	}
	if gotUser.GetId().GetOpaqueId() != "einstein" {
		t.Fatalf("isTokenValid() user = %q, want einstein", gotUser.GetId().GetOpaqueId())
	}
	if !reflect.DeepEqual(gotScopes, scopes) {
		t.Fatalf("isTokenValid() scopes = %+v, want %+v", gotScopes, scopes)
	}
}

func TestGetCredsForUserAgent(t *testing.T) {
	type test struct {
		userAgent            string
		userAgentMap         map[string]string
		availableCredentials []string
		expected             []string
	}

	tests := []*test{
		// no user agent we return all available credentials
		{
			userAgent:            "",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// map set but user agent not in map
		{
			userAgent:            "curl",
			userAgentMap:         map[string]string{"mirall": "basic"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"basic", "bearer"},
		},

		// no user map we return all available credentials
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// user agent set but no mapping set we return all credentials
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// user mapping set to non available credential, we return all available
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{"mirall": "notfound"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"basic", "bearer"},
		},

		// user mapping set and we return only desired credential
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{"mirall": "bearer"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"bearer"},
		},
	}

	for _, test := range tests {
		got := getCredsForUserAgent(
			test.userAgent,
			test.userAgentMap,
			test.availableCredentials)

		if !match(got, test.expected) {
			fail(t, got, test.expected)
		}
	}
}

func match(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func fail(t *testing.T, got, expected []string) {
	t.Fatalf("got: %+v expected: %+v", got, expected)
}
