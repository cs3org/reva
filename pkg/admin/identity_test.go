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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/interceptors/auth"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestActAs(t *testing.T) {
	setup(t)
	ctx := appctx.ContextSetToken(context.Background(), "admin-token")
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, "admin-token")

	uctx, u, err := ActAs(ctx, &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"})
	if err != nil {
		t.Fatal(err)
	}
	if u.GetUsername() != "marie" || len(u.GetGroups()) != 1 {
		t.Fatalf("unexpected user %+v", u)
	}
	if cu, _ := appctx.ContextGetUser(uctx); cu.GetUsername() != "marie" {
		t.Fatalf("unexpected context user %+v", cu)
	}
	md, _ := metadata.FromOutgoingContext(uctx)
	sent := md.Get(appctx.TokenHeader)
	if len(sent) != 1 || sent[0] == "admin-token" {
		t.Fatalf("expected the admin token replaced, got %v", sent)
	}

	interceptor, err := auth.NewUnary(map[string]any{
		"token_managers": map[string]any{"jwt": map[string]any{"secret": testSecret}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, req any) (string, error) {
		var seen string
		ctx := appctx.ContextSetToken(context.Background(), sent[0])
		_, err := interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ any) (any, error) {
			u, _ := appctx.ContextGetUser(ctx)
			seen = u.GetUsername()
			return nil, nil
		})
		return seen, err
	}
	if seen, err := call("/cs3.gateway.v1beta1.GatewayAPI/Stat", &provider.StatRequest{}); err != nil || seen != "marie" {
		t.Fatalf("expected a CS3 call as marie, got %q, %v", seen, err)
	}
	if _, err := call("/reva.admin.v1beta1.AdminAPI/Invoke", &adminpb.InvokeRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected an Admin API call refused, got %v", err)
	}
}

func TestActAsUnknownUser(t *testing.T) {
	setup(t)
	if _, _, err := ActAs(context.Background(), &userpb.UserId{OpaqueId: "nobody"}); err == nil {
		t.Fatal("expected an error")
	} else if _, ok := err.(errtypes.IsNotFound); !ok {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestActAsFirstSkipsUnknownUsers(t *testing.T) {
	setup(t)
	calls := 0
	u, err := ActAsFirst(context.Background(), []*userpb.UserId{{OpaqueId: "nobody"}, {OpaqueId: "marie"}}, func(context.Context) (*rpc.Status, error) {
		calls++
		return &rpc.Status{Code: rpc.Code_CODE_OK}, nil
	})
	if err != nil || calls != 1 || u.GetId().GetIdp() != "cernbox" {
		t.Fatalf("got %v, %v after %d calls", u, err, calls)
	}
	if u, err := ActAsFirst(context.Background(), []*userpb.UserId{{OpaqueId: "nobody"}}, nil); u != nil || err == nil {
		t.Fatalf("expected no user and an error, got %v, %v", u, err)
	}
}
