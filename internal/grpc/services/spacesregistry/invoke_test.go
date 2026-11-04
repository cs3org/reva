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

package spacesregistry

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/projects"
	projectsql "github.com/cs3org/reva/v3/pkg/projects/manager/sql"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/stats"
	"google.golang.org/grpc"
)

func user(name string, status userpb.UserStatus, groups ...string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: name}, Username: name, Status: status, Groups: groups}
}

var (
	users = map[string]*userpb.User{
		"operator": user("operator", userpb.UserStatus_USER_STATUS_ACTIVE),
		"einstein": user("einstein", userpb.UserStatus_USER_STATUS_ACTIVE, "cernbox-project-atlas-writers"),
		"cboxsvc":  user("cboxsvc", userpb.UserStatus_USER_STATUS_ACTIVE),
		"atlassvc": user("atlassvc", userpb.UserStatus_USER_STATUS_ACTIVE),
		"leaving":  user("leaving", userpb.UserStatus_USER_STATUS_EXPIRING),
	}
	// paths are the project roots that exist, by the user who can stat them.
	paths   = map[string]string{"/eos/project/a/atlas": "atlassvc", "/eos/project/c/cernbox": "cboxsvc"}
	members = map[string][]string{"cernbox-project-atlas-readers": {}, "cernbox-project-atlas-writers": {"einstein"}, "cernbox-project-atlas-admins": {}, "cernbox-project-cernbox-readers": {}, "cernbox-project-cernbox-writers": {}, "cernbox-project-cernbox-admins": {}}
)

func init() {
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "spacesregistry-test-secret"})
	revaservice.SetGlobal(resolver{})
}

type resolver struct{ revaservice.Clients }

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return fakeGateway{}, nil }

type fakeGateway struct{ gateway.GatewayAPIClient }

// GetUser matches the opaque id, and the idp if one is given, as the user
// providers do: the catalogue keeps no idp.
func (fakeGateway) GetUser(_ context.Context, req *userpb.GetUserRequest, _ ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	id := req.GetUserId()
	if u, ok := users[id.GetOpaqueId()]; ok && (id.GetIdp() == "" || id.GetIdp() == u.GetId().GetIdp()) {
		return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: u}, nil
	}
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (fakeGateway) GetUserGroups(_ context.Context, req *userpb.GetUserGroupsRequest, _ ...grpc.CallOption) (*userpb.GetUserGroupsResponse, error) {
	return &userpb.GetUserGroupsResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Groups: users[req.GetUserId().GetOpaqueId()].GetGroups()}, nil
}

func (fakeGateway) GetMembers(_ context.Context, req *grouppb.GetMembersRequest, _ ...grpc.CallOption) (*grouppb.GetMembersResponse, error) {
	m, ok := members[req.GetGroupId().GetOpaqueId()]
	if !ok {
		return &grouppb.GetMembersResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	res := &grouppb.GetMembersResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}
	for _, u := range m {
		res.Members = append(res.Members, &userpb.UserId{OpaqueId: u})
	}
	return res, nil
}

