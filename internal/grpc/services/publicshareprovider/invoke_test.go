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
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	revaconfig "github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/publicshare/manager/memory"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var einstein = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox"}, Username: "einstein"}

func seed(t *testing.T, s *service, inode string, expires time.Time) string {
	t.Helper()
	md := &provider.ResourceInfo{
		Id:    &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: inode},
		Path:  "/eos/user/e/einstein/" + inode,
		Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Owner: einstein.Id,
	}
	grant := &link.Grant{
		Permissions: &link.PublicSharePermissions{Permissions: &provider.ResourcePermissions{Stat: true}},
		Expiration:  &typespb.Timestamp{Seconds: uint64(expires.Unix())},
	}
	l, err := s.sm.CreatePublicShare(appctx.ContextSetUser(context.Background(), einstein), einstein, md, grant, "", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	return l.Token
}

func TestExpiringPagesByExpirationAndID(t *testing.T) {
	sm, err := sqlmgr.NewPublicShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "links.db")})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{Set: invoke.NewSet(), conf: &config{Driver: "sql"}, sm: sm}
	s.registerInvocations()

	now := time.Now().Truncate(time.Second)
	day := 24 * time.Hour
	want := []string{seed(t, s, "a", now.Add(3*day)), seed(t, s, "b", now.Add(3*day)), seed(t, s, "c", now.Add(5*day))}
	seed(t, s, "d", now.Add(10*day))

	args := map[string]any{"from": now.Format(time.RFC3339), "to": now.Add(7 * day).Format(time.RFC3339), "limit": "2"}
	var got []string
	for range 3 {
		res, err := s.Invoke(context.Background(), "expiring", args)
		if err != nil {
			t.Fatal(err)
		}
		links := res["links"].([]map[string]any)
		if len(links) == 0 {
			break
		}
		for _, l := range links {
			got = append(got, l["token"].(string))
		}
		last := links[len(links)-1]
		args["after_expiration"], args["after_id"] = last["expires_at"], fmt.Sprint(last["id"])
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpiringRefusesBadArguments(t *testing.T) {
	sm, err := memory.New(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &service{Set: invoke.NewSet(), conf: &config{Driver: "memory"}, sm: sm}
	s.registerInvocations()
	for _, args := range []map[string]any{
		{"from": "2026-01-01T00:00:00Z", "to": "2026-01-02T00:00:00Z"},
		{"from": "yesterday", "to": "2026-01-02T00:00:00Z"},
		{"from": "2026-01-02T00:00:00Z", "to": "2026-01-01T00:00:00Z"},
	} {
		if _, err := s.Invoke(context.Background(), "expiring", args); err == nil {
			t.Errorf("%v must be refused", args)
		}
	}
}

func user(name string, groups ...string) *userpb.User {
	return &userpb.User{Id: &userpb.UserId{Idp: "cernbox", OpaqueId: name, Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: name, Groups: groups}
}

var (
	// richard initiated a link on a home of einstein long ago and has no
	// right on it anymore; cbox owns the project, which admin1 and admin2
	// administer.
	users = map[string]*userpb.User{}

	files = []*provider.ResourceInfo{
		{Id: &provider.ResourceId{StorageId: "home", OpaqueId: "100", SpaceId: "home-einstein"}, Path: "/home/einstein/doc", Owner: user("einstein").Id, Type: provider.ResourceType_RESOURCE_TYPE_FILE},
		{Id: &provider.ResourceId{StorageId: "home", OpaqueId: "101", SpaceId: "home-einstein"}, Path: "/home/einstein/other", Owner: user("einstein").Id, Type: provider.ResourceType_RESOURCE_TYPE_FILE},
		{Id: &provider.ResourceId{StorageId: "project", OpaqueId: "200", SpaceId: "project-x"}, Path: "/eos/project/x/xproj/data", Owner: user("cbox").Id, Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER},
		{Id: &provider.ResourceId{StorageId: "home2", OpaqueId: "300", SpaceId: "home-feynman"}, Path: "/home/feynman/doc", Owner: user("feynman").Id, Type: provider.ResourceType_RESOURCE_TYPE_FILE},
	}
	granters = map[string][]string{"home": {"einstein"}, "project": {"cbox", "admin1", "admin2"}, "home2": {"feynman"}}
)

func init() {
	for _, n := range []string{"operator", "einstein", "richard", "cbox", "feynman", "bob"} {
		users[n] = user(n)
	}
	for _, n := range []string{"admin1", "admin2"} {
		users[n] = user(n, "cernbox-project-xproj-admins")
	}
	sharedconf.Init(&revaconfig.Shared{JWTSecret: "publicshareprovider-test-secret"})
	revaservice.SetGlobal(resolver{})
}

type resolver struct{ revaservice.Clients }

func (resolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) { return gw, nil }

var gw = &fakeGateway{}

// fakeGateway answers the gateway calls of the invocations. UpdatePublicShare
// and RemovePublicShare check the caller as the real gateway does: the
// provider must return the link, and the caller must be allowed to change the
// grants of the resource.
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
	g := canGrant(ctx, f.Id.StorageId)
	info.PermissionSet = &provider.ResourcePermissions{Stat: true, AddGrant: g, UpdateGrant: g, RemoveGrant: g}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: info}, nil
}

func (*fakeGateway) GetPath(_ context.Context, req *provider.GetPathRequest, _ ...grpc.CallOption) (*provider.GetPathResponse, error) {
	if f := fileOf(&provider.Reference{ResourceId: req.GetResourceId()}); f != nil {
		return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Path: f.Path}, nil
	}
	return &provider.GetPathResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func (g *fakeGateway) check(ctx context.Context, ref *link.PublicShareReference) *rpc.Status {
	res, _ := g.svc.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: ref})
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

