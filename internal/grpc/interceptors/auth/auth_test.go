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

package auth

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testSecret = "auth-interceptor-test-secret"

// noGateway is a resolver that cannot find the gateway, as a process does while
// every gateway node is passed over.
type noGateway struct {
	service.Clients
	asked atomic.Int32
}

func (r *noGateway) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.asked.Add(1)
	return nil, errors.New(`service registry: no selectable grpc node for "gateway"`)
}

var resolver = &noGateway{}

// setup returns an interceptor that fetches the groups from the gateway, and a
// valid token for einstein.
func setup(t *testing.T) (grpc.UnaryServerInterceptor, string, *userpb.User) {
	t.Helper()
	sharedconf.Init(&revaconfig.Shared{JWTSecret: testSecret, SkipUserGroupsInToken: true})
	if !sharedconf.SkipUserGroupsInToken() {
		t.Skip("shared config already initialised without skip_user_groups_in_token")
	}
	service.SetGlobal(resolver)

	interceptor, err := NewUnary(map[string]any{
		"token_managers": map[string]any{"jwt": map[string]any{"secret": testSecret}},
	}, nil)
	if err != nil {
		t.Fatalf("build interceptor: %v", err)
	}
	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	u := &userpb.User{
		Id:       &userpb.UserId{Idp: "https://idp.example.org", OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_PRIMARY},
		Username: "einstein",
	}
	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("owner scope: %v", err)
	}
	tkn, err := mgr.MintToken(context.Background(), u, scopes)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return interceptor, tkn, u
}

func call(interceptor grpc.UnaryServerInterceptor, tkn string) (*userpb.User, error) {
	var seen *userpb.User
	ctx := appctx.ContextSetToken(context.Background(), tkn)
	_, err := interceptor(ctx, &registry.GetStorageProvidersRequest{}, &grpc.UnaryServerInfo{
		FullMethod: "/cs3.storage.registry.v1beta1.RegistryAPI/GetStorageProviders",
	}, func(ctx context.Context, _ any) (any, error) {
		seen, _ = appctx.ContextGetUser(ctx)
		return nil, nil
	})
	return seen, err
}

// A valid token that cannot be checked is a retryable failure, not a reason to
// send the user back to log in.
func TestAnUnreachableGatewayIsNotAnInvalidToken(t *testing.T) {
	interceptor, tkn, _ := setup(t)

	_, err := call(interceptor, tkn)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", err)
	}
}

func TestCachedGroupsNeedNoGateway(t *testing.T) {
	interceptor, tkn, u := setup(t)
	_ = userGroupsCache.Set(u.Id.OpaqueId, []string{"physicists"})
	asked := resolver.asked.Load()

	seen, err := call(interceptor, tkn)
	if err != nil {
		t.Fatalf("expected the call through, got %v", err)
	}
	if resolver.asked.Load() != asked {
		t.Fatal("cached groups must not resolve the gateway")
	}
	if !slices.Equal(seen.GetGroups(), []string{"physicists"}) {
		t.Fatalf("expected the cached groups, got %v", seen.GetGroups())
	}
}

func TestATamperedTokenIsStillInvalid(t *testing.T) {
	interceptor, tkn, _ := setup(t)

	_, err := call(interceptor, tkn+"x")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
}
