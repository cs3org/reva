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
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var directory = map[string]*userpb.User{
	"alice": {Id: &userpb.UserId{Idp: "idp", OpaqueId: "alice", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "alice", Groups: []string{"users"}},
	"boss":  {Id: &userpb.UserId{Idp: "idp", OpaqueId: "boss", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "boss", Groups: []string{"users", "reva_admins"}},
	// Without groups on the user, the groups come from GetUserGroups.
	"quiet":  {Id: &userpb.UserId{Idp: "idp", OpaqueId: "quiet", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "quiet"},
	"robot":  {Id: &userpb.UserId{Idp: "idp", OpaqueId: "robot", Type: userpb.UserType_USER_TYPE_SERVICE}, Username: "robot", Groups: []string{"users"}},
	"guest1": {Id: &userpb.UserId{Idp: "idp", OpaqueId: "guest1", Type: userpb.UserType_USER_TYPE_LIGHTWEIGHT}, Username: "guest1", Groups: []string{"users"}},
	"remote": {Id: &userpb.UserId{Idp: "idp", OpaqueId: "remote", Type: userpb.UserType_USER_TYPE_FEDERATED}, Username: "remote", Groups: []string{"users"}},
}

type fakeDirectory struct{ gateway.GatewayAPIClient }

func (fakeDirectory) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if u, ok := directory[req.Value]; ok {
		return &userpb.GetUserByClaimResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserByClaimResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_NOT_FOUND, Message: "no such user"}}, nil
}

func (fakeDirectory) GetUserGroups(_ context.Context, req *userpb.GetUserGroupsRequest, _ ...grpc.CallOption) (*userpb.GetUserGroupsResponse, error) {
	groups := []string{"users"}
	if req.UserId.OpaqueId == "quiet" {
		groups = append(groups, "reva_admins")
	}
	return &userpb.GetUserGroupsResponse{Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK}, Groups: groups}, nil
}

type directoryClients struct{ service.Clients }

func (directoryClients) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	return fakeDirectory{}, nil
}

func impersonationService(t *testing.T) *svc {
	t.Helper()
	service.SetGlobal(directoryClients{})
	s, err := New(context.Background(), map[string]any{
		"admin_group":         "reva_admins",
		"socket":              socketOff,
		"machine_auth_apikey": "machine-secret",
		"impersonation_ttl":   "12h",
		"token_managers":      map[string]any{"jwt": map[string]any{"secret": "testsecret"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.(*svc)
}

// adminContext is the context of an admin caller. The audit lines go to the returned buffer.
func adminContext(t *testing.T, opaqueID string) (context.Context, *bytes.Buffer) {
	t.Helper()
	scopes, err := scope.AddAdminScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	l := zerolog.New(&buf)
	ctx := appctx.WithLogger(context.Background(), &l)
	ctx = appctx.ContextSetUser(ctx, &userpb.User{Id: &userpb.UserId{Idp: "idp", OpaqueId: opaqueID}, Username: opaqueID, Groups: []string{"reva_admins"}})
	ctx = appctx.ContextSetScopes(ctx, scopes)
	return ctx, &buf
}

func TestImpersonatedTokenCarriesActor(t *testing.T) {
	s := impersonationService(t)
	ctx, audit := adminContext(t, "gdelmont")

	res, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "alice", Reason: "INC42"})
	if err != nil {
		t.Fatalf("Impersonate: %v", err)
	}
	u, scopes, err := s.userTokenManager.DismantleToken(context.Background(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice" || scope.Actor(scopes) != "gdelmont" {
		t.Fatalf("token for %q with actor %q, want alice and gdelmont", u.Username, scope.Actor(scopes))
	}
	if _, ok := scopes["user"]; !ok {
		t.Errorf("a primary target must get the owner scope: %v", scopes)
	}
	for _, want := range []string{`"action":"impersonate"`, `"actor":"gdelmont"`, `"target":"alice"`, `"granted":true`, `"reason":"INC42"`} {
		if !strings.Contains(audit.String(), want) {
			t.Errorf("audit %s has no %s", audit, want)
		}
	}

	// A login goes through an auth manager, and gets no actor.
	_, login, err := s.machineAuth.Authenticate(ctx, "alice", "machine-secret")
	if err != nil {
		t.Fatal(err)
	}
	if a := scope.Actor(login); a != "" {
		t.Errorf("a login token carries the actor %q", a)
	}
}

func TestImpersonateOverTheSocketRecordsTheUID(t *testing.T) {
	s := impersonationService(t)
	ctx, _ := adminContext(t, "uid:0")
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: peerCredAuthInfo{cred: peerCred{uid: 0}}})

	res, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "alice"})
	if err != nil {
		t.Fatalf("Impersonate: %v", err)
	}
	_, scopes, err := s.userTokenManager.DismantleToken(context.Background(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.Actor(scopes); got != "socket:0" {
		t.Fatalf("actor %q, want socket:0", got)
	}
}

func TestImpersonatedTokenCannotLaunder(t *testing.T) {
	s := impersonationService(t)
	ctx, _ := adminContext(t, "gdelmont")
	res, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "boss", AllowPrivileged: true})
	if err != nil {
		t.Fatalf("Impersonate: %v", err)
	}
	ctx = appctx.ContextSetToken(context.Background(), res.Token)

	if _, err := s.RequestAdmin(ctx, &adminpb.RequestAdminRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("RequestAdmin with an impersonated token: %v, want PermissionDenied", err)
	}
	if c, err := s.CheckAdmin(ctx, &adminpb.CheckAdminRequest{}); err != nil || c.Admin {
		t.Errorf("CheckAdmin with an impersonated token = %v, %v; want not admin", c, err)
	}

	u, scopes, err := s.userTokenManager.DismantleToken(context.Background(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	// Even with the admin scope added, the actor stops a second impersonation.
	scopes, err = scope.AddAdminScope(scopes)
	if err != nil {
		t.Fatal(err)
	}
	ctx = appctx.ContextSetScopes(appctx.ContextSetUser(context.Background(), u), scopes)
	if _, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "alice"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("Impersonate with an impersonated token: %v, want PermissionDenied", err)
	}
}