func (g *fakeGateway) UpdatePublicShare(ctx context.Context, req *link.UpdatePublicShareRequest, _ ...grpc.CallOption) (*link.UpdatePublicShareResponse, error) {
	if st := g.check(ctx, req.GetRef()); st != nil {
		return &link.UpdatePublicShareResponse{Status: st}, nil
	}
	return g.svc.UpdatePublicShare(ctx, req)
}

func (g *fakeGateway) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest, _ ...grpc.CallOption) (*link.RemovePublicShareResponse, error) {
	if st := g.check(ctx, req.GetRef()); st != nil {
		return &link.RemovePublicShareResponse{Status: st}, nil
	}
	return g.svc.RemovePublicShare(ctx, req)
}

func newService(t *testing.T, c *config) *service {
	t.Helper()
	sm, err := sqlmgr.NewPublicShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "links.db")})
	if err != nil {
		t.Fatal(err)
	}
	c.Driver = "sql"
	s := &service{Set: invoke.NewSet(), conf: c, sm: sm}
	s.registerInvocations()
	gw.svc, gw.actedAs = s, nil
	return s
}

func as(name string) context.Context {
	return appctx.ContextSetUser(context.Background(), users[name])
}

func createLink(t *testing.T, s *service, path, owner, initiator string, exp time.Time, password string) (string, string) {
	t.Helper()
	grant := &link.Grant{
		Permissions: &link.PublicSharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()},
		Password:    password,
	}
	if !exp.IsZero() {
		grant.Expiration = &typespb.Timestamp{Seconds: uint64(exp.Unix())}
	}
	l, err := s.sm.CreatePublicShare(as(owner), users[owner], fileOf(&provider.Reference{Path: path}), grant, "", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if initiator != owner {
		if err := s.sm.(adminManager).TransferPublicShare(as(owner), &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: l.Id}}, initiator); err != nil {
			t.Fatal(err)
		}
	}
	return l.Id.OpaqueId, l.Token
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

func cs3(r invoke.Result) map[string]any { return r["public_share"].(map[string]any) }

func idOf(r invoke.Result) string { return cs3(r)["id"].(map[string]any)["opaqueId"].(string) }

