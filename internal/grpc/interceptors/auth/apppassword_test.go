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
	"errors"
	"slices"
	"testing"

	"github.com/bluele/gcache"
	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/token"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const thesis = "/eos/user/e/einstein/thesis"

var (
	homeID    = &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "1"}
	chapterID = &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "2"}
	privateID = &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "3"}
)

// fakeGateway answers the lookups of the interceptor and records the token
// each one ran with.
type fakeGateway struct {
	gateway.GatewayAPIClient
	lookups int
	tokens  []string
}

var ok = &rpc.Status{Code: rpc.Code_CODE_OK}

func (g *fakeGateway) seen(ctx context.Context) {
	g.lookups++
	md, _ := metadata.FromOutgoingContext(ctx)
	g.tokens = append(g.tokens, md.Get(appctx.TokenHeader)...)
}

func (g *fakeGateway) GetPath(ctx context.Context, req *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	g.seen(ctx)
	paths := map[string]string{
		homeID.OpaqueId:    "/eos/user/e/einstein",
		chapterID.OpaqueId: thesis + "/chapter1.tex",
		privateID.OpaqueId: "/eos/user/e/einstein/private",
	}
	p, found := paths[req.GetResourceId().GetOpaqueId()]
	if !found {
		return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	return &provider.GetPathResponse{Status: ok, Path: p}, nil
}

func (g *fakeGateway) GetShare(ctx context.Context, req *collaboration.GetShareRequest, _ ...grpc.CallOption) (*collaboration.GetShareResponse, error) {
	g.seen(ctx)
	resource := map[string]*provider.ResourceId{"in": chapterID, "out": privateID}[req.GetRef().GetId().GetOpaqueId()]
	return &collaboration.GetShareResponse{Status: ok, Share: &collaboration.Share{ResourceId: resource}}, nil
}

func (g *fakeGateway) GetReceivedShare(ctx context.Context, req *collaboration.GetReceivedShareRequest, _ ...grpc.CallOption) (*collaboration.GetReceivedShareResponse, error) {
	g.seen(ctx)
	resource := map[string]*provider.ResourceId{"in": chapterID, "out": privateID}[req.GetRef().GetId().GetOpaqueId()]
	return &collaboration.GetReceivedShareResponse{Status: ok, Share: &collaboration.ReceivedShare{Share: &collaboration.Share{ResourceId: resource}}}, nil
}

func (g *fakeGateway) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest, _ ...grpc.CallOption) (*link.GetPublicShareResponse, error) {
	g.seen(ctx)
	resource := map[string]*provider.ResourceId{"in": chapterID, "out": privateID}[req.GetRef().GetToken()]
	return &link.GetPublicShareResponse{Status: ok, Share: &link.PublicShare{ResourceId: resource}}, nil
}

func shareRef(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: id}}}
}

func appPasswordSetup(t *testing.T) (token.Manager, *userpb.User, *scope.AppPasswordScope) {
	t.Helper()
	setup(t)
	scopeExpansionCache = gcache.New(100).LFU().Build()
	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	u := &userpb.User{Id: &userpb.UserId{Idp: "https://idp.example.org", OpaqueId: "einstein"}, Username: "einstein"}
	ap := &scope.AppPasswordScope{PasswordID: "pw1", Client: scope.ClientMCP, Permissions: scope.ScriptPermissions(), Roots: []string{thesis}}
	ap.Permissions = append(ap.Permissions, scope.PermSharesManage)
	return mgr, u, ap
}

