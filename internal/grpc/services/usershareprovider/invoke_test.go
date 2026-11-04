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

package usershareprovider

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func user(name string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: name, Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: name}
}

var (
	// richard initiated a share on a home of einstein long ago and has no
	// right on it anymore; cbox owns the project, which admin1 and admin2
	// administer.
	users = map[string]*userpb.User{}

	files = []*provider.ResourceInfo{
		{Id: &provider.ResourceId{StorageId: "home", OpaqueId: "100", SpaceId: "home-einstein"}, Path: "/home/einstein/doc", Owner: user("einstein").Id},
		{Id: &provider.ResourceId{StorageId: "home", OpaqueId: "101", SpaceId: "home-einstein"}, Path: "/home/einstein/other", Owner: user("einstein").Id},
		{Id: &provider.ResourceId{StorageId: "project", OpaqueId: "200", SpaceId: "project-x"}, Path: "/project/data", Owner: user("cbox").Id},
		{Id: &provider.ResourceId{StorageId: "home2", OpaqueId: "300", SpaceId: "home-feynman"}, Path: "/home/feynman/doc", Owner: user("feynman").Id},
	}
	granters = map[string][]string{"home": {"einstein"}, "project": {"cbox", "admin1", "admin2"}, "home2": {"feynman"}}
)

func init() {
	for _, n := range []string{"operator", "einstein", "marie", "richard", "cbox", "admin1", "admin2", "feynman", "bob"} {
		users[n] = user(n)
	}
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "usershareprovider-test-secret"})
	revaservice.SetGlobal(resolver{})
}

type resolver struct{ revaservice.Clients }

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return gw, nil }

var gw = &fakeGateway{}

// fakeGateway answers the gateway calls of the invocations. UpdateShare and
// RemoveShare check the caller as the real gateway does: the provider must
// return the share, and the caller must be allowed to change the grants.
type fakeGateway struct {
	gateway.GatewayAPIClient
	svc     *service
	mu      sync.Mutex
	actedAs []string
}

// GetUser matches the opaque id, and the idp if one is given, as the user
// providers do: the share tables keep no idp.
func (*fakeGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	id := req.GetUserId()
	if u, ok := users[id.GetOpaqueId()]; ok && (id.GetIdp() == "" || id.GetIdp() == u.GetId().GetIdp()) {
		return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

// GetUserByClaim serves the sql manager, which looks the type of a grantee
// up by username when it creates a share; the invocations never call it.
func (*fakeGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if u, ok := users[req.GetValue()]; ok && req.GetClaim() == "username" {
		return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func fileOf(ref *provider.Reference) *provider.ResourceInfo {
	for _, f := range files {
		if f.Path == ref.GetPath() || f.Id.StorageId == ref.GetResourceId().GetStorageId() && f.Id.OpaqueId == ref.GetResourceId().GetOpaqueId() {
			return f
		}
	}
	return nil
}

func canGrant(ctx context.Context, storageID string) bool {
	u, _ := appctx.ContextGetUser(ctx)
	return slices.Contains(granters[storageID], u.GetUsername())
}

func (*fakeGateway) Stat(ctx context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	f := fileOf(req.GetRef())
	if f == nil {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	info := proto.Clone(f).(*provider.ResourceInfo)
	info.PermissionSet = &provider.ResourcePermissions{Stat: true, AddGrant: canGrant(ctx, f.Id.StorageId)}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: info}, nil
}

func (*fakeGateway) GetPath(_ context.Context, req *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	if f := fileOf(&provider.Reference{ResourceId: req.GetResourceId()}); f != nil {
		return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Path: f.Path}, nil
	}
	return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *fakeGateway) check(ctx context.Context, ref *collaboration.ShareReference) *rpc.Status {
	res, _ := g.svc.GetShare(ctx, &collaboration.GetShareRequest{Ref: ref})
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return res.GetStatus()
	}
	if !canGrant(ctx, res.GetShare().GetResourceId().GetStorageId()) {
		return &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}
	}
	u, _ := appctx.ContextGetUser(ctx)
	g.mu.Lock()
	g.actedAs = append(g.actedAs, u.GetUsername())
	g.mu.Unlock()
	return nil
}

func (g *fakeGateway) UpdateShare(ctx context.Context, req *collaboration.UpdateShareRequest, _ ...grpc.CallOption) (*collaboration.UpdateShareResponse, error) {
	if st := g.check(ctx, req.GetRef()); st != nil {
		return &collaboration.UpdateShareResponse{Status: st}, nil
	}
	return g.svc.UpdateShare(ctx, req)
}

func (g *fakeGateway) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest, _ ...grpc.CallOption) (*collaboration.RemoveShareResponse, error) {
	if st := g.check(ctx, req.GetRef()); st != nil {
		return &collaboration.RemoveShareResponse{Status: st}, nil
	}
	return g.svc.RemoveShare(ctx, req)
}