func ids(rs []invoke.Result) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, idOf(r))
	}
	return out
}

func TestEveryInvocationIsShared(t *testing.T) {
	s := newService(t, &config{})
	for _, spec := range s.Invocations() {
		if spec.Scope != invoke.ScopeShared {
			t.Errorf("%s is not shared", spec.Name)
		}
	}
}

func TestList(t *testing.T) {
	s := newService(t, &config{})
	doc, docToken := createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "")
	other, _ := createLink(t, s, "/home/einstein/other", "einstein", "einstein", time.Now().Add(-time.Hour), "")
	project, _ := createLink(t, s, "/eos/project/x/xproj/data", "cbox", "admin1", time.Time{}, "")

	for _, tc := range []struct {
		args map[string]any
		want []string
	}{
		{map[string]any{"owner": "einstein"}, []string{doc, other}},
		{map[string]any{"owner": "einstein", "include_expired": "false"}, []string{doc}},
		{map[string]any{"initiator": "admin1"}, []string{project}},
		{map[string]any{"owner": admin.UserArg(user("einstein").Id)}, []string{doc, other}},
		{map[string]any{"space": "home-einstein"}, []string{doc, other}},
		{map[string]any{"space": "project-x", "owner": "einstein"}, []string{}},
		{map[string]any{"resource_id": "home!101"}, []string{other}},
		{map[string]any{"resource_id": "home$home-einstein!100", "space": "home-einstein"}, []string{doc}},
		{map[string]any{"id": project}, []string{project}},
		{map[string]any{"token": docToken}, []string{doc}},
		{map[string]any{"id": "999"}, []string{}},
		{map[string]any{"token": "nope"}, []string{}},
	} {
		if got := ids(list(t, s, tc.args)); !slices.Equal(got, tc.want) {
			t.Errorf("list %v: got %v, want %v", tc.args, got, tc.want)
		}
	}

	for _, args := range []map[string]any{{}, {"include_orphans": "true"}, {"resource_id": "home"}} {
		if err := s.InvokeStream(as("operator"), "list", args, func(invoke.Result) error { return nil }); err == nil {
			t.Errorf("list %v: expected an error", args)
		}
	}

	got := list(t, s, map[string]any{"initiator": "admin1", "resolve_paths": "true"})
	if len(got) != 1 || got[0]["path"] != "/eos/project/x/xproj/data" {
		t.Fatalf("resolve_paths: got %v", got)
	}
	if c := cs3(got[0]); c["owner"].(map[string]any)["opaqueId"] != "cbox" || c["creator"].(map[string]any)["opaqueId"] != "admin1" {
		t.Fatalf("unexpected link %v", c)
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
	s := newService(t, &config{PublicLinkBaseURL: "https://example.org/s/"})
	id, token := createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "secret")

	for _, args := range []map[string]any{{"id": id}, {"token": token}} {
		res := mustInvoke(t, s, "get", args)
		extra := res["extra"].(map[string]string)
		if idOf(res) != id || res["path"] != "/home/einstein/doc" || res["password_protected"] != true ||
			extra["url"] != "https://example.org/s/"+token || extra["initial_path"] != "/home/einstein/doc" {
			t.Fatalf("get %v: unexpected result %v", args, res)
		}
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		l, err := s.sm.(adminManager).GetPublicLink(context.Background(), &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: token}}, false)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), l.Password) || strings.Contains(string(b), "secret") {
			t.Fatalf("the result holds the password: %s", b)
		}
	}
	for _, args := range []map[string]any{{"id": "999"}, {"token": "nope"}} {
		if _, err := s.Invoke(as("operator"), "get", args); !isNotFound(err) {
			t.Fatalf("get %v: expected not found, got %v", args, err)
		}
	}
	if _, err := s.Invoke(as("operator"), "get", map[string]any{}); err == nil {
		t.Fatal("expected an error without id or token")
	}

	s = newService(t, &config{})
	id, _ = createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "")
	if res := mustInvoke(t, s, "get", map[string]any{"id": id}); res["extra"].(map[string]string)["url"] != "" || res["password_protected"] != nil {
		t.Fatalf("unexpected result %v", res)
	}
}

