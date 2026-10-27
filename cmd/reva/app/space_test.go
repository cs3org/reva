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

package revacmd

import (
	"cmp"
	"context"
	"encoding/json"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func testSpace(id, name, typ, owner, path string) *provider.StorageSpace {
	return &provider.StorageSpace{
		Id:        &provider.StorageSpaceId{OpaqueId: id},
		Name:      name,
		SpaceType: typ,
		Owner:     &userpb.User{Id: &userpb.UserId{OpaqueId: owner}},
		RootInfo:  &provider.ResourceInfo{Path: path},
	}
}

// spacesBackend has the home of alice, a project of alice, and a project of
// bob that alice can read but not change.
func spacesBackend() *clitest.Backend {
	b := clitest.New("alice")
	b.AddSpace(testSpace("eoshome-a$alice", "alice", "personal", "alice", "/eos/user/a/alice"))
	b.AddSpace(testSpace("eosproject-c$cernbox", "cernbox", "project", "alice", "/eos/project/c/cernbox"))
	b.AddSpace(testSpace("eosproject-a$atlas", "atlas", "project", "bob", "/eos/project/a/atlas"))
	return b
}

func TestSpaceUserPlane(t *testing.T) {
	b := spacesBackend()
	out := mustRun(t, b, "space", "list", "--type", "project")
	if !strings.Contains(out, "cernbox") || !strings.Contains(out, "atlas") || strings.Contains(out, "personal") {
		t.Errorf("space list --type project = %q", out)
	}
	// The user plane prints the CS3 message.
	var listed []map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, b, "space", "list", "-o", "json")), &listed); err != nil || len(listed) != 3 || listed[0]["spaceType"] != "personal" {
		t.Errorf("space list -o json = %v, %v", listed, err)
	}
	if out := mustRun(t, b, "space", "info", "/eos/project/c/cernbox/docs/a.txt"); !strings.Contains(out, "eosproject-c$cernbox") {
		t.Errorf("space info by path = %q", out)
	}
	mustRun(t, b, "space", "update", "cernbox", "--description", "the CERNBox project")
	if out := mustRun(t, b, "space", "info", "eosproject-c$cernbox"); !strings.Contains(out, "the CERNBox project") {
		t.Errorf("space info after update = %q", out)
	}
}

// Without admin rights, a space that the caller may not change stays not
// found, and nothing tells that it exists.
func TestSpaceUpdateOfAnotherOwner(t *testing.T) {
	_, err := runFake(t, spacesBackend(), "--no-elevate", "space", "update", "atlas", "--description", "x")
	if !cli.IsNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestSpaceUpdateNeedsAField(t *testing.T) {
	started := false
	root := rootCommand(&cli.Runtime{}, &started)
	root.SetArgs([]string{"space", "update", "cernbox"})
	if err := root.Execute(); err == nil || started {
		t.Fatalf("err = %v, started = %v: want a usage error", err, started)
	}
}

type fakeSpacesAdmin struct {
	adminpb.UnimplementedAdminAPIServer
	adminpb.UnimplementedSpacesAdminServer

	mu     sync.Mutex
	spaces map[string]*adminpb.AdminSpace
	calls  map[string]proto.Message
}

func (f *fakeSpacesAdmin) record(method string, req proto.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method] = req
}

func (f *fakeSpacesAdmin) find(ref *adminpb.SpaceRef) (*adminpb.AdminSpace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.spaces {
		if m.Space.Name == ref.GetName() || m.Space.Id.OpaqueId == ref.GetId().GetOpaqueId() {
			return m, nil
		}
	}
	return nil, status.Error(codes.NotFound, "space not found")
}

func (f *fakeSpacesAdmin) CheckAdmin(context.Context, *adminpb.CheckAdminRequest) (*adminpb.CheckAdminResponse, error) {
	return &adminpb.CheckAdminResponse{Admin: true}, nil
}

func (f *fakeSpacesAdmin) RequestAdmin(context.Context, *adminpb.RequestAdminRequest) (*adminpb.RequestAdminResponse, error) {
	return &adminpb.RequestAdminResponse{Token: "admin-token"}, nil
}

