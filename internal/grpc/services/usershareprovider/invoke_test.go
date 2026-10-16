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
	"path/filepath"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/share"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"google.golang.org/grpc"
)

var (
	operator   = &userpb.User{Id: &userpb.UserId{OpaqueId: "operator"}, Username: "operator"}
	einstein   = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "einstein"}
	marie      = &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "cernbox", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "marie"}
	cernboxsvc = &userpb.User{Id: &userpb.UserId{OpaqueId: "cernboxsvc", Idp: "cernbox", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "cernboxsvc"}
	admin1     = &userpb.User{Id: &userpb.UserId{OpaqueId: "admin1", Idp: "cernbox", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "admin1", Groups: []string{"cernbox-project-cernbox-admins"}}
	admin2     = &userpb.User{Id: &userpb.UserId{OpaqueId: "admin2", Idp: "cernbox", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "admin2", Groups: []string{"cernbox-project-cernbox-admins"}}
	users      = []*userpb.User{operator, einstein, marie, cernboxsvc, admin1, admin2}

	files = map[string]*provider.ResourceInfo{
		"/eos/user/e/einstein/doc":    {Id: &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "100", SpaceId: "home-e"}, Path: "/eos/user/e/einstein/doc", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: einstein.Id},
		"/eos/user/e/einstein/other":  {Id: &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "101", SpaceId: "home-e"}, Path: "/eos/user/e/einstein/other", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: einstein.Id},
		"/eos/project/c/cernbox/data": {Id: &provider.ResourceId{StorageId: "eosproject-c", OpaqueId: "200", SpaceId: "project-cernbox"}, Path: "/eos/project/c/cernbox/data", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: cernboxsvc.Id},
	}
)

// fakeGateway answers the few gateway calls the invocations make, writing
// through the service's own manager where the real gateway would call the
// provider back.
type fakeGateway struct {
	gateway.GatewayAPIClient
	svc           *service
	grantsRemoved []string
}

func (fakeGateway) GetUserByClaim(_ context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	for _, u := range users {
		if req.GetClaim() == "username" && req.GetValue() == u.Username {
			return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
		}
	}
	return &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "not found"}}, nil
}

func (fakeGateway) Stat(_ context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	if info, ok := files[req.GetRef().GetPath()]; ok {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: info}, nil
	}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "not found"}}, nil
}

func (fakeGateway) GetPath(_ context.Context, req *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	for p, info := range files {
		if info.Id.StorageId == req.GetResourceId().GetStorageId() && info.Id.OpaqueId == req.GetResourceId().GetOpaqueId() {
			return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Path: p}, nil
		}
	}
	return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *fakeGateway) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest, _ ...grpc.CallOption) (*collaboration.CreateShareResponse, error) {
	// the real gateway applies the grant, then records the share through the provider
	sh, err := g.svc.sm.Share(ctx, req.ResourceInfo, req.Grant)
	if err != nil {
		return &collaboration.CreateShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL, Message: err.Error()}}, nil
	}
	return &collaboration.CreateShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: sh}, nil
}

func (g *fakeGateway) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest, _ ...grpc.CallOption) (*collaboration.RemoveShareResponse, error) {
	// the real gateway checks the caller, removes the grant and deletes the record
	if u, ok := appctx.ContextGetUser(ctx); !ok || u.Username == "operator" {
		return &collaboration.RemoveShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "not the initiator"}}, nil
	}
	g.grantsRemoved = append(g.grantsRemoved, req.GetRef().GetId().GetOpaqueId())
	if err := g.svc.sm.Unshare(ctx, req.Ref); err != nil {
		return &collaboration.RemoveShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL, Message: err.Error()}}, nil
	}
	return &collaboration.RemoveShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

type fakeProvider struct {
	provider.ProviderAPIClient
	removed []*provider.Grant
	added   []*provider.Grant
}