func TestAppPasswordRootsByID(t *testing.T) {
	mgr, u, ap := appPasswordSetup(t)
	cases := []struct {
		name string
		req  any
		want bool
	}{
		{"resource under the root", &provider.StatRequest{Ref: &provider.Reference{ResourceId: chapterID}}, true},
		{"resource outside", &provider.StatRequest{Ref: &provider.Reference{ResourceId: privateID}}, false},
		{"relative to a parent", &provider.ListContainerRequest{Ref: &provider.Reference{ResourceId: homeID, Path: "./thesis/figures"}}, true},
		{"relative, outside", &provider.ListContainerRequest{Ref: &provider.Reference{ResourceId: homeID, Path: "./private"}}, false},
		{"relative, escaping", &provider.ListContainerRequest{Ref: &provider.Reference{ResourceId: chapterID, Path: "../../private"}}, false},
		{"unknown resource", &provider.StatRequest{Ref: &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: "404"}}}, false},
		{"GetPath", &provider.GetPathRequest{ResourceId: chapterID}, true},
		{"move inside", &provider.MoveRequest{Source: &provider.Reference{ResourceId: chapterID}, Destination: &provider.Reference{ResourceId: homeID, Path: "./thesis/chapter2.tex"}}, true},
		{"move out", &provider.MoveRequest{Source: &provider.Reference{ResourceId: chapterID}, Destination: &provider.Reference{ResourceId: homeID, Path: "./chapter1.tex"}}, false},
		{"move in", &provider.MoveRequest{Source: &provider.Reference{ResourceId: privateID}, Destination: &provider.Reference{Path: thesis + "/stolen"}}, false},
		// The id decides, not the path the client claims.
		{"share a resource under the root", &collaboration.CreateShareRequest{ResourceInfo: &provider.ResourceInfo{Id: chapterID}}, true},
		{"share a resource outside", &collaboration.CreateShareRequest{ResourceInfo: &provider.ResourceInfo{Id: privateID, Path: thesis + "/chapter1.tex"}}, false},
		{"remove a share inside", &collaboration.RemoveShareRequest{Ref: shareRef("in")}, true},
		{"remove a share outside", &collaboration.RemoveShareRequest{Ref: shareRef("out")}, false},
		{"update a share outside by its body", &collaboration.UpdateShareRequest{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "out"}}}, false},
		{"accept a share inside", &collaboration.UpdateReceivedShareRequest{Share: &collaboration.ReceivedShare{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "in"}}}}, true},
		{"accept a share outside", &collaboration.UpdateReceivedShareRequest{Share: &collaboration.ReceivedShare{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "out"}}}}, false},
		{"remove a link outside", &link.RemovePublicShareRequest{Ref: &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: "out"}}}, false},
		{"update a link inside", &link.UpdatePublicShareRequest{Ref: &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: "in"}}}, true},
	}
	for _, c := range cases {
		g := &fakeGateway{}
		err := verifyAppPasswordScope(context.Background(), c.req, ap, u, mgr, func(context.Context) (gateway.GatewayAPIClient, error) { return g, nil })
		if got := err == nil; got != c.want {
			t.Errorf("%s: allowed = %v (%v), want %v", c.name, got, err, c.want)
		}
	}
}

// The lookups run with a token of their own: the same user and app password,
// listing only, without roots, so that they do not recurse into the check.
func TestAppPasswordLookupToken(t *testing.T) {
	mgr, u, ap := appPasswordSetup(t)
	g := &fakeGateway{}
	gw := func(context.Context) (gateway.GatewayAPIClient, error) { return g, nil }
	ctx := metadata.AppendToOutgoingContext(context.Background(), appctx.TokenHeader, "the-caller-token")

	if err := verifyAppPasswordScope(ctx, &collaboration.RemoveShareRequest{Ref: shareRef("in")}, ap, u, mgr, gw); err != nil {
		t.Fatalf("expected the request through, got %v", err)
	}
	if g.lookups != 2 || len(g.tokens) != 2 || g.tokens[0] != g.tokens[1] {
		t.Fatalf("expected two lookups with one token each, got %d lookups with %q", g.lookups, g.tokens)
	}
	owner, scopes, err := mgr.DismantleToken(context.Background(), g.tokens[0])
	if err != nil {
		t.Fatalf("dismantle lookup token: %v", err)
	}
	got, found := scope.GetAppPasswordScope(scopes)
	if owner.GetUsername() != "einstein" || len(scopes) != 1 || !found {
		t.Fatalf("lookup token for %q with scopes %v", owner.GetUsername(), scopes)
	}
	if got.PasswordID != ap.PasswordID || len(got.Roots) != 0 || !slices.Equal(got.Permissions, []string{scope.PermFilesList, scope.PermSharesList}) {
		t.Fatalf("lookup token scope %+v", got)
	}

	// Resolved once per app password and resource.
	if err := verifyAppPasswordScope(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: chapterID}}, ap, u, mgr, gw); err != nil {
		t.Fatalf("expected the request through, got %v", err)
	}
	if g.lookups != 2 {
		t.Fatalf("expected the cached path, got %d lookups", g.lookups)
	}
}

