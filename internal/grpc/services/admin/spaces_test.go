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
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeCatalogue is a spacesregistry with two instances over one store, as in
// production. It records each call with the instance that ran it.
type fakeCatalogue struct {
	mu     sync.Mutex
	spaces []*adminpb.AdminSpace
	calls  []fakeCall
}

type fakeCall struct {
	node, invocation string
	args             map[string]any
}

func (f *fakeCatalogue) record(node, invocation string, args map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{node: node, invocation: invocation, args: args})
}

func (f *fakeCatalogue) callsOf(invocation string) []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, c := range f.calls {
		if c.invocation == invocation {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeCatalogue) find(args map[string]any) (*adminpb.AdminSpace, error) {
	for _, m := range f.spaces {
		if args["id"] == m.Space.Id.OpaqueId || args["name"] == m.Space.Name {
			return m, nil
		}
	}
	return nil, errtypes.NotFound("project")
}

// fakeInvokable lets the fake see the raw arguments, as the real typed
// handlers would get them.
type fakeInvokable struct {
	node string
	f    *fakeCatalogue
}

var fakeShared = []string{"list", "get", "update", "set_status", "set_owner", "create", "delete", "check"}

func (i fakeInvokable) Invocations() []invoke.InvocationSpec {
	var out []invoke.InvocationSpec
	for _, n := range append(slices.Clone(fakeShared), "flush_cache") {
		out = append(out, invoke.InvocationSpec{Name: n, Kind: invoke.KindReadonly, Streaming: n == "list" || n == "check"})
	}
	return out
}

func (i fakeInvokable) Invoke(_ context.Context, name string, args map[string]any) (invoke.Result, error) {
	i.f.record(i.node, name, args)
	switch name {
	case "flush_cache":
		return invoke.Result{"flushed": true}, nil
	case "create":
		return invoke.ProtoResult(&adminpb.AdminSpace{Space: &provider.StorageSpace{Name: args["name"].(string)}, Status: "active"})
	}
	m, err := i.f.find(args)
	if err != nil {
		return nil, err
	}
	if name == "set_status" {
		m = proto.CloneOf(m)
		m.Status = args["status"].(string)
	}
	return invoke.ProtoResult(m)
}

func (i fakeInvokable) InvokeStream(_ context.Context, name string, args map[string]any, emit invoke.StreamEmit) error {
	i.f.record(i.node, name, args)
	for _, m := range i.f.spaces {
		var r invoke.Result
		var err error
		if name == "check" {
			r, err = invoke.ProtoResult(&adminpb.SpaceCheck{Space: m, Findings: []*adminpb.SpaceFinding{{Kind: "no_members"}}})
		} else {
			r, err = invoke.ProtoResult(m)
		}
		if err != nil {
			return err
		}
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func project(id, name, path, st string) *adminpb.AdminSpace {
	return &adminpb.AdminSpace{
		Space:  &provider.StorageSpace{Id: &provider.StorageSpaceId{OpaqueId: id}, Name: name, RootInfo: &provider.ResourceInfo{Path: path}},
		Status: st,
	}
}

// spacesClient serves SpacesAdmin over a fake catalogue with two instances.
func spacesClient(t *testing.T) (adminpb.SpacesAdminClient, *fakeCatalogue) {
	t.Helper()
	f := &fakeCatalogue{spaces: []*adminpb.AdminSpace{
		project("eosproject-c$cernbox", "cernbox", "/eos/project/c/cernbox", "active"),
		project("eosproject-c$cernbox-old", "cernbox", "/eos/project/c/cernbox", "archived"),
		project("eosproject-c$cernbox-docs", "cernbox-docs", "/eos/project/c/cernbox/docs", "active"),
	}}

	ctrlLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctrl := grpc.NewServer()
	controlpb.RegisterControlServer(ctrl, control.NewServer())
	go func() { _ = ctrl.Serve(ctrlLn) }()
	t.Cleanup(ctrl.Stop)

	addr := ctrlLn.Addr().String()
	var nodes []registry.Node
	for _, port := range []string{"9001", "9002"} {
		id := "127.0.0.1:" + port + "/" + spacesRegistry
		invoke.RegisterInstance(id, spacesRegistry, nil, fakeInvokable{node: id, f: f}, nil)
		nodes = append(nodes, registry.NewNode(id, "127.0.0.1:"+port, map[string]string{
			registry.MetaState:           registry.StateReady,
			registry.MetaControl:         addr,
			invoke.MetaSharedInvocations: strings.Join(fakeShared, ","),
		}))
	}
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	// The first registry set in the process stays, so add to that one.
	service.SetGlobalRegistry(reg)
	if err := service.GlobalRegistry().Add(registry.NewService(spacesRegistry, nodes)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.GlobalRegistry().Remove(registry.NewService(spacesRegistry, nil)) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	(&svc{}).register(srv)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return adminpb.NewSpacesAdminClient(conn), f
}

func byName(n string) *adminpb.SpaceRef {
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: n}}
}

func byPath(p string) *adminpb.SpaceRef {
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Path{Path: p}}
}

func drain[M any](t *testing.T, recv func() (M, error)) ([]M, error) {
	t.Helper()
	var out []M
	for {
		m, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
}

// The CLI owner fallback reads NotFound as "the space does not exist", so an
// errtypes.NotFound of the catalogue must arrive as NotFound.
func TestGetSpaceNotFound(t *testing.T) {
	c, _ := spacesClient(t)
	ctx := context.Background()
	if m, err := c.GetSpace(ctx, &adminpb.GetSpaceRequest{Ref: byName("cernbox")}); err != nil || m.Space.Id.OpaqueId != "eosproject-c$cernbox" {
		t.Fatalf("GetSpace: %v, %v", m, err)
	}
	for _, ref := range []*adminpb.SpaceRef{byName("nope"), byPath("/eos/user/a/alice")} {
		if _, err := c.GetSpace(ctx, &adminpb.GetSpaceRequest{Ref: ref}); status.Code(err) != codes.NotFound {
			t.Errorf("GetSpace(%v): got %v, want NotFound", ref, err)
		}
	}
	if _, err := c.GetSpace(ctx, &adminpb.GetSpaceRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetSpace without a ref: got %v, want InvalidArgument", err)
	}
}

// A path selects the space with the deepest root that holds it, and a space
// that is not archived before an archived one.
func TestSpaceRefByPath(t *testing.T) {
	c, _ := spacesClient(t)
	for p, want := range map[string]string{
		"/eos/project/c/cernbox":             "eosproject-c$cernbox",
		"/eos/project/c/cernbox/a/b.txt":     "eosproject-c$cernbox",
		"/eos/project/c/cernbox/docs/x":      "eosproject-c$cernbox-docs",
		"/eos/project/c/cernbox-other/x.txt": "",
	} {
		m, err := c.GetSpace(context.Background(), &adminpb.GetSpaceRequest{Ref: byPath(p)})
		switch {
		case want == "" && status.Code(err) != codes.NotFound:
			t.Errorf("%s: got %v, want NotFound", p, err)
		case want != "" && (err != nil || m.Space.Id.OpaqueId != want):
			t.Errorf("%s: got %v %v, want %s", p, m.GetSpace().GetId(), err, want)
		}
	}
}

// A change runs on one instance, and then every instance drops its cache.
func TestMutationFlushesEveryInstance(t *testing.T) {
	c, f := spacesClient(t)
	m, err := c.SetSpaceStatus(context.Background(), &adminpb.SetSpaceStatusRequest{Ref: byPath("/eos/project/c/cernbox/docs"), Status: "archiving"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "archiving" {
		t.Errorf("status: %q", m.Status)
	}
	set := f.callsOf("set_status")
	if len(set) != 1 || set[0].args["id"] != "eosproject-c$cernbox-docs" {
		t.Fatalf("set_status calls: %+v", set)
	}
	var flushed []string
	for _, c := range f.callsOf("flush_cache") {
		flushed = append(flushed, c.node)
	}
	slices.Sort(flushed)
	if want := []string{"127.0.0.1:9001/spacesregistry", "127.0.0.1:9002/spacesregistry"}; !slices.Equal(flushed, want) {
		t.Fatalf("flush_cache ran on %v, want %v", flushed, want)
	}
}

func TestFailedMutationDoesNotFlush(t *testing.T) {
	c, f := spacesClient(t)
	_, err := c.SetSpaceOwner(context.Background(), &adminpb.SetSpaceOwnerRequest{Ref: byName("nope"), Owner: "bob"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	if n := len(f.callsOf("flush_cache")); n != 0 {
		t.Fatalf("flush_cache ran %d times", n)
	}
}

// An empty value clears a field, so it is sent; a field that is not set is not.
func TestUpdateSpaceSendsTheSetFields(t *testing.T) {
	c, f := spacesClient(t)
	ctx := context.Background()
	if _, err := c.UpdateSpace(ctx, &adminpb.UpdateSpaceRequest{Ref: byName("cernbox")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an update without fields: got %v, want InvalidArgument", err)
	}
	if _, err := c.UpdateSpace(ctx, &adminpb.UpdateSpaceRequest{Ref: byName("cernbox"), Description: new(""), Fallback: true}); err != nil {
		t.Fatal(err)
	}
	up := f.callsOf("update")
	if len(up) != 1 {
		t.Fatalf("update calls: %+v", up)
	}
	if d, ok := up[0].args["description"]; !ok || d != "" {
		t.Errorf("description: %v", up[0].args)
	}
	if _, ok := up[0].args["readme"]; ok {
		t.Errorf("readme is sent: %v", up[0].args)
	}
	if up[0].args["fallback"] != "true" {
		t.Errorf("fallback: %v", up[0].args)
	}
}

func TestListSpaces(t *testing.T) {
	c, f := spacesClient(t)
	ctx := context.Background()
	after := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	st, err := c.ListSpaces(ctx, &adminpb.ListSpacesRequest{Owner: "alice", CreatedAfter: timestamppb.New(after)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := drain(t, st.Recv)
	if err != nil || len(got) != 3 {
		t.Fatalf("ListSpaces: %d spaces, %v", len(got), err)
	}
	// list is shared: the two instances read one store, so one runs it.
	list := f.callsOf("list")
	if len(list) != 1 {
		t.Fatalf("list ran %d times", len(list))
	}
	if a := list[0].args; a["owner"] != "alice" || a["created_after"] != "2026-01-02T03:04:05Z" || a["member"] != nil {
		t.Errorf("list args: %v", a)
	}

	// No space has a profile yet.
	st, err = c.ListSpaces(ctx, &adminpb.ListSpacesRequest{Profile: "hpc"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := drain(t, st.Recv); err != nil || len(got) != 0 {
		t.Fatalf("ListSpaces with a profile: %v, %v", got, err)
	}
}

func TestCheckSpaces(t *testing.T) {
	c, _ := spacesClient(t)
	st, err := c.CheckSpaces(context.Background(), &adminpb.CheckSpacesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := drain(t, st.Recv)
	if err != nil || len(got) != 3 {
		t.Fatalf("CheckSpaces: %v, %v", got, err)
	}
	if got[0].Space.Space.Name != "cernbox" || got[0].Findings[0].Kind != "no_members" {
		t.Fatalf("unexpected check %v", got[0])
	}
}
