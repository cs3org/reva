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

package scope_test

import (
	"context"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
)

const testSecret = "scope-actor-test-secret"

func verify(t *testing.T, scopes map[string]*authpb.Scope, resource any) bool {
	t.Helper()
	ok, err := scope.VerifyScope(context.Background(), scopes, resource)
	if err != nil {
		t.Fatalf("VerifyScope: %v", err)
	}
	return ok
}

func TestActorScopeGrantsNothing(t *testing.T) {
	actorOnly := scope.AddActorScope("gdelmont", nil)
	for _, r := range []any{&provider.StatRequest{}, &adminpb.ImpersonateRequest{}, &adminpb.RequestAdminRequest{}, "/remote.php/webdav"} {
		if verify(t, actorOnly, r) {
			t.Errorf("the actor scope alone authorizes %T", r)
		}
	}

	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	impersonated := scope.AddActorScope("gdelmont", owner)
	if !verify(t, impersonated, &provider.StatRequest{}) {
		t.Error("an impersonated owner token must still act as the user")
	}
	if verify(t, impersonated, &adminpb.ImpersonateRequest{}) {
		t.Error("an impersonated owner token must not satisfy an admin request")
	}
	if got := scope.Actor(impersonated); got != "gdelmont" {
		t.Errorf("Actor = %q, want gdelmont", got)
	}
	if owner, _ := scope.AddOwnerScope(nil); scope.Actor(owner) != "" {
		t.Errorf("Actor of a token that is not impersonated = %q", scope.Actor(owner))
	}
}

// A JWT claim would be lost on a cache hit and on a machine re-mint. The scope is not.
func TestActorSurvivesCacheAndReMint(t *testing.T) {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: testSecret})
	if sharedconf.GetJWTSecret("") != testSecret {
		t.Skip("shared config already initialised with another secret")
	}
	tm, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatal(err)
	}
	u := &userpb.User{Id: &userpb.UserId{Idp: "idp", OpaqueId: "alice", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "alice"}
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	tkn, err := tm.MintToken(context.Background(), u, scope.AddActorScope("gdelmont", owner))
	if err != nil {
		t.Fatal(err)
	}

	var scopes map[string]*authpb.Scope
	for _, pass := range []string{"miss", "hit"} {
		if _, scopes, err = tm.DismantleToken(context.Background(), tkn); err != nil {
			t.Fatal(err)
		}
		if got := scope.Actor(scopes); got != "gdelmont" {
			t.Fatalf("cache %s: actor %q, want gdelmont", pass, got)
		}
	}

	ctx := appctx.ContextSetUser(context.Background(), u)
	ctx = appctx.ContextSetScopes(ctx, scopes)
	ctx, err = scope.ContextWithMachineScope(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reminted, _ := appctx.ContextGetToken(ctx)
	_, scopes, err = tm.DismantleToken(context.Background(), reminted)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scopes[scope.MachineScope]; !ok {
		t.Fatalf("re-minted token has no machine scope: %v", scopes)
	}
	if got := scope.Actor(scopes); got != "gdelmont" {
		t.Fatalf("re-minted token: actor %q, want gdelmont", got)
	}
}

func TestMachineScopeDeclinesAdminRequests(t *testing.T) {
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := scope.AddMachineScope(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []any{&adminpb.ImpersonateRequest{}, &adminpb.InvokeRequest{}, scope.MethodResource("/reva.admin.v1beta1.AdminAPI/InvokeStream")} {
		if verify(t, machine, r) {
			t.Errorf("a machine-scoped token satisfies the admin request %v", r)
		}
	}
	if !verify(t, machine, &provider.StatRequest{}) {
		t.Error("a machine-scoped token must still satisfy a CS3 request")
	}
}