func (p *fakeProvider) AddGrant(_ context.Context, req *provider.AddGrantRequest, _ ...grpc.CallOption) (*provider.AddGrantResponse, error) {
	p.added = append(p.added, req.Grant)
	return &provider.AddGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

func (p *fakeProvider) RemoveGrant(_ context.Context, req *provider.RemoveGrantRequest, _ ...grpc.CallOption) (*provider.RemoveGrantResponse, error) {
	p.removed = append(p.removed, req.Grant)
	return &provider.RemoveGrantResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
}

type resolver struct{ revaservice.Clients }

var gw = &fakeGateway{}

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return gw, nil }

func init() {
	revaservice.SetGlobal(resolver{Clients: revaservice.Global()})
}

func newService(t *testing.T) (*service, *fakeProvider) {
	t.Helper()
	sm, err := sqlmgr.NewShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "shares.db")})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := admin.NewIdentity("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	prov := &fakeProvider{}
	s := &service{conf: &config{Driver: "sql"}, sm: sm, identity: identity}
	s.grantsAt = func(context.Context, string) (provider.ProviderAPIClient, error) { return prov, nil }
	s.registerInvocations()
	gw.svc, gw.grantsRemoved = s, nil
	return s, prov
}

func adminContext() context.Context {
	return appctx.ContextSetUser(context.Background(), operator)
}

func mustInvoke(t *testing.T, s *service, name string, args map[string]any) invoke.Result {
	t.Helper()
	res, err := s.Invoke(adminContext(), name, args)
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return res
}

func shares(t *testing.T, res invoke.Result) []invoke.Result {
	t.Helper()
	out, ok := res["shares"].([]invoke.Result)
	if !ok {
		t.Fatalf("no shares in %v", res)
	}
	return out
}

func TestCreateListRemove(t *testing.T) {
	s, _ := newService(t)
	res := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "editor"})
	if res["created"] != true || res["owner"] != "einstein" || res["initiator"] != "einstein" || res["permissions"] != 15 || res["role"] != "editor" {
		t.Fatalf("create: %v", res)
	}
	if g := res["grantee"].(map[string]any); g["type"] != "user" || g["id"] != "marie" {
		t.Fatalf("grantee: %v", g)
	}
	id := res["id"].(string)

	for _, args := range []map[string]any{
		{"owner": "einstein"}, {"initiator": "einstein"}, {"sharee": "marie"}, {"space": "home-e"},
		{"resource": "eoshome-e:100"}, {"path": "/eos/user/e/einstein/doc"}, {"id": id},
	} {
		if got := shares(t, mustInvoke(t, s, "list", args)); len(got) != 1 || got[0]["id"] != id {
			t.Fatalf("list %v: %v", args, got)
		}
	}
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "marie"})); len(got) != 0 {
		t.Fatalf("marie owns nothing: %v", got)
	}
	if _, err := s.Invoke(adminContext(), "list", map[string]any{"include_orphans": "true"}); err == nil {
		t.Fatal("list without a selector must be refused")
	}
	got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "resolve_paths": "true"}))
	if got[0]["path"] != "/eos/user/e/einstein/doc" {
		t.Fatalf("resolve_paths: %v", got[0])
	}
	if got := mustInvoke(t, s, "get", map[string]any{"id": id, "resolve_path": "true"}); got["path"] != "/eos/user/e/einstein/doc" {
		t.Fatalf("get: %v", got)
	}

	// creating it again is reported, or ignored on request
	if _, err := s.Invoke(adminContext(), "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "1"}); err == nil {
		t.Fatal("a duplicate share must be refused")
	}
	again := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "1", "ignore_existing": "true"})
	if again["created"] != false || again["id"] != id {
		t.Fatalf("ignore_existing: %v", again)
	}

	// removal goes through the gateway as the initiator, which removes the grant
	rm := mustInvoke(t, s, "remove", map[string]any{"id": id})
	if rm["grant_removed"] != true || len(gw.grantsRemoved) != 1 || gw.grantsRemoved[0] != id {
		t.Fatalf("remove: %v, grants removed %v", rm, gw.grantsRemoved)
	}
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 0 {
		t.Fatalf("removed share still listed: %v", got)
	}
	if _, err := s.Invoke(adminContext(), "get", map[string]any{"id": id}); err == nil {
		t.Fatal("a removed share must not be found")
	}
}