func (f *fakeSpacesAdmin) ListSpaces(req *adminpb.ListSpacesRequest, st adminpb.SpacesAdmin_ListSpacesServer) error {
	f.record("ListSpaces", req)
	for _, m := range f.spaces {
		if err := st.Send(m); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSpacesAdmin) GetSpace(_ context.Context, req *adminpb.GetSpaceRequest) (*adminpb.AdminSpace, error) {
	f.record("GetSpace", req)
	return f.find(req.Ref)
}

func (f *fakeSpacesAdmin) UpdateSpace(_ context.Context, req *adminpb.UpdateSpaceRequest) (*adminpb.AdminSpace, error) {
	f.record("UpdateSpace", req)
	m, err := f.find(req.Ref)
	if err != nil {
		return nil, err
	}
	for _, field := range req.Fields {
		if d, ok := field.GetField().(*provider.UpdateStorageSpaceRequest_UpdateField_Description); ok {
			m.Space.Description = d.Description
		}
	}
	return m, nil
}

func (f *fakeSpacesAdmin) SetSpaceStatus(_ context.Context, req *adminpb.SetSpaceStatusRequest) (*adminpb.AdminSpace, error) {
	f.record("SetSpaceStatus", req)
	m, err := f.find(req.Ref)
	if err != nil {
		return nil, err
	}
	m.Status = req.Status
	return m, nil
}

func (f *fakeSpacesAdmin) SetSpaceOwner(_ context.Context, req *adminpb.SetSpaceOwnerRequest) (*adminpb.AdminSpace, error) {
	f.record("SetSpaceOwner", req)
	m, err := f.find(req.Ref)
	if err != nil {
		return nil, err
	}
	m.Space.Owner = &userpb.User{Id: req.Owner}
	return m, nil
}

func (f *fakeSpacesAdmin) RegisterSpace(_ context.Context, req *adminpb.RegisterSpaceRequest) (*adminpb.AdminSpace, error) {
	f.record("RegisterSpace", req)
	return &adminpb.AdminSpace{Space: req.Space, Status: cmp.Or(req.Status, "active")}, nil
}

func (f *fakeSpacesAdmin) CheckSpaces(req *adminpb.CheckSpacesRequest, st adminpb.SpacesAdmin_CheckSpacesServer) error {
	f.record("CheckSpaces", req)
	return st.Send(&adminpb.SpaceCheck{Space: f.spaces["atlas"], Findings: []*adminpb.SpaceFinding{{Kind: "owner_missing", Detail: "bob"}}})
}

// adminArgs serves a fake Admin API with SpacesAdmin, and returns the
// global flags that point the CLI to it.
func adminArgs(t *testing.T) (*fakeSpacesAdmin, []string) {
	t.Helper()
	f := &fakeSpacesAdmin{
		spaces: map[string]*adminpb.AdminSpace{
			"atlas": {Space: testSpace("eosproject-a$atlas", "atlas", "project", "bob", "/eos/project/a/atlas"), Status: "active",
				Extra: map[string]string{"created_at": "2020-01-01T00:00:00Z"}},
			"cms": {Space: testSpace("eosproject-c$cms", "cms", "project", "carol", "/eos/project/c/cms"), Status: "archived"},
		},
		calls: map[string]proto.Message{},
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(s, f)
	adminpb.RegisterSpacesAdminServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return f, []string{"--admin-host", lis.Addr().String(), "--insecure", "--token", "login-token", "--yes"}
}

func TestSpaceOperatorOptions(t *testing.T) {
	f, flags := adminArgs(t)
	out := mustRun(t, spacesBackend(), append(flags, "space", "list", "--owner", "bob", "--include-archived")...)
	if !strings.Contains(out, "archived") || !strings.Contains(out, "cms") {
		t.Errorf("space list --owner = %q, want the catalogue with a status column", out)
	}
	req := f.calls["ListSpaces"].(*adminpb.ListSpacesRequest)
	if fs := req.GetFilters(); len(fs) != 1 || fs[0].GetType() != provider.ListStorageSpacesRequest_Filter_TYPE_OWNER || fs[0].GetOwner().GetOpaqueId() != "bob" || !req.IncludeArchived {
		t.Errorf("ListSpaces request = %v", req)
	}
	// The catalogue record is the AdminSpace.
	var m map[string]any
	if err := json.Unmarshal([]byte(mustRun(t, spacesBackend(), append(flags, "space", "info", "atlas", "--catalogue", "-o", "json")...)), &m); err != nil || m["status"] != "active" {
		t.Errorf("space info --catalogue = %v, %v", m, err)
	}
}

// The owner fallback: an admin finds with GetSpace a space that the user
// plane does not show (cms), or does not let the caller change (atlas), and
// changes it with UpdateSpace.
func TestSpaceOwnerFallback(t *testing.T) {
	f, flags := adminArgs(t)
	if out := mustRun(t, spacesBackend(), append(flags, "space", "info", "cms")...); !strings.Contains(out, "archived") {
		t.Errorf("space info cms = %q, want the catalogue record", out)
	}
	mustRun(t, spacesBackend(), append(flags, "space", "update", "atlas", "--description", "")...)
	req := f.calls["UpdateSpace"].(*adminpb.UpdateSpaceRequest)
	if fs := req.GetFields(); req.GetRef().GetId().GetOpaqueId() != "eosproject-a$atlas" || len(fs) != 1 || fs[0].GetField() == nil || fs[0].GetDescription() != "" || !req.Fallback {
		t.Errorf("UpdateSpace request = %v", req)
	}
	// A space that the catalogue does not have either: the original answer.
	if _, err := runFake(t, spacesBackend(), append(flags, "space", "info", "nope")...); !cli.IsNotFound(err) {
		t.Errorf("space info nope: err = %v, want not found", err)
	}
}

func TestSpaceOperatorCommands(t *testing.T) {
	f, flags := adminArgs(t)
	if out := mustRun(t, spacesBackend(), append(flags, "space", "set-status", "atlas", "archiving")...); !strings.Contains(out, "archiving") {
		t.Errorf("space set-status = %q", out)
	}
	if req := f.calls["SetSpaceStatus"].(*adminpb.SetSpaceStatusRequest); req.GetRef().GetName() != "atlas" || req.Status != "archiving" {
		t.Errorf("SetSpaceStatus request = %v", req)
	}
	if _, err := runFake(t, spacesBackend(), append(flags, "space", "set-status", "atlas", "gone")...); err == nil {
		t.Error("an unknown status: no error")
	}
	if out := mustRun(t, spacesBackend(), append(flags, "space", "check", "--owner", "bob")...); !strings.Contains(out, "owner_missing") {
		t.Errorf("space check = %q", out)
	}
	if req := f.calls["CheckSpaces"].(*adminpb.CheckSpacesRequest); req.GetOwner().GetOpaqueId() != "bob" {
		t.Errorf("CheckSpaces request = %v", req)
	}
	if out := mustRun(t, spacesBackend(), append(flags, "space", "set-owner", "atlas", "carol")...); !strings.Contains(out, "carol") {
		t.Errorf("space set-owner = %q", out)
	}
	if req := f.calls["SetSpaceOwner"].(*adminpb.SetSpaceOwnerRequest); req.GetOwner().GetOpaqueId() != "carol" {
		t.Errorf("SetSpaceOwner request = %v", req)
	}
}

// register sends the space in the form that AdminSpace has: the roles are
// the groups of reader, writer and admin, with their permissions.
func TestSpaceRegister(t *testing.T) {
	f, flags := adminArgs(t)
	mustRun(t, spacesBackend(), append(flags, "space", "register", "lhcb", "--owner", "lhcbsvc", "--path", "/eos/project/l/lhcb",
		"--storage-id", "eosproject-l", "--readers", "lhcb-readers", "--admins", "lhcb-admins", "--status", "creating")...)
	req := f.calls["RegisterSpace"].(*adminpb.RegisterSpaceRequest)
	s := req.GetSpace()
	if s.GetName() != "lhcb" || s.GetOwner().GetId().GetOpaqueId() != "lhcbsvc" || s.GetRootInfo().GetPath() != "/eos/project/l/lhcb" ||
		s.GetRoot().GetStorageId() != "eosproject-l" || s.GetId() != nil || req.Status != "creating" {
		t.Fatalf("RegisterSpace request = %v", req)
	}
	var roles []string
	for _, r := range s.GetRoles() {
		roles = append(roles, r.GetRoleName()+"="+r.GetRecipients()[0].GetGroupId().GetOpaqueId())
		if r.GetPermissionSet() == nil {
			t.Errorf("role %s has no permissions", r.GetRoleName())
		}
	}
	if want := []string{"reader=lhcb-readers", "admin=lhcb-admins"}; !slices.Equal(roles, want) {
		t.Errorf("roles = %v, want %v", roles, want)
	}
}
