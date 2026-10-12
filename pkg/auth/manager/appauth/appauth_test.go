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

package appauth

import (
	"context"
	"testing"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

var (
	einstein = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "einstein"}
	guest    = &userpb.User{Id: &userpb.UserId{OpaqueId: "guest", Type: userpb.UserType_USER_TYPE_LIGHTWEIGHT}, Username: "guest"}
)

// fakeGateway knows one app password per user.
type fakeGateway struct {
	gateway.GatewayAPIClient
	lookups []*appauthpb.GetAppPasswordRequest
}

var ok = &rpc.Status{Code: rpc.Code_CODE_OK}

func (g *fakeGateway) users() map[string]*userpb.User {
	return map[string]*userpb.User{einstein.Id.OpaqueId: einstein, guest.Id.OpaqueId: guest}
}

func (g *fakeGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	if u, found := g.users()[req.GetUserId().GetOpaqueId()]; found {
		return &userpb.GetUserResponse{Status: ok, User: u}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *fakeGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if u, found := g.users()[req.GetValue()]; found {
		return &userpb.GetUserByClaimResponse{Status: ok, User: u}, nil
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *fakeGateway) GetAppPassword(_ context.Context, req *appauthpb.GetAppPasswordRequest, _ ...grpc.CallOption) (*appauthpb.GetAppPasswordResponse, error) {
	g.lookups = append(g.lookups, req)
	owner := map[string]string{"einstein-secret": "einstein", "guest-secret": "guest"}[req.GetPassword()]
	if owner == "" || (req.GetUser() != nil && req.GetUser().GetOpaqueId() != owner) {
		return &appauthpb.GetAppPasswordResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{PasswordID: owner + "-pw", Client: scope.ClientWebDAV, Permissions: scope.ReadOnlyPermissions()}, nil)
	if err != nil {
		return nil, err
	}
	return &appauthpb.GetAppPasswordResponse{Status: ok, AppPassword: &appauthpb.AppPassword{
		TokenScope: scopes,
		User:       &userpb.UserId{OpaqueId: owner},
	}}, nil
}

type resolver struct {
	service.Clients
	gw *fakeGateway
}

func (r *resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return r.gw, nil }

var gw = &fakeGateway{}

func init() {
	service.SetGlobal(&resolver{gw: gw})
}

func TestAuthenticateWithoutUsername(t *testing.T) {
	m := &manager{}
	gw.lookups = nil
	u, scopes, err := m.Authenticate(context.Background(), "", "einstein-secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if u.GetUsername() != "einstein" || u.GetId().GetType() != userpb.UserType_USER_TYPE_PRIMARY {
		t.Fatalf("user %+v", u)
	}
	ap, found := scope.GetAppAuthScope(scopes)
	if !found || ap.PasswordID != "einstein-pw" || ap.Lightweight {
		t.Fatalf("scope %+v", ap)
	}
	if len(gw.lookups) != 1 || gw.lookups[0].GetUser() != nil {
		t.Fatalf("the app password must be looked up by itself: %+v", gw.lookups)
	}

	if _, _, err := m.Authenticate(context.Background(), "", "wrong"); err == nil {
		t.Fatal("a wrong app password must be refused")
	} else if _, isNotFound := err.(errtypes.NotFound); !isNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestAuthenticateWithUsername(t *testing.T) {
	m := &manager{}
	gw.lookups = nil
	u, _, err := m.Authenticate(context.Background(), "einstein", "einstein-secret")
	if err != nil || u.GetUsername() != "einstein" {
		t.Fatalf("authenticate: %v, %v", u, err)
	}
	if len(gw.lookups) != 1 || gw.lookups[0].GetUser().GetOpaqueId() != "einstein" {
		t.Fatalf("the app password must be looked up for the user: %+v", gw.lookups)
	}
	if _, _, err := m.Authenticate(context.Background(), "guest", "einstein-secret"); err == nil {
		t.Fatal("another user's app password must be refused")
	}
}

// The app password of a lightweight account is held to the lightweight rules.
func TestAuthenticateLightweight(t *testing.T) {
	m := &manager{}
	for _, username := range []string{"", "guest"} {
		_, scopes, err := m.Authenticate(context.Background(), username, "guest-secret")
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if ap, found := scope.GetAppAuthScope(scopes); !found || !ap.Lightweight {
			t.Fatalf("username %q: scope %+v", username, ap)
		}
	}
}