func TestGetPublicShareNotFoundStatus(t *testing.T) {
	s := newService(t, &config{})
	id, token := createLink(t, s, "/home/einstein/doc", "einstein", "richard", time.Time{}, "")
	get := func(u string, ref *link.PublicShareReference) rpc.Code {
		t.Helper()
		res, err := s.GetPublicShare(as(u), &link.GetPublicShareRequest{Ref: ref})
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		return res.GetStatus().GetCode()
	}
	byID := func(id string) *link.PublicShareReference { ref, _ := linkRef(id, ""); return ref }

	for _, u := range []string{"einstein", "richard"} {
		if c := get(u, byID(id)); c != rpc.Code_CODE_OK {
			t.Errorf("%s: got %v", u, c)
		}
	}
	if c := get("einstein", byID("999")); c != rpc.Code_CODE_NOT_FOUND {
		t.Errorf("a missing link: got %v", c)
	}
	if c := get("bob", &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: token}}); c != rpc.Code_CODE_OK {
		t.Errorf("by token: got %v", c)
	}
}

func TestUpdate(t *testing.T) {
	s := newService(t, &config{})
	// richard cannot change the link anymore, so the owner does it.
	id, _ := createLink(t, s, "/home/einstein/doc", "einstein", "richard", time.Time{}, "")
	ref, _ := linkRef(id, "")

	res := mustInvoke(t, s, "update", map[string]any{"id": id, "role": "editor", "expires_at": "2099-01-01", "password": "secret", "fallback": "true"})
	if !slices.Equal(gw.actedAs, []string{"einstein", "einstein", "einstein"}) {
		t.Fatalf("acted as %v, want einstein for the three fields", gw.actedAs)
	}
	l, err := s.sm.(adminManager).GetPublicLink(context.Background(), ref, false)
	if err != nil {
		t.Fatal(err)
	}
	if permissions.RoleFromOCSPermissions(permissions.OcsPermissions(l.Permissions)).Name != permissions.RoleEditor ||
		!l.Expiration.Valid || l.Expiration.V.Year() != 2099 || l.Password == "" {
		t.Fatalf("link not updated: %+v", l.ProtoShare)
	}
	if cs3(res)["expiration"] == nil || res["password_protected"] != true {
		t.Fatalf("unexpected result %v", res)
	}

	mustInvoke(t, s, "update", map[string]any{"id": id, "expires_at": "none", "password": ""})
	if l, _ = s.sm.(adminManager).GetPublicLink(context.Background(), ref, false); l.Expiration.Valid || l.Password != "" {
		t.Fatalf("expiration or password not cleared: %+v", l)
	}

	for _, args := range []map[string]any{
		{"id": id},
		{"id": id, "role": "manager"},
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
	s := newService(t, &config{})
	// bob owns nothing here: the gateway refuses both users.
	id, _ := createLink(t, s, "/home/einstein/doc", "bob", "richard", time.Time{}, "")
	if _, err := s.Invoke(as("operator"), "update", map[string]any{"id": id, "role": "editor"}); err == nil {
		t.Fatal("expected an error")
	}
	if len(gw.actedAs) != 0 {
		t.Fatalf("acted as %v", gw.actedAs)
	}
}

func TestUpdateRWLinkMaxExpiration(t *testing.T) {
	s := newService(t, &config{PubRWLinkMaxExpiration: int64(7 * 24 * time.Hour / time.Second)})
	id, _ := createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "")
	soon := time.Now().Add(3 * 24 * time.Hour).Format(time.DateOnly)
	late := time.Now().Add(30 * 24 * time.Hour).Format(time.DateOnly)

	for _, args := range []map[string]any{
		{"id": id, "role": "editor"},
		{"id": id, "role": "editor", "expires_at": late},
	} {
		if _, err := s.Invoke(as("operator"), "update", args); err == nil {
			t.Errorf("update %v: expected an error", args)
		}
	}
	mustInvoke(t, s, "update", map[string]any{"id": id, "role": "editor", "expires_at": soon})
	for _, args := range []map[string]any{
		{"id": id, "expires_at": "none"},
		{"id": id, "expires_at": late},
	} {
		if _, err := s.Invoke(as("operator"), "update", args); err == nil {
			t.Errorf("update %v: expected an error", args)
		}
	}
	mustInvoke(t, s, "update", map[string]any{"id": id, "role": "viewer", "expires_at": "none"})

	// A legacy editor link without an expiration can still change its
	// password.
	legacy, _ := createLink(t, s, "/home/einstein/other", "einstein", "einstein", time.Time{}, "")
	ref, _ := linkRef(legacy, "")
	if _, err := s.sm.UpdatePublicShare(as("einstein"), users["einstein"], &link.UpdatePublicShareRequest{Ref: ref, Update: &link.UpdatePublicShareRequest_Update{
		Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
		Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: permissions.NewEditorRole().CS3ResourcePermissions()}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	mustInvoke(t, s, "update", map[string]any{"id": legacy, "password": "secret"})
}

func TestRemove(t *testing.T) {
	s := newService(t, &config{})
	viaGateway, _ := createLink(t, s, "/eos/project/x/xproj/data", "cbox", "admin1", time.Time{}, "")
	dbOnly, _ := createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "")
	orphan, _ := createLink(t, s, "/home/einstein/other", "einstein", "einstein", time.Time{}, "")
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
			t.Errorf("link %s: expected not found, got %v", id, err)
		}
	}
}

