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
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc/metadata"
)

func TestAsReplacesTheAdminToken(t *testing.T) {
	id, err := NewIdentity("secret")
	if err != nil {
		t.Fatal(err)
	}
	admin := &userpb.User{Id: &userpb.UserId{OpaqueId: "operator"}, Username: "operator"}
	ctx := appctx.ContextSetUser(context.Background(), admin)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, "admin-token", "x-other", "kept")

	marie := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "cernbox"}, Username: "marie"}
	uctx, err := id.As(ctx, marie)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := appctx.ContextGetUser(uctx); !ok || u.Username != "marie" {
		t.Fatalf("context user = %v, want marie", u)
	}
	md, _ := metadata.FromOutgoingContext(uctx)
	tokens := md.Get(appctx.TokenHeader)
	if len(tokens) != 1 || tokens[0] == "admin-token" {
		t.Fatalf("outgoing tokens = %v, want exactly the minted user token", tokens)
	}
	if got := md.Get("x-other"); len(got) != 1 || got[0] != "kept" {
		t.Fatalf("other metadata lost: %v", md)
	}
	if tkn, ok := appctx.ContextGetToken(uctx); !ok || tkn != tokens[0] {
		t.Fatalf("context token differs from the outgoing one")
	}

	// the token verifies with the same secret and carries the owner scope
	mgr, err := jwt.New(map[string]any{"secret": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	u, scopes, err := mgr.DismantleToken(context.Background(), tokens[0])
	if err != nil || u.Username != "marie" {
		t.Fatalf("dismantle: %v, %v", u, err)
	}
	if _, ok := scopes["user"]; !ok || scope.HasAdminScope(scopes) {
		t.Fatalf("scopes = %v, want the owner scope only", scopes)
	}

	// the original context is untouched
	if u, _ := appctx.ContextGetUser(ctx); u.Username != "operator" {
		t.Fatalf("original context changed: %v", u)
	}
	if Actor(ctx) != "operator" || Actor(context.Background()) != "unknown" {
		t.Fatalf("Actor: %q, %q", Actor(ctx), Actor(context.Background()))
	}
}

func TestNewIdentityNeedsASecret(t *testing.T) {
	if _, err := NewIdentity(""); err == nil {
		t.Fatal("an empty secret must be refused")
	}
	var nilID *Identity
	if _, err := nilID.As(context.Background(), &userpb.User{}); err == nil {
		t.Fatal("a nil identity must refuse to act")
	}
}

func TestParseTime(t *testing.T) {
	for _, v := range []string{"", "none", "NONE", "never"} {
		if _, none, err := ParseTime(v); err != nil || !none {
			t.Errorf("%q: none=%v err=%v", v, none, err)
		}
	}
	got, none, err := ParseTime("2026-03-01")
	if err != nil || none {
		t.Fatalf("date: %v %v", none, err)
	}
	if want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Fatalf("date = %v, want %v", got, want)
	}
	got, _, err = ParseTime("2026-03-01T10:00:00Z")
	if err != nil || !got.Equal(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("rfc3339 = %v, %v", got, err)
	}
	if _, _, err := ParseTime("tomorrow"); err == nil {
		t.Fatal("garbage must be rejected")
	}
	if FormatTime(time.Date(2026, 3, 1, 12, 0, 0, 0, time.FixedZone("x", 3600))) != "2026-03-01T11:00:00Z" {
		t.Fatal("FormatTime must render UTC RFC 3339")
	}
}
