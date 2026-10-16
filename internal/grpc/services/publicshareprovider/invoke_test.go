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

package publicshareprovider

import (
	"context"
	"path/filepath"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var (
	operator   = &userpb.User{Id: &userpb.UserId{OpaqueId: "operator"}, Username: "operator"}
	einstein   = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox"}, Username: "einstein"}
	marie      = &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "cernbox"}, Username: "marie"}
	cernboxsvc = &userpb.User{Id: &userpb.UserId{OpaqueId: "cernboxsvc", Idp: "cernbox"}, Username: "cernboxsvc"}
	admin1     = &userpb.User{Id: &userpb.UserId{OpaqueId: "admin1", Idp: "cernbox"}, Username: "admin1", Groups: []string{"cernbox-project-cernbox-admins"}}
	admin2     = &userpb.User{Id: &userpb.UserId{OpaqueId: "admin2", Idp: "cernbox"}, Username: "admin2", Groups: []string{"cernbox-project-cernbox-admins"}}
	users      = []*userpb.User{operator, einstein, marie, cernboxsvc, admin1, admin2}

	files = map[string]*provider.ResourceInfo{
		"/eos/user/e/einstein/doc":    {Id: &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "100", SpaceId: "home-e"}, Path: "/eos/user/e/einstein/doc", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: einstein.Id},
		"/eos/user/e/einstein/other":  {Id: &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: "101", SpaceId: "home-e"}, Path: "/eos/user/e/einstein/other", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: einstein.Id},
		"/eos/project/c/cernbox/data": {Id: &provider.ResourceId{StorageId: "eosproject-c", OpaqueId: "200", SpaceId: "project-cernbox"}, Path: "/eos/project/c/cernbox/data", Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Owner: cernboxsvc.Id},
	}
)

type fakeGateway struct {
	gateway.GatewayAPIClient
	svc *service
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
		// a copy, since create annotates the info with the link name
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: proto.Clone(info).(*provider.ResourceInfo)}, nil
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

func (g *fakeGateway) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest, _ ...grpc.CallOption) (*link.CreatePublicShareResponse, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok || u.Username == "operator" {
		return &link.CreatePublicShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "not a user"}}, nil
	}
	l, err := g.svc.sm.CreatePublicShare(ctx, u, req.ResourceInfo, req.Grant, req.Description, req.Internal, req.NotifyUploads, req.NotifyUploadsExtraRecipients)
	if err != nil {
		return &link.CreatePublicShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL, Message: err.Error()}}, nil
	}
	return &link.CreatePublicShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: l}, nil
}

type resolver struct{ revaservice.Clients }

var gw = &fakeGateway{}

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return gw, nil }

func init() {
	revaservice.SetGlobal(resolver{Clients: revaservice.Global()})
}

func newService(t *testing.T) *service {
	t.Helper()
	sm, err := sqlmgr.NewPublicShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "links.db")})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := admin.NewIdentity("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	s := &service{conf: &config{Driver: "sql"}, sm: sm, identity: identity}
	s.registerInvocations()
	gw.svc = s
	return s
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

func links(t *testing.T, res invoke.Result) []invoke.Result {
	t.Helper()
	out, ok := res["links"].([]invoke.Result)
	if !ok {
		t.Fatalf("no links in %v", res)
	}
	return out
}

func TestCreateListRemove(t *testing.T) {
	s := newService(t)
	res := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "editor", "password": "s3cret", "expires_at": "2099-01-01", "name": "handout"})
	if res["created"] != true || res["owner"] != "einstein" || res["initiator"] != "einstein" || res["permissions"] != 15 || res["password_protected"] != true || res["name"] != "handout" || res["expired"] != false {
		t.Fatalf("create: %v", res)
	}
	for k := range res {
		if k == "password" {
			t.Fatal("the password hash must not be returned")
		}
	}
	id, token := res["id"].(string), res["token"].(string)
	if token == "" {
		t.Fatal("no token")
	}
	for _, args := range []map[string]any{
		{"owner": "einstein"}, {"initiator": "einstein"}, {"id": id}, {"token": token}, {"space": "home-e"},
		{"resource": "eoshome-e:100"}, {"path": "/eos/user/e/einstein/doc"}, {"expiring_from": "2098-12-31", "expiring_to": "2099-01-02"},
	} {
		if got := links(t, mustInvoke(t, s, "list", args)); len(got) != 1 || got[0]["id"] != id {
			t.Fatalf("list %v: %v", args, got)
		}
	}
	if got := links(t, mustInvoke(t, s, "list", map[string]any{"expiring_from": "2099-01-02"})); len(got) != 0 {
		t.Fatalf("window after the expiry: %v", got)
	}
	if _, err := s.Invoke(adminContext(), "list", map[string]any{}); err == nil {
		t.Fatal("list without a selector must be refused")
	}
	if got := mustInvoke(t, s, "get", map[string]any{"token": token, "resolve_path": "true"}); got["path"] != "/eos/user/e/einstein/doc" {
		t.Fatalf("get: %v", got)
	}
	// the manager hashed the password
	if _, err := s.sm.GetPublicShareByToken(context.Background(), token, &link.PublicShareAuthentication{Spec: &link.PublicShareAuthentication_Password{Password: "s3cret"}}, false); err != nil {
		t.Fatalf("password check: %v", err)
	}

	mustInvoke(t, s, "remove", map[string]any{"token": token})
	if got := links(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 0 {
		t.Fatalf("removed link listed: %v", got)
	}
}