type fakeProvider struct {
	provider.ProviderAPIClient
	added, removed []*provider.Grant
}

func (p *fakeProvider) AddGrant(_ context.Context, req *provider.AddGrantRequest, _ ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	p.added = append(p.added, req.Grant)
	return &provider.AddGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (p *fakeProvider) RemoveGrant(_ context.Context, req *provider.RemoveGrantRequest, _ ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	p.removed = append(p.removed, req.Grant)
	return &provider.RemoveGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func newService(t *testing.T) (*service, *fakeProvider) {
	t.Helper()
	sm, err := sqlmgr.NewShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "shares.db")})
	if err != nil {
		t.Fatal(err)
	}
	prov := &fakeProvider{}
	s := &service{Set: invoke.NewSet(), conf: &config{Driver: "sql"}, sm: sm}
	s.grantsAt = func(context.Context, string) (provider.ProviderAPIClient, error) { return prov, nil }
	s.registerInvocations()
	gw.svc, gw.actedAs = s, nil
	return s, prov
}

func as(name string) context.Context {
	return appctx.ContextSetUser(context.Background(), users[name])
}

func createShare(t *testing.T, s *service, path, owner, initiator, sharee string, exp *typespb.Timestamp) string {
	t.Helper()
	info := fileOf(&provider.Reference{Path: path})
	sh, err := s.sm.Share(as(owner), info, &collaboration.ShareGrant{
		Grantee:     userGrantee(users[sharee].Id),
		Permissions: &collaboration.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()},
		Expiration:  exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if initiator != owner {
		if err := s.sm.(adminManager).TransferShare(as(owner), shareRef(sh.Id.OpaqueId), initiator); err != nil {
			t.Fatal(err)
		}
	}
	return sh.Id.OpaqueId
}

func mustInvoke(t *testing.T, s *service, name string, args map[string]any) invoke.Result {
	t.Helper()
	res, err := s.Invoke(as("operator"), name, args)
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return res
}

func list(t *testing.T, s *service, args map[string]any) []invoke.Result {
	t.Helper()
	var out []invoke.Result
	if err := s.InvokeStream(as("operator"), "list", args, func(r invoke.Result) error {
		out = append(out, r)
		return nil
	}); err != nil {
		t.Fatalf("list %v: %v", args, err)
	}
	return out
}

func cs3(r invoke.Result) map[string]any { return r["share"].(map[string]any) }

func idOf(r invoke.Result) string { return cs3(r)["id"].(map[string]any)["opaqueId"].(string) }

func ids(rs []invoke.Result) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, idOf(r))
	}
	return out
}