func TestTransfer(t *testing.T) {
	s := newService(t, &config{})
	id, _ := createLink(t, s, "/eos/project/x/xproj/data", "cbox", "admin1", time.Time{}, "")

	if _, err := s.Invoke(as("operator"), "transfer", map[string]any{"id": id, "initiator": "bob"}); err == nil {
		t.Fatal("expected an error for a user who cannot share the resource")
	}
	res := mustInvoke(t, s, "transfer", map[string]any{"id": id, "initiator": admin.UserArg(user("admin2").Id)})
	if c := cs3(res); c["creator"].(map[string]any)["opaqueId"] != "admin2" || c["owner"].(map[string]any)["opaqueId"] != "cbox" {
		t.Fatalf("unexpected link %v", c)
	}
}

func TestMove(t *testing.T) {
	s := newService(t, &config{})
	id, token := createLink(t, s, "/home/einstein/doc", "einstein", "einstein", time.Time{}, "")

	res := mustInvoke(t, s, "move", map[string]any{"token": token, "path": "/home/feynman/doc", "as": "feynman"})
	rid := cs3(res)["resourceId"].(map[string]any)
	if idOf(res) != id || res["path"] != "/home/feynman/doc" || cs3(res)["owner"].(map[string]any)["opaqueId"] != "feynman" ||
		rid["storageId"] != "home2" || rid["opaqueId"] != "300" || rid["spaceId"] != "home-feynman" {
		t.Fatalf("unexpected result %v", res)
	}
	if _, err := s.Invoke(as("operator"), "move", map[string]any{"id": id, "path": "/nowhere"}); err == nil {
		t.Fatal("expected an error for a path that does not exist")
	}
}

func TestUnsupportedDriver(t *testing.T) {
	sm, err := memory.New(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &service{Set: invoke.NewSet(), conf: &config{Driver: "memory"}, sm: sm}
	s.registerInvocations()
	_, err = s.Invoke(as("operator"), "get", map[string]any{"id": "1"})
	if _, ok := err.(errtypes.IsNotSupported); !ok {
		t.Fatalf("get: expected not supported, got %v", err)
	}
	err = s.InvokeStream(as("operator"), "list", map[string]any{"owner": "einstein"}, func(invoke.Result) error { return nil })
	if _, ok := err.(errtypes.IsNotSupported); !ok {
		t.Fatalf("list: expected not supported, got %v", err)
	}
}