func (fakeGateway) Stat(ctx context.Context, req *provider.StatRequest, _ ...grpc.CallOption) (*provider.StatResponse, error) {
	u, _ := appctx.ContextGetUser(ctx)
	if by, ok := paths[req.GetRef().GetPath()]; ok && by == u.GetUsername() {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: &provider.ResourceInfo{Path: req.GetRef().GetPath()}}, nil
	}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func newService(t *testing.T) *service {
	t.Helper()
	cat, err := projectsql.New(context.Background(), map[string]any{
		"db_engine":              "sqlite",
		"db_name":                filepath.Join(t.TempDir(), "projects.db"),
		"path_template":          "/eos/project/{{ .Initial }}/{{ .Name }}",
		"storage_id_template":    "newproject-{{ .Initial }}",
		"readers_group_template": "cernbox-project-{{ .Name }}-readers",
		"writers_group_template": "cernbox-project-{{ .Name }}-writers",
		"admins_group_template":  "cernbox-project-{{ .Name }}-admins",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{c: &config{Driver: "sql"}, projects: cat}
	s.Set = s.invocations()
	return s
}

func adminContext() context.Context {
	return appctx.ContextSetUser(context.Background(), users["operator"])
}

// result is the JSON form of an invocation result, as the control channel
// sends it.
func result(t *testing.T, r invoke.Result) map[string]any {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mustInvoke(t *testing.T, s *service, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := s.Invoke(adminContext(), name, args)
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return result(t, res)
}

func stream(t *testing.T, s *service, name string, args map[string]any) []map[string]any {
	t.Helper()
	var out []map[string]any
	err := s.InvokeStream(adminContext(), name, args, func(r invoke.Result) error {
		out = append(out, result(t, r))
		return nil
	})
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return out
}

func spaceOf(r map[string]any) map[string]any { return r["space"].(map[string]any) }

func names(rs []map[string]any) []string {
	var out []string
	for _, r := range rs {
		out = append(out, spaceOf(r)["name"].(string))
	}
	return out
}

func TestCatalogue(t *testing.T) {
	s := newService(t)
	specs := map[string]invoke.InvocationSpec{}
	for _, sp := range s.Invocations() {
		specs[sp.Name] = sp
	}
	for _, n := range []string{"list", "get", "update", "set_status", "set_owner", "create", "delete", "check"} {
		if specs[n].Scope != invoke.ScopeShared {
			t.Errorf("%s must be shared: the catalogue is one database", n)
		}
	}
	if specs["flush_cache"].Scope != invoke.ScopeInstance {
		t.Error("flush_cache must run on every instance: each one has its cache")
	}
	if specs["create"].Kind != invoke.KindDangerous || specs["delete"].Kind != invoke.KindDangerous {
		t.Error("create and delete are dangerous")
	}
	if _, ok := specs[stats.Invocation]; !ok {
		t.Error("the stats invocation of the catalogue must be listed")
	}
	if _, ok := specs["space"]; !ok {
		t.Error("space must stay")
	}
}

func TestCreateListGet(t *testing.T) {
	s := newService(t)
	r := mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cboxsvc", "status": "creating"})
	sp := spaceOf(r)
	if sp["name"] != "cernbox" || sp["spaceType"] != "project" || sp["rootInfo"].(map[string]any)["path"] != "/eos/project/c/cernbox" || r["status"] != "creating" {
		t.Fatalf("create: %v", r)
	}
	if id := sp["id"].(map[string]any)["opaqueId"].(string); id[:len("newproject-c$")] != "newproject-c$" {
		t.Fatalf("storage id: %s", id)
	}
	if r["extra"].(map[string]any)["created_at"] == nil {
		t.Fatalf("extra: %v", r)
	}
	mustInvoke(t, s, "create", map[string]any{"name": "atlas", "owner": "atlassvc"})

	for _, args := range []map[string]any{
		{"name": "zzz", "owner": "nobody"},                     // unknown owner
		{"name": "cernbox", "owner": "einstein"},               // taken
		{"name": "zzz", "owner": "einstein", "status": "gone"}, // unknown status
	} {
		if _, err := s.Invoke(adminContext(), "create", args); err == nil {
			t.Errorf("create %v must fail", args)
		}
	}

	if got := names(stream(t, s, "list", nil)); !slices.Equal(got, []string{"atlas", "cernbox"}) {
		t.Fatalf("list: %v", got)
	}
	if got := names(stream(t, s, "list", map[string]any{"status": "creating"})); !slices.Equal(got, []string{"cernbox"}) {
		t.Fatalf("list by status: %v", got)
	}
	if got := names(stream(t, s, "list", map[string]any{"created_after": "1h"})); len(got) != 2 {
		t.Fatalf("created in the last hour: %v", got)
	}
	// einstein reaches atlas through its writers group, which the gateway
	// gives.
	rs := stream(t, s, "list", map[string]any{"member": admin.UserArg(users["einstein"].Id), "type": "project"})
	if got := names(rs); !slices.Equal(got, []string{"atlas"}) {
		t.Fatalf("list by member: %v", got)
	}
	if perms := spaceOf(rs[0])["rootInfo"].(map[string]any)["permissionSet"].(map[string]any); perms["initiateFileUpload"] != true {
		t.Fatalf("the permissions of the member: %v", perms)
	}
	if err := s.InvokeStream(adminContext(), "list", map[string]any{"type": "personal"}, func(invoke.Result) error { return nil }); err == nil {
		t.Fatal("the catalogue has no personal spaces")
	}

	if got := mustInvoke(t, s, "get", map[string]any{"name": "atlas"}); spaceOf(got)["owner"].(map[string]any)["id"].(map[string]any)["opaqueId"] != "atlassvc" {
		t.Fatalf("get: %v", got)
	}
	id := spaceOf(mustInvoke(t, s, "get", map[string]any{"name": "atlas"}))["id"].(map[string]any)["opaqueId"]
	if got := mustInvoke(t, s, "get", map[string]any{"id": id}); spaceOf(got)["name"] != "atlas" {
		t.Fatalf("get by id: %v", got)
	}
	_, err := s.Invoke(adminContext(), "get", map[string]any{"name": "nope"})
	var nf errtypes.IsNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("an unknown project must be not found: %v", err)
	}
}

func TestUpdateActsAsTheOwner(t *testing.T) {
	s := newService(t)
	mustInvoke(t, s, "create", map[string]any{"name": "atlas", "owner": "atlassvc"})
	r := mustInvoke(t, s, "update", map[string]any{"name": "atlas", "description": "ATLAS data", "readme": "/eos/project/a/atlas/README.md", "fallback": "true"})
	if sp := spaceOf(r); sp["description"] != "ATLAS data" || sp["readmeId"] != "/eos/project/a/atlas/README.md" {
		t.Fatalf("update: %v", r)
	}
	if _, err := s.Invoke(adminContext(), "update", map[string]any{"name": "atlas"}); err == nil {
		t.Fatal("an update without a field must fail")
	}

	// Without an owner that the userprovider knows, nobody can change it.
	mustInvoke(t, s, "create", map[string]any{"name": "cms", "owner": "einstein"})
	delete(users, "einstein")
	defer func() {
		users["einstein"] = user("einstein", userpb.UserStatus_USER_STATUS_ACTIVE, "cernbox-project-atlas-writers")
	}()
	if _, err := s.Invoke(adminContext(), "update", map[string]any{"name": "cms", "description": "x"}); err == nil {
		t.Fatal("an owner that is gone must be reported")
	}
}

func TestStatusOwnerDelete(t *testing.T) {
	s := newService(t)
	mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cboxsvc"})

	if _, err := s.Invoke(adminContext(), "set_status", map[string]any{"name": "cernbox", "status": "gone"}); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
	if r := mustInvoke(t, s, "set_status", map[string]any{"name": "cernbox", "status": "archiving"}); r["status"] != "archiving" {
		t.Fatalf("archiving: %v", r)
	}
	if _, err := s.Invoke(adminContext(), "set_owner", map[string]any{"name": "cernbox", "owner": "nobody"}); err == nil {
		t.Fatal("an unknown owner must be rejected")
	}
	elsewhere := admin.UserArg(&userpb.UserId{Idp: "elsewhere", OpaqueId: "einstein"})
	if _, err := s.Invoke(adminContext(), "set_owner", map[string]any{"name": "cernbox", "owner": elsewhere}); err == nil {
		t.Fatal("a user of another idp must be rejected")
	}
	if r := mustInvoke(t, s, "set_owner", map[string]any{"name": "cernbox", "owner": admin.UserArg(users["einstein"].Id)}); spaceOf(r)["owner"].(map[string]any)["id"].(map[string]any)["opaqueId"] != "einstein" {
		t.Fatalf("set_owner: %v", r)
	}
	if r := mustInvoke(t, s, "set_status", map[string]any{"name": "cernbox", "status": "archived"}); r["extra"].(map[string]any)["archived_at"] == nil {
		t.Fatalf("archived: %v", r)
	}
	if _, err := s.Invoke(adminContext(), "update", map[string]any{"name": "cernbox", "description": "x"}); err == nil {
		t.Fatal("an archived project cannot be updated")
	}
	if got := stream(t, s, "list", nil); len(got) != 0 {
		t.Fatalf("an archived project is not listed by default: %v", got)
	}
	if got := stream(t, s, "list", map[string]any{"include_archived": "true"}); len(got) != 1 {
		t.Fatalf("include_archived: %v", got)
	}

	if r := mustInvoke(t, s, "delete", map[string]any{"name": "cernbox"}); spaceOf(r)["name"] != "cernbox" {
		t.Fatalf("delete: %v", r)
	}
	if _, err := s.Invoke(adminContext(), "get", map[string]any{"name": "cernbox"}); err == nil {
		t.Fatal("a deleted project must not be found")
	}
	mustInvoke(t, s, "delete", map[string]any{"name": "cernbox", "hard": "true"})
	mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cboxsvc"})
	if r := mustInvoke(t, s, "flush_cache", nil); r["flushed"] != true {
		t.Fatalf("flush: %v", r)
	}
}

func TestCheck(t *testing.T) {
	s := newService(t)
	// atlas is fine: its path stats as the owner and einstein writes to it.
	mustInvoke(t, s, "create", map[string]any{"name": "atlas", "owner": "atlassvc"})
	// cernbox exists, but its groups have no members.
	mustInvoke(t, s, "create", map[string]any{"name": "cernbox", "owner": "cboxsvc"})
	// alice has no directory and an owner that is about to go.
	mustInvoke(t, s, "create", map[string]any{"name": "alice", "owner": "leaving", "readers": "cernbox-project-atlas-writers"})
	// bob has an owner that is gone.
	mustInvoke(t, s, "create", map[string]any{"name": "bob", "owner": "einstein", "readers": "cernbox-project-atlas-writers"})
	delete(users, "einstein")
	defer func() {
		users["einstein"] = user("einstein", userpb.UserStatus_USER_STATUS_ACTIVE, "cernbox-project-atlas-writers")
	}()

	kinds := map[string][]string{}
	for _, r := range stream(t, s, "check", nil) {
		for _, f := range r["findings"].([]any) {
			name := spaceOf(r["space"].(map[string]any))["name"].(string)
			kinds[name] = append(kinds[name], f.(map[string]any)["kind"].(string))
		}
	}
	want := map[string][]string{
		"alice":   {"owner_expiring", "path_missing", "group_missing", "group_missing"},
		"bob":     {"owner_missing", "group_missing", "group_missing"},
		"cernbox": {"no_members"},
	}
	if len(kinds) != len(want) {
		t.Fatalf("check: %v", kinds)
	}
	for n, w := range want {
		if !slices.Equal(kinds[n], w) {
			t.Errorf("%s: got %v, want %v", n, kinds[n], w)
		}
	}
}

func TestUnsupportedDriver(t *testing.T) {
	s := &service{c: &config{Driver: "memory"}, projects: stubCatalogue{}}
	s.Set = s.invocations()
	_, err := s.Invoke(adminContext(), "get", map[string]any{"name": "x"})
	var ns errtypes.IsNotSupported
	if !errors.As(err, &ns) {
		t.Fatalf("a driver without the admin surface must be reported: %v", err)
	}
	for _, sp := range s.Invocations() {
		if sp.Name == stats.Invocation {
			t.Fatal("a driver that reports no stats has no stats invocation")
		}
	}
}

type stubCatalogue struct{ projects.Catalogue }