func TestList(t *testing.T) {
	s, _ := newService(t)
	doc := createShare(t, s, "/home/einstein/doc", "einstein", "einstein", "marie", nil)
	other := createShare(t, s, "/home/einstein/other", "einstein", "einstein", "bob", &typespb.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())})
	project := createShare(t, s, "/project/data", "cbox", "admin1", "marie", nil)

	for _, tc := range []struct {
		args map[string]any
		want []string
	}{
		{map[string]any{"owner": "einstein"}, []string{doc, other}},
		{map[string]any{"owner": "einstein", "include_expired": "false"}, []string{doc}},
		{map[string]any{"initiator": "admin1"}, []string{project}},
		{map[string]any{"sharee": "marie"}, []string{doc, project}},
		{map[string]any{"sharee": "marie", "owner": "cbox"}, []string{project}},
		{map[string]any{"space": "home-einstein"}, []string{doc, other}},
		{map[string]any{"resource_id": "home!100"}, []string{doc}},
		{map[string]any{"resource_id": "home$home-einstein!101", "owner": "einstein"}, []string{other}},
		{map[string]any{"id": project}, []string{project}},
		{map[string]any{"id": "999"}, []string{}},
		{map[string]any{"path": "/home/einstein/doc", "owner": "einstein"}, []string{doc}},
		{map[string]any{"path": "/project/data", "as": "admin2"}, []string{project}},
		// The admin facade sends the full UserId and Grantee.
		{map[string]any{"owner": admin.UserArg(user("einstein").Id)}, []string{doc, other}},
		{map[string]any{"sharee": admin.GranteeArg(userGrantee(user("marie").Id)), "initiator": admin.UserArg(user("admin1").Id)}, []string{project}},
		{map[string]any{"path": "/project/data", "as": admin.UserArg(user("admin2").Id)}, []string{project}},
	} {
		if got := ids(list(t, s, tc.args)); !slices.Equal(got, tc.want) {
			t.Errorf("list %v: got %v, want %v", tc.args, got, tc.want)
		}
	}

	for _, args := range []map[string]any{{}, {"include_orphans": "true"}, {"path": "/home/einstein/doc"}, {"resource_id": "100"}} {
		if err := s.InvokeStream(as("operator"), "list", args, func(invoke.Result) error { return nil }); err == nil {
			t.Errorf("list %v: expected an error", args)
		}
	}

	got := list(t, s, map[string]any{"initiator": "admin1", "resolve_paths": "true"})
	if len(got) != 1 || got[0]["path"] != "/project/data" {
		t.Fatalf("resolve_paths: got %v", got)
	}
	if c := cs3(got[0]); c["owner"].(map[string]any)["opaqueId"] != "cbox" || c["creator"].(map[string]any)["opaqueId"] != "admin1" {
		t.Fatalf("unexpected share %v", c)
	}

	mustInvoke(t, s, "mark_orphan", map[string]any{"id": doc})
	if got := ids(list(t, s, map[string]any{"owner": "einstein"})); !slices.Equal(got, []string{other}) {
		t.Fatalf("orphan listed by default: %v", got)
	}
	got = list(t, s, map[string]any{"owner": "einstein", "include_orphans": "true"})
	if !slices.Equal(ids(got), []string{doc, other}) || got[0]["orphan"] != true || got[0]["extra"].(map[string]string)["orphaned_at"] == "" {
		t.Fatalf("include_orphans: got %v", got)
	}
}