func TestCreateValidation(t *testing.T) {
	s := newService(t)
	cases := []map[string]any{
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "editor"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "3"},
		{"path": "/eos/user/e/einstein/missing", "initiator": "einstein", "permissions": "1"},
		{"path": "/eos/user/e/einstein/doc", "initiator": "nobody", "permissions": "1"},
	}
	for _, args := range cases {
		if _, err := s.Invoke(adminContext(), "create", args); err == nil {
			t.Errorf("create %v must fail", args)
		}
	}
	res := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "editor", "force": "true"})
	if res["expires_at"] != nil || res["role"] != "editor" {
		t.Fatalf("forced editor link: %v", res)
	}
	res = mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "uploader"})
	if res["permissions"] != 4 || res["role"] != "uploader" {
		t.Fatalf("uploader link: %v", res)
	}
}

func TestUpdates(t *testing.T) {
	s := newService(t)
	id := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "editor", "expires_at": "2099-01-01"})["id"].(string)

	if _, err := s.Invoke(adminContext(), "set_expiry", map[string]any{"id": id, "expires_at": "none"}); err == nil {
		t.Fatal("clearing the expiry of an editor link needs force")
	}
	if res := mustInvoke(t, s, "set_expiry", map[string]any{"id": id, "expires_at": "none", "force": "true"}); res["expires_at"] != nil {
		t.Fatalf("clear: %v", res)
	}
	res := mustInvoke(t, s, "set_expiry", map[string]any{"id": id, "expires_at": "2020-01-01"})
	if res["expired"] != true {
		t.Fatalf("past: %v", res)
	}
	if got := links(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "include_expired": "false"})); len(got) != 0 {
		t.Fatalf("expired link listed: %v", got)
	}
	if res := mustInvoke(t, s, "set_password", map[string]any{"id": id, "password": "pw"}); res["password_protected"] != true {
		t.Fatalf("set_password: %v", res)
	}
	if res := mustInvoke(t, s, "set_password", map[string]any{"id": id, "password": ""}); res["password_protected"] != false {
		t.Fatalf("clear password: %v", res)
	}

	res = mustInvoke(t, s, "move", map[string]any{"id": id, "path": "/eos/user/e/einstein/other"})
	if r := res["resource"].(map[string]any); r["inode"] != "101" || res["path"] != "/eos/user/e/einstein/other" {
		t.Fatalf("move: %v", res)
	}
	mustInvoke(t, s, "mark_orphan", map[string]any{"id": id})
	if got := links(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein"})); len(got) != 0 {
		t.Fatalf("orphan listed: %v", got)
	}
	if got := links(t, mustInvoke(t, s, "list", map[string]any{"owner": "einstein", "include_orphans": "true"})); len(got) != 1 || got[0]["orphan"] != true {
		t.Fatalf("orphan hidden: %v", got)
	}
}

func TestTransfer(t *testing.T) {
	s := newService(t)
	id := mustInvoke(t, s, "create", map[string]any{"path": "/eos/project/c/cernbox/data", "initiator": "admin1", "permissions": "1"})["id"].(string)
	personal := mustInvoke(t, s, "create", map[string]any{"path": "/eos/user/e/einstein/doc", "initiator": "einstein", "permissions": "1"})["id"].(string)
	if _, err := s.Invoke(adminContext(), "transfer", map[string]any{"id": id, "initiator": "marie"}); err == nil {
		t.Fatal("a non-admin must not receive a project link")
	}
	if _, err := s.Invoke(adminContext(), "transfer", map[string]any{"id": personal, "initiator": "admin2"}); err == nil {
		t.Fatal("a personal link cannot be transferred")
	}
	if res := mustInvoke(t, s, "transfer", map[string]any{"id": id, "initiator": "admin2"}); res["initiator"] != "admin2" || res["owner"] != "admin1" {
		t.Fatalf("transfer: %v", res)
	}
}

func TestUnsupportedDriver(t *testing.T) {
	s := &service{conf: &config{Driver: "json"}, sm: stubManager{}}
	s.registerInvocations()
	if _, err := s.Invoke(adminContext(), "list", map[string]any{"owner": "einstein"}); err == nil {
		t.Fatal("a driver without the admin surface must be reported")
	}
}

type stubManager struct{ publicshare.Manager }
