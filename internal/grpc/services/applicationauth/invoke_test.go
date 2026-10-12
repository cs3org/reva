// Copyright 2018-2024 CERN
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

package applicationauth

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/sql"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/invoke"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

var (
	root  = &userpb.User{Id: &userpb.UserId{OpaqueId: "admin-id"}, Username: "admin"}
	guest = &userpb.User{Id: &userpb.UserId{OpaqueId: "guest-id", Type: userpb.UserType_USER_TYPE_GUEST}, Username: "guest"}
	marie = &userpb.User{Id: &userpb.UserId{OpaqueId: "marie-id", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "marie"}
)

type fakeGateway struct{ gateway.GatewayAPIClient }

func (fakeGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	for _, u := range []*userpb.User{root, guest, marie} {
		if req.GetClaim() == "username" && req.GetValue() == u.Username {
			return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
		}
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "not found"}}, nil
}

type resolver struct{ revaservice.Clients }

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return fakeGateway{}, nil }

func init() {
	revaservice.SetGlobal(resolver{})
}

func newService(t *testing.T) *service {
	t.Helper()
	am, err := sql.New(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "appauth.db")})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{conf: &config{}, am: am}
	s.registerInvocations()
	return s
}

func adminContext() context.Context {
	return appctx.ContextSetUser(context.Background(), root)
}

func TestAdminCreatesForAnyUser(t *testing.T) {
	s := newService(t)
	res, err := s.Invoke(adminContext(), "create", map[string]any{
		"user": "guest", "client": "cli", "permissions": "files:list, files:read", "expires_in": "3600", "label": "assistant",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res["client"] != "cli" || res["created_by"] != "admin-id" || res["password"] == "" || res["expires_at"] == nil {
		t.Fatalf("result %v", res)
	}

	// The guest can use it, and sees who created it.
	pw, err := s.am.GetAppPassword(context.Background(), nil, res["password"].(string))
	if err != nil || pw.GetUser().GetOpaqueId() != "guest-id" {
		t.Fatalf("get: %v, %v", pw, err)
	}
	list, err := s.ListAppPasswords(appctx.ContextSetUser(context.Background(), guest), &appauthpb.ListAppPasswordsRequest{})
	if err != nil || len(list.AppPasswords) != 1 {
		t.Fatalf("list: %v, %v", list, err)
	}
	var creators map[string]string
	if err := json.Unmarshal(list.GetOpaque().GetMap()[appauth.OpaqueCreatedBy].GetValue(), &creators); err != nil {
		t.Fatal(err)
	}
	if creators[res["password_id"].(string)] != "admin-id" {
		t.Fatalf("creators %v", creators)
	}
}

func TestAdminListsAndRevokes(t *testing.T) {
	s := newService(t)
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{Client: scope.ClientWebDAV, Permissions: scope.ReadOnlyPermissions()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pw, err := s.am.GenerateAppPassword(appctx.ContextSetUser(context.Background(), marie), scopes, "script", nil)
	if err != nil {
		t.Fatal(err)
	}
	ap, _ := scope.GetAppAuthScope(pw.TokenScope)

	res, err := s.Invoke(adminContext(), "list", map[string]any{"user": "marie"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	listed := res["app_passwords"].([]invoke.Result)
	if len(listed) != 1 || listed[0]["password_id"] != ap.PasswordID || listed[0]["created_by"] != "marie-id" || listed[0]["password"] != nil {
		t.Fatalf("listed %v", listed)
	}

	if _, err := s.Invoke(adminContext(), "revoke", map[string]any{"user": "marie", "password_id": ap.PasswordID}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.am.GetAppPassword(context.Background(), nil, pw.Password); err == nil {
		t.Fatal("a revoked app password must not be found")
	}
	if _, err := s.Invoke(adminContext(), "list", map[string]any{"user": "nobody"}); err == nil {
		t.Fatal("an unknown user must be reported")
	}
}