func TestGet(t *testing.T) {
	s, _ := newService(t)
	id := createShare(t, s, "/home/einstein/doc", "einstein", "einstein", "marie", nil)

	res := mustInvoke(t, s, "get", map[string]any{"id": id})
	if idOf(res) != id || res["path"] != "/home/einstein/doc" || res["orphan"] != nil || res["extra"].(map[string]string)["initial_path"] != "/home/einstein/doc" {
		t.Fatalf("unexpected result %v", res)
	}
	if _, err := s.Invoke(as("operator"), "get", map[string]any{"id": "999"}); !isNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}

	key := func(owner, storage, opaque, sharee string) string {
		b, err := protojson.Marshal(&collaboration.ShareKey{
			Owner:      &userpb.UserId{OpaqueId: owner},
			ResourceId: &provider.ResourceId{StorageId: storage, OpaqueId: opaque},
			Grantee:    userGrantee(&userpb.UserId{OpaqueId: sharee}),
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if res := mustInvoke(t, s, "get", map[string]any{"key": key("einstein", "home", "100", "marie")}); idOf(res) != id {
		t.Fatalf("get by key: unexpected result %v", res)
	}
	if _, err := s.Invoke(as("operator"), "get", map[string]any{"key": key("einstein", "home", "100", "bob")}); !isNotFound(err) {
		t.Fatalf("get by the key of no share: expected not found, got %v", err)
	}
	for _, args := range []map[string]any{{}, {"id": id, "key": key("einstein", "home", "100", "marie")}, {"key": `{"owner":{"opaqueId":"einstein"}}`}, {"key": "{"}} {
		if _, err := s.Invoke(as("operator"), "get", args); err == nil || isNotFound(err) {
			t.Errorf("get %v: got %v, want a bad request", args, err)
		}
	}
}

// A share that the caller may not see looks like a share that does not exist.
func TestGetShareNotFoundStatus(t *testing.T) {
	s, _ := newService(t)
	id := createShare(t, s, "/home/einstein/doc", "einstein", "richard", "marie", nil)

	for _, u := range []string{"einstein", "richard"} {
		res, err := s.GetShare(as(u), &collaboration.GetShareRequest{Ref: shareRef(id)})
		if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			t.Fatalf("%s: got %v, %v", u, res.GetStatus(), err)
		}
	}
	for _, ref := range []string{id, "999"} {
		res, err := s.GetShare(as("bob"), &collaboration.GetShareRequest{Ref: shareRef(ref)})
		if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_NOT_FOUND {
			t.Fatalf("share %q: got %v, %v", ref, res.GetStatus(), err)
		}
	}
}

func TestUpdate(t *testing.T) {
	s, _ := newService(t)
	// richard cannot change the share anymore, so the owner does it.
	id := createShare(t, s, "/home/einstein/doc", "einstein", "richard", "marie", nil)

	res := mustInvoke(t, s, "update", map[string]any{"id": id, "role": "editor", "expires_at": "2099-01-01", "fallback": "true"})
	if !slices.Equal(gw.actedAs, []string{"einstein", "einstein"}) {
		t.Fatalf("acted as %v, want einstein for both fields", gw.actedAs)
	}
	sh, err := s.sm.(adminManager).GetShareUnfiltered(context.Background(), shareRef(id))
	if err != nil {
		t.Fatal(err)
	}
	if sh.Permissions != uint8(permissions.NewEditorRole().OCSPermissions()) || !sh.Expiration.Valid || sh.Expiration.V.Year() != 2099 {
		t.Fatalf("share not updated: %+v", sh.ProtoShare)
	}
	if cs3(res)["expiration"] == nil {
		t.Fatalf("result without the expiration: %v", res)
	}

	mustInvoke(t, s, "update", map[string]any{"id": id, "expires_at": "none"})
	if sh, _ = s.sm.(adminManager).GetShareUnfiltered(context.Background(), shareRef(id)); sh.Expiration.Valid {
		t.Fatal("expiration not cleared")
	}

	for _, args := range []map[string]any{
		{"id": id},
		{"id": id, "role": "boss"},
		{"id": id, "expires_at": "soon"},
	} {
		if _, err := s.Invoke(as("operator"), "update", args); err == nil {
			t.Errorf("update %v: expected an error", args)
		}
	}
	if _, err := s.Invoke(as("operator"), "update", map[string]any{"id": "999", "role": "viewer"}); !isNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestUpdateRefusedForEveryone(t *testing.T) {
	s, _ := newService(t)
	// bob owns nothing here: the gateway refuses both users.
	id := createShare(t, s, "/home/einstein/doc", "bob", "richard", "marie", nil)
	if _, err := s.Invoke(as("operator"), "update", map[string]any{"id": id, "role": "editor"}); err == nil {
		t.Fatal("expected an error")
	}
	if len(gw.actedAs) != 0 {
		t.Fatalf("acted as %v", gw.actedAs)
	}
}

func TestRemove(t *testing.T) {
	s, _ := newService(t)
	viaGateway := createShare(t, s, "/project/data", "cbox", "admin1", "marie", nil)
	dbOnly := createShare(t, s, "/home/einstein/doc", "einstein", "einstein", "marie", nil)
	orphan := createShare(t, s, "/home/einstein/other", "einstein", "einstein", "marie", nil)
	mustInvoke(t, s, "mark_orphan", map[string]any{"id": orphan})

	res := mustInvoke(t, s, "remove", map[string]any{"id": viaGateway})
	if idOf(res) != viaGateway || !slices.Equal(gw.actedAs, []string{"admin1"}) {
		t.Fatalf("remove: got %v, acted as %v", res, gw.actedAs)
	}
	mustInvoke(t, s, "remove", map[string]any{"id": dbOnly, "db_only": "true"})
	mustInvoke(t, s, "remove", map[string]any{"id": orphan})
	if len(gw.actedAs) != 1 {
		t.Fatalf("db_only and orphan removals went through the gateway: %v", gw.actedAs)
	}
	for _, id := range []string{viaGateway, dbOnly, orphan} {
		if _, err := s.Invoke(as("operator"), "get", map[string]any{"id": id}); !isNotFound(err) {
			t.Errorf("share %s: expected not found, got %v", id, err)
		}
	}
}

func TestSyncExpiry(t *testing.T) {
	s, prov := newService(t)
	expired := createShare(t, s, "/home/einstein/doc", "einstein", "einstein", "marie", &typespb.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())})
	live := createShare(t, s, "/home/einstein/other", "einstein", "einstein", "marie", nil)

	if _, err := s.Invoke(as("operator"), "sync_expiry", map[string]any{"id": live}); err == nil {
		t.Fatal("expected an error for a share that is not expired")
	}
	res := mustInvoke(t, s, "sync_expiry", map[string]any{"id": expired})
	if len(prov.removed) != 1 || prov.removed[0].GetGrantee().GetUserId().GetOpaqueId() != "marie" {
		t.Fatalf("removed grants %v", prov.removed)
	}
	if res["extra"].(map[string]string)["expired_at"] == "" {
		t.Fatalf("share not marked expired: %v", res)
	}
}