func TestAppPasswordNeedsNoLookup(t *testing.T) {
	mgr, u, ap := appPasswordSetup(t)
	unreachable := func(context.Context) (gateway.GatewayAPIClient, error) {
		t.Fatal("the gateway must not be resolved")
		return nil, nil
	}
	// Refused by the permissions before anything is resolved.
	readOnly := *ap
	readOnly.Permissions = scope.ReadOnlyPermissions()
	if err := verifyAppPasswordScope(context.Background(), &provider.DeleteRequest{Ref: &provider.Reference{ResourceId: chapterID}}, &readOnly, u, mgr, unreachable); err == nil {
		t.Fatal("a read-only app password must not delete")
	}
	unrooted := *ap
	unrooted.Roots = nil
	if err := verifyAppPasswordScope(context.Background(), &appauthpb.GenerateAppPasswordRequest{}, &unrooted, u, mgr, unreachable); err == nil {
		t.Fatal("an app password must not create app passwords")
	}
}

func TestAppPasswordLookupUnavailable(t *testing.T) {
	mgr, u, ap := appPasswordSetup(t)
	down := func(context.Context) (gateway.GatewayAPIClient, error) { return nil, errors.New("no gateway") }
	err := verifyAppPasswordScope(context.Background(), &provider.StatRequest{Ref: &provider.Reference{ResourceId: chapterID}}, ap, u, mgr, down)
	if !errors.As(err, new(unverifiedError)) {
		t.Fatalf("an unreachable gateway must leave the token unverified, got %v", err)
	}
}

// Through the interceptor: an app password token is checked by its own scope,
// and nothing else widens it.
func TestAppPasswordInterceptor(t *testing.T) {
	interceptor, _, u := setup(t)
	_ = userGroupsCache.Set(u.Id.OpaqueId, []string{"physicists"})
	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatal(err)
	}
	mint := func(ap scope.AppPasswordScope) string {
		scopes, err := scope.AddAppPasswordScope(&ap, nil)
		if err != nil {
			t.Fatal(err)
		}
		tkn, err := mgr.MintToken(context.Background(), u, scopes)
		if err != nil {
			t.Fatal(err)
		}
		return tkn
	}
	sync := mint(scope.AppPasswordScope{PasswordID: "pw1", Client: scope.ClientSync, Permissions: []string{scope.PermAll}})
	rooted := mint(scope.AppPasswordScope{PasswordID: "pw2", Client: scope.ClientMCP, Permissions: scope.ReadOnlyPermissions(), Roots: []string{thesis}})

	cases := []struct {
		name string
		tkn  string
		req  any
		want codes.Code
	}{
		{"wildcard, storage request", sync, &provider.StatRequest{Ref: &provider.Reference{ResourceId: chapterID}}, codes.OK},
		{"wildcard, app password management", sync, &appauthpb.GenerateAppPasswordRequest{}, codes.PermissionDenied},
		{"wildcard, token minting", sync, &gateway.AuthenticateRequest{}, codes.PermissionDenied},
		{"rooted, path inside", rooted, &provider.StatRequest{Ref: &provider.Reference{Path: thesis + "/chapter1.tex"}}, codes.OK},
		{"rooted, path outside", rooted, &provider.StatRequest{Ref: &provider.Reference{Path: "/eos/user/e/einstein/private"}}, codes.PermissionDenied},
		{"rooted, not granted", rooted, &provider.DeleteRequest{Ref: &provider.Reference{Path: thesis + "/chapter1.tex"}}, codes.PermissionDenied},
		// The resolution needs the gateway, which this test cannot reach.
		{"rooted, by id", rooted, &provider.StatRequest{Ref: &provider.Reference{ResourceId: chapterID}}, codes.Unavailable},
	}
	for _, c := range cases {
		ctx := appctx.ContextSetToken(context.Background(), c.tkn)
		var seen map[string]*authpb.Scope
		_, err := interceptor(ctx, c.req, &grpc.UnaryServerInfo{FullMethod: "/test"}, func(ctx context.Context, _ any) (any, error) {
			seen, _ = appctx.ContextGetScopes(ctx)
			return nil, nil
		})
		if got := status.Code(err); got != c.want {
			t.Errorf("%s: got %v (%v), want %v", c.name, got, err, c.want)
		}
		if err == nil {
			if _, found := scope.GetAppPasswordScope(seen); !found {
				t.Errorf("%s: the handler must see the app password scope", c.name)
			}
		}
	}
}