func TestImpersonatePrivilegedTargets(t *testing.T) {
	s := impersonationService(t)
	for _, target := range []string{"boss", "quiet", "robot"} {
		t.Run(target, func(t *testing.T) {
			ctx, audit := adminContext(t, "gdelmont")
			_, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: target})
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("without allow_privileged: %v, want PermissionDenied", err)
			}
			if !strings.Contains(audit.String(), `"action":"impersonate_privileged"`) || !strings.Contains(audit.String(), `"granted":false`) {
				t.Errorf("refusal not audited: %s", audit)
			}

			ctx, audit = adminContext(t, "gdelmont")
			if _, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: target, AllowPrivileged: true}); err != nil {
				t.Fatalf("with allow_privileged: %v", err)
			}
			var lines []string
			for l := range strings.SplitSeq(audit.String(), "\n") {
				if strings.Contains(l, `"audit":true`) {
					lines = append(lines, l)
				}
			}
			if len(lines) != 2 || !strings.Contains(lines[0], `"action":"impersonate_privileged"`) || !strings.Contains(lines[0], `"granted":true`) {
				t.Errorf("want the privileged impersonation on its own audit line, got %s", audit)
			}
		})
	}
}

func TestImpersonateGivesTheScopesOfTheLogin(t *testing.T) {
	s := impersonationService(t)
	ctx, _ := adminContext(t, "gdelmont")

	res, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "guest1"})
	if err != nil {
		t.Fatalf("Impersonate: %v", err)
	}
	_, scopes, err := s.userTokenManager.DismantleToken(context.Background(), res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scopes["lightweight"]; !ok {
		t.Errorf("a lightweight target must get the lightweight scope: %v", scopes)
	}
	if _, ok := scopes["user"]; ok {
		t.Errorf("a lightweight target must not get the owner scope: %v", scopes)
	}

	if _, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "remote"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("federated target: %v, want InvalidArgument", err)
	}
}

func TestImpersonateMissingUser(t *testing.T) {
	s := impersonationService(t)
	ctx, _ := adminContext(t, "gdelmont")
	if _, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "nobody"}); status.Code(err) != codes.NotFound {
		t.Fatalf("missing target: %v, want NotFound", err)
	}
}

func TestImpersonateTTL(t *testing.T) {
	s := impersonationService(t)
	ctx, _ := adminContext(t, "gdelmont")
	for _, tc := range []struct {
		ttl, want time.Duration
	}{
		{0, 12 * time.Hour},
		{time.Hour, time.Hour},
		{48 * time.Hour, 12 * time.Hour},
	} {
		res, err := s.Impersonate(ctx, &adminpb.ImpersonateRequest{User: "alice", TtlSeconds: int64(tc.ttl.Seconds())})
		if err != nil {
			t.Fatalf("ttl %s: %v", tc.ttl, err)
		}
		exp, err := s.userTokenManager.(token.ValidatedExpiry).ValidatedExpiresAt(context.Background(), res.Token)
		if err != nil {
			t.Fatal(err)
		}
		if res.ExpiresAt != exp.Unix() {
			t.Errorf("ttl %s: expires_at %d, token expires %d", tc.ttl, res.ExpiresAt, exp.Unix())
		}
		if got := time.Until(exp); got < tc.want-time.Minute || got > tc.want {
			t.Errorf("ttl %s: token lives %s, want %s", tc.ttl, got.Round(time.Minute), tc.want)
		}
		if _, scopes, _ := s.userTokenManager.DismantleToken(context.Background(), res.Token); scope.Actor(scopes) != "gdelmont" {
			t.Errorf("ttl %s: the token lost the actor", tc.ttl)
		}
	}
}
