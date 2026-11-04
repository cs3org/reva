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
	"maps"
	"slices"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/internal/grpc/interceptors/auth"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testSecret = "admin-service-test-secret"

// fakeGateway lets the interceptor resolve a gateway when it refuses a token;
// it knows only the user marie and any other call panics.
type fakeGateway struct{ service.Clients }

func (fakeGateway) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	return userGateway{}, nil
}

type userGateway struct{ gateway.GatewayAPIClient }

func (userGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	if id := req.GetUserId(); id.GetOpaqueId() == "marie" && (id.GetIdp() == "" || id.GetIdp() == "cernbox") {
		return &userpb.GetUserResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			User:   &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}, Username: "marie", Groups: []string{"physics"}},
		}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (userGateway) GetUserGroups(context.Context, *userpb.GetUserGroupsRequest, ...grpc.CallOption) (*userpb.GetUserGroupsResponse, error) {
	return &userpb.GetUserGroupsResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Groups: []string{"physics"}}, nil
}

func setup(t *testing.T) {
	t.Helper()
	sharedconf.Init(&revaconfig.Shared{JWTSecret: testSecret, SkipUserGroupsInToken: true})
	if sharedconf.GetJWTSecret("") != testSecret {
		t.Skip("shared config already initialised with another secret")
	}
	service.SetGlobal(fakeGateway{})
	serviceTokens = map[string]serviceToken{}
	now = time.Now
	t.Cleanup(func() { now = time.Now })
}

func TestAsService(t *testing.T) {
	setup(t)
	ctx := appctx.ContextSetToken(context.Background(), "user-token")
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, "user-token")

	actx, err := AsService(ctx, "reminders")
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(actx)
	sent := md.Get(appctx.TokenHeader)
	if len(sent) != 1 || sent[0] == "user-token" {
		t.Fatalf("expected the user token replaced, got %v", sent)
	}
	if tkn, _ := appctx.ContextGetToken(actx); tkn != sent[0] {
		t.Fatalf("context token %q, outgoing %q", tkn, sent[0])
	}
	// the caller keeps its own token for its other calls
	if md, _ := metadata.FromOutgoingContext(ctx); !slices.Equal(md.Get(appctx.TokenHeader), []string{"user-token"}) {
		t.Fatalf("caller context changed: %v", md)
	}

	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatal(err)
	}
	u, scopes, err := mgr.DismantleToken(context.Background(), sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if u.GetUsername() != "service:reminders" || u.GetId().GetOpaqueId() != "service:reminders" || u.GetId().GetIdp() != "reva" || u.GetId().GetType() != userpb.UserType_USER_TYPE_SERVICE {
		t.Fatalf("unexpected identity %+v", u)
	}
	if keys := slices.Collect(maps.Keys(scopes)); !slices.Equal(keys, []string{"admin"}) {
		t.Fatalf("expected only the admin scope, got %v", keys)
	}
}

func TestAsServiceReusesTheToken(t *testing.T) {
	setup(t)
	t0 := time.Now()
	now = func() time.Time { return t0 }
	if _, err := AsService(context.Background(), "reminders"); err != nil {
		t.Fatal(err)
	}
	first := serviceTokens["reminders"]

	now = func() time.Time { return t0.Add(2 * time.Minute) }
	if _, err := AsService(context.Background(), "reminders"); err != nil {
		t.Fatal(err)
	}
	if serviceTokens["reminders"] != first {
		t.Fatal("expected the token reused while more than half of its lifetime is left")
	}

	now = func() time.Time { return t0.Add(3 * time.Minute) }
	if _, err := AsService(context.Background(), "reminders"); err != nil {
		t.Fatal(err)
	}
	if got := serviceTokens["reminders"].expires; !got.Equal(t0.Add(8 * time.Minute)) {
		t.Fatalf("expected a new token expiring at %v, got %v", t0.Add(8*time.Minute), got)
	}
}

func TestAsServiceReachesOnlyTheAdminAPI(t *testing.T) {
	setup(t)
	interceptor, err := auth.NewUnary(map[string]any{
		"token_managers": map[string]any{"jwt": map[string]any{"secret": testSecret}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actx, err := AsService(context.Background(), "reminders")
	if err != nil {
		t.Fatal(err)
	}
	tkn, _ := appctx.ContextGetToken(actx)

	call := func(method string, req any) (*userpb.User, error) {
		var seen *userpb.User
		ctx := appctx.ContextSetToken(context.Background(), tkn)
		_, err := interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ any) (any, error) {
			seen, _ = appctx.ContextGetUser(ctx)
			return nil, nil
		})
		return seen, err
	}

	u, err := call("/reva.admin.v1beta1.AdminAPI/Invoke", &adminpb.InvokeRequest{})
	if err != nil {
		t.Fatalf("expected the Admin API call through, got %v", err)
	}
	if u.GetUsername() != "service:reminders" {
		t.Fatalf("unexpected caller %+v", u)
	}

	if _, err := call("/cs3.gateway.v1beta1.GatewayAPI/GetUser", &userpb.GetUserRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected a gateway call refused, got %v", err)
	}
}