func TestTransfer(t *testing.T) {
	s, _ := newService(t)
	id := createShare(t, s, "/project/data", "cbox", "admin1", "marie", nil)

	if _, err := s.Invoke(as("operator"), "transfer", map[string]any{"id": id, "initiator": "bob"}); err == nil {
		t.Fatal("expected an error for a user who cannot share the resource")
	}
	other := admin.UserArg(&userpb.UserId{Idp: "elsewhere", OpaqueId: "admin2"})
	if _, err := s.Invoke(as("operator"), "transfer", map[string]any{"id": id, "initiator": other}); !errors.As(err, new(errtypes.IsNotFound)) {
		t.Fatalf("expected the user of another idp not found, got %v", err)
	}
	res := mustInvoke(t, s, "transfer", map[string]any{"id": id, "initiator": admin.UserArg(user("admin2").Id)})
	if c := cs3(res); c["creator"].(map[string]any)["opaqueId"] != "admin2" || c["owner"].(map[string]any)["opaqueId"] != "cbox" {
		t.Fatalf("unexpected share %v", c)
	}
}

func TestMove(t *testing.T) {
	s, prov := newService(t)
	id := createShare(t, s, "/home/einstein/doc", "einstein", "einstein", "marie", nil)

	res := mustInvoke(t, s, "move", map[string]any{"id": id, "path": "/home/feynman/doc", "as": "feynman", "apply_grant": "true"})
	rid := cs3(res)["resourceId"].(map[string]any)
	if res["path"] != "/home/feynman/doc" || cs3(res)["owner"].(map[string]any)["opaqueId"] != "feynman" ||
		rid["storageId"] != "home2" || rid["opaqueId"] != "300" || rid["spaceId"] != "home-feynman" {
		t.Fatalf("unexpected result %v", res)
	}
	if len(prov.added) != 1 || prov.added[0].GetGrantee().GetUserId().GetType() != userpb.UserType_USER_TYPE_PRIMARY {
		t.Fatalf("added grants %v", prov.added)
	}
	if _, err := s.Invoke(as("operator"), "move", map[string]any{"id": id, "path": "/nowhere"}); err == nil {
		t.Fatal("expected an error for a path that does not exist")
	}
}

type stubManager struct{ share.Manager }

func TestUnsupportedDriver(t *testing.T) {
	s := &service{Set: invoke.NewSet(), conf: &config{Driver: "json"}, sm: stubManager{}}
	s.registerInvocations()
	if len(s.Invocations()) == 0 {
		t.Fatal("expected the invocations listed")
	}
	_, err := s.Invoke(as("operator"), "get", map[string]any{"id": "1"})
	if _, ok := err.(errtypes.IsNotSupported); !ok {
		t.Fatalf("get: expected not supported, got %v", err)
	}
	err = s.InvokeStream(as("operator"), "list", map[string]any{"owner": "einstein"}, func(invoke.Result) error { return nil })
	if _, ok := err.(errtypes.IsNotSupported); !ok {
		t.Fatalf("list: expected not supported, got %v", err)
	}
}