func TestCreateValidation(t *testing.T) {
	s, _ := newService(t)
	cases := []map[string]any{
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "nobody", "permissions": "1"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "7"},
		{"path": "/eos/user/e/einstein/missing", "initiator": "einstein", "sharee": "marie", "permissions": "1"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "nobody", "sharee": "marie", "permissions": "1"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "1", "expires_at": "soon"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "einstein", "permissions": "1"},
	}
	for _, args := range cases {
		if _, err := s.Invoke(adminContext(), "create", args); err == nil {
			t.Errorf("create %v must fail", args)
		}
	}
	// a group, by name
	res := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "atlas-users", "permissions": "viewer", "type": "group", "expires_at": "2099-01-01"})
	if g := res["grantee"].(map[string]any); g["type"] != "group" || g["id"] != "atlas-users" || res["expires_at"] == nil || res["expired"] != false {
		t.Fatalf("group share: %v", res)
	}
}

func TestExpiry(t *testing.T) {
	s, prov := newService(t)
	id := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "15"})["id"].(string)

	if _, err := s.Invoke(adminContext(), "sync_expiry", map[string]any{"id": id}); err == nil {
		t.Fatal("sync_expiry on a live share must be refused")
	}
	res := mustInvoke(t, s, "set_expiry", map[string]any{"id": id, "expires_at": "2099-12-31", "sync": "true"})
	if res["expired"] != false || res["grant_removed"] != nil || len(prov.removed) != 0 {
		t.Fatalf("future expiry: %v, removed %v", res, prov.removed)
	}
	res = mustInvoke(t, s, "set_expiry", map[string]any{"id": id, "expires_at": "2020-01-01", "sync": "false"})
	if res["expired"] != true || len(prov.removed) != 0 {
		t.Fatalf("past expiry without sync: %v, removed %v", res, prov.removed)
	}
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "include_expired": "false"})); len(got) != 0 {
		t.Fatalf("expired share listed: %v", got)
	}
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 1 {
		t.Fatalf("expired share hidden by default: %v", got)
	}
	// syncing removes the grant for the right grantee, keeping the record
	sync := mustInvoke(t, s, "sync_expiry", map[string]any{"id": id})
	if sync["grant_removed"] != true || len(prov.removed) != 1 || prov.removed[0].Grantee.GetUserId().GetOpaqueId() != "marie" || !prov.removed[0].Permissions.Delete {
		t.Fatalf("sync: %v, removed %v", sync, prov.removed)
	}
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 1 {
		t.Fatalf("synced share must stay recorded: %v", got)
	}
	// a past date with sync (the default) does both at once
	id2 := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/other", "initiator": "einstein", "sharee": "marie", "permissions": "1"})["id"].(string)
	res = mustInvoke(t, s, "set_expiry", map[string]any{"id": id2, "expires_at": "2020-01-01"})
	if res["grant_removed"] != true || len(prov.removed) != 2 {
		t.Fatalf("set_expiry in the past: %v, removed %v", res, prov.removed)
	}
	// and none clears it
	res = mustInvoke(t, s, "set_expiry", map[string]any{"id": id, "expires_at": "none"})
	if res["expires_at"] != nil {
		t.Fatalf("clearing: %v", res)
	}
}

