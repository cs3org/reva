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
	"slices"
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	_ "github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	_ "github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeServerStream) Context() context.Context { return f.ctx }

// adminTestTokens returns the unary and stream interceptors, and a token for
// each kind of scope.
func adminTestTokens(t *testing.T) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor, map[string]string) {
	t.Helper()
	unary, _, u := setup(t)
	stream, err := NewStream(map[string]any{
		"token_managers": map[string]any{"jwt": map[string]any{"secret": testSecret}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// NewStream makes a new cache, so seed it after.
	_ = userGroupsCache.Set(u.Id.OpaqueId, []string{"physicists"})

	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatal(err)
	}
	type adder func(map[string]*authpb.Scope) (map[string]*authpb.Scope, error)
	lightweight := func(s map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
		return scope.AddLightweightAccountScope(authpb.Role_ROLE_OWNER, s)
	}
	tokens := map[string]string{}
	for name, adds := range map[string][]adder{
		"admin":          {scope.AddAdminScope},
		"user":           {scope.AddOwnerScope},
		"lightweight":    {lightweight},
		"machine":        {scope.AddMachineScope},
		"machine + user": {scope.AddOwnerScope, scope.AddMachineScope},
	} {
		var scopes map[string]*authpb.Scope
		for _, add := range adds {
			if scopes, err = add(scopes); err != nil {
				t.Fatal(err)
			}
		}
		if tokens[name], err = mgr.MintToken(context.Background(), u, scopes); err != nil {
			t.Fatal(err)
		}
	}
	return unary, stream, tokens
}

// callMethod runs a call to the method descriptor through the interceptor that
// matches its kind, with a request message of its input type.
func callMethod(unary grpc.UnaryServerInterceptor, stream grpc.StreamServerInterceptor, md protoreflect.MethodDescriptor, tkn string) error {
	ctx := appctx.ContextSetToken(context.Background(), tkn)
	method := "/" + string(md.Parent().FullName()) + "/" + string(md.Name())
	if md.IsStreamingServer() || md.IsStreamingClient() {
		return stream(nil, fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method},
			func(any, grpc.ServerStream) error { return nil })
	}
	mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
	if err != nil {
		return err
	}
	_, err = unary(ctx, mt.New().Interface(), &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) { return nil, nil })
	return err
}

func rangeMethods(match func(pkg string) bool, f func(protoreflect.MethodDescriptor)) {
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if !match(string(fd.Package())) {
			return true
		}
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			for j := range sd.Methods().Len() {
				f(sd.Methods().Get(j))
			}
		}
		return true
	})
}

// Every admin and control method, unary and streaming, through the
// interceptors: only the admin token passes, except for the step-up methods.
func TestOnlyAnAdminTokenCallsAdminMethods(t *testing.T) {
	unary, stream, tokens := adminTestTokens(t)
	stepUp := []string{"RequestAdmin", "CheckAdmin"}

	var n int
	rangeMethods(func(pkg string) bool { return pkg == "reva.admin.v1beta1" || pkg == "reva.control.v1beta1" }, func(md protoreflect.MethodDescriptor) {
		n++
		up := md.Parent().Name() == "AdminAPI" && slices.Contains(stepUp, string(md.Name()))
		for name, tkn := range tokens {
			want := codes.PermissionDenied
			switch {
			case !up && name == "admin", up && name != "admin" && name != "lightweight":
				want = codes.OK
			}
			if got := status.Code(callMethod(unary, stream, md, tkn)); got != want {
				t.Errorf("%s token, %s: got %v, want %v", name, md.FullName(), got, want)
			}
		}
	})
	if n == 0 {
		t.Fatal("no admin method in the proto registry")
	}
}

// A typed admin RPC that takes a CS3 request is still an admin method: the
// lightweight scope accepts a ListSharesRequest, and must not reach it.
func TestAnAdminMethodWithACS3RequestNeedsTheAdminScope(t *testing.T) {
	unary, _, tokens := adminTestTokens(t)
	for name, tkn := range tokens {
		want := codes.PermissionDenied
		if name == "admin" {
			want = codes.OK
		}
		ctx := appctx.ContextSetToken(context.Background(), tkn)
		_, err := unary(ctx, &collaboration.ListSharesRequest{}, &grpc.UnaryServerInfo{FullMethod: "/reva.admin.v1beta1.SharesAdmin/ListShares"},
			func(context.Context, any) (any, error) { return nil, nil })
		if got := status.Code(err); got != want {
			t.Errorf("%s token: got %v, want %v", name, got, want)
		}
	}
}

// Every CS3 method through the interceptors, with an admin token.
func TestAnAdminTokenCallsNoCS3Method(t *testing.T) {
	unary, stream, tokens := adminTestTokens(t)
	var n int
	rangeMethods(func(pkg string) bool { return strings.HasPrefix(pkg, "cs3.") }, func(md protoreflect.MethodDescriptor) {
		n++
		if err := callMethod(unary, stream, md, tokens["admin"]); status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s: got %v, want PermissionDenied", md.FullName(), err)
		}
	})
	if n == 0 {
		t.Fatal("no CS3 method in the proto registry")
	}
}