func TestTransferMoveOrphan(t *testing.T) {
	s, prov := newService(t)
	id := mustInvoke(t, s, "create", map[string]any{"path": "/eos/project/c/cernbox/data", "initiator": "admin1", "sharee": "marie", "permissions": "1"})["id"].(string)
	personal := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": "marie", "permissions": "1"})["id"].(string)

	if _, err := s.Invoke(adminContext(), "transfer", map[string]any{"id": id, "initiator": "marie"}); err == nil {
		t.Fatal("a non-admin must not receive a project share")
	}
	if _, err := s.Invoke(adminContext(), "transfer", map[string]any{"id": personal, "initiator": "admin2"}); err == nil {
		t.Fatal("a personal share cannot be transferred")
	}
	// the manager records the creating user as owner and initiator; a transfer only moves the initiator
	if res := mustInvoke(t, s, "transfer", map[string]any{"id": id, "initiator": "admin2"}); res["initiator"] != "admin2" || res["owner"] != "admin1" {
		t.Fatalf("transfer: %v", res)
	}

	res := mustInvoke(t, s, "move", map[string]any{"id": personal, "path": "/eos/user/e/einstein/other"})
	if r := res["resource"].(map[string]any); r["inode"] != "101" || res["owner"] != "einstein" || res["path"] != "/eos/user/e/einstein/other" || res["grant_applied"] != nil {
		t.Fatalf("move: %v", res)
	}
	// moving with apply_grant also grants the sharee on the new resource
	res = mustInvoke(t, s, "move", map[string]any{"id": personal, "path": "/eos/user/e/einstein/doc", "apply_grant": "true"})
	if res["grant_applied"] != true || len(prov.added) != 1 || prov.added[0].Grantee.GetUserId().GetOpaqueId() != "marie" {
		t.Fatalf("move with grant: %v, added %v", res, prov.added)
	}
	if _, err := s.Invoke(adminContext(), "move", map[string]any{"id": personal, "path": "/eos/project/c/cernbox/data"}); err == nil {
		t.Fatal("moving to a project needs an explicit owner")
	}

	mustInvoke(t, s, "mark_orphan", map[string]any{"id": personal})
	if got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 0 {
		t.Fatalf("orphan listed by default: %v", got)
	}
	got := shares(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "include_orphans": "true"}))
	if len(got) != 1 || got[0]["orphan"] != true || got[0]["orphaned_at"] == nil {
		t.Fatalf("orphan: %v", got)
	}
	// removing an orphan only deletes the record
	if rm := mustInvoke(t, s, "remove", map[string]any{"id": personal}); rm["grant_removed"] != false || len(gw.grantsRemoved) != 0 {
		t.Fatalf("remove orphan: %v", rm)
	}
	// db_only does the same for a live share
	if rm := mustInvoke(t, s, "remove", map[string]any{"id": id, "db_only": "true"}); rm["grant_removed"] != false || len(gw.grantsRemoved) != 0 {
		t.Fatalf("remove db_only: %v", rm)
	}
}

func TestListPaging(t *testing.T) {
	s, _ := newService(t)
	for _, sharee := range []string{"marie", "admin1", "admin2"} {
		mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "sharee": sharee, "permissions": "1"})
	}
	res := mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "limit": "2"})
	if res["count"] != 2 || res["total"] != 3 || res["truncated"] != true {
		t.Fatalf("page 1: %v", res)
	}
	res = mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "limit": "2", "offset": "2"})
	if res["count"] != 1 || res["truncated"] != false {
		t.Fatalf("page 2: %v", res)
	}
}

func TestUnsupportedDriver(t *testing.T) {
	s := &service{conf: &config{Driver: "json"}, sm: stubManager{}}
	s.registerInvocations()
	if _, err := s.Invoke(adminContext(), "list", map[string]any{"owner": "einstein"}); err == nil {
		t.Fatal("a driver without the admin surface must be reported")
	}
	if len(s.Invocations()) == 0 {
		t.Fatal("the catalog must still be advertised")
	}
}

type stubManager struct{ share.Manager }
