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
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// routeTable is the shared routing table of the fake storage registries.
type routeTable struct {
	mu     sync.Mutex
	routes map[string]string
	writes int
}

// fakeStorageRegistry is one storage registry instance: it resolves with its
// own copy of the table, which changes only on route_reload, as in the
// dynamic driver.
type fakeStorageRegistry struct {
	table    *routeTable
	mu       sync.Mutex
	live     map[string]string
	failLoad bool
}

func (f *fakeStorageRegistry) set() *invoke.Set {
	type pathArgs struct {
		Path string `arg:"path,required"`
	}
	type routeArgs struct {
		Path    string `arg:"path,required"`
		MountID string `arg:"mount_id,required"`
	}
	set := invoke.NewSet()
	set.Add("routes", "").Shared().Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) {
		f.table.mu.Lock()
		defer f.table.mu.Unlock()
		routes := []storage.Route{}
		for _, p := range slices.Sorted(maps.Keys(f.table.routes)) {
			routes = append(routes, storage.Route{Path: p, MountID: f.table.routes[p]})
		}
		return invoke.Result{"routes": routes}, nil
	}))
	set.Add("resolve", "").Handle(invoke.Typed(func(_ context.Context, a pathArgs) (invoke.Result, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !strings.HasPrefix(a.Path, "/") {
			return nil, errtypes.BadRequest("path is not absolute")
		}
		for p, m := range f.live {
			if a.Path == p || strings.HasPrefix(a.Path, p+"/") {
				return invoke.Result{"path": a.Path, "mounts": []storage.Mount{{MountID: m, MountPath: p, Address: "lb-" + m + ":9000"}}}, nil
			}
		}
		return nil, errtypes.NotFound("no route for " + a.Path)
	}))
	set.Add("route_add", "").Mutating().Shared().Handle(invoke.Typed(func(_ context.Context, a routeArgs) (invoke.Result, error) {
		if !strings.HasPrefix(a.MountID, "eoshome-") {
			return nil, errtypes.BadRequest("mount " + a.MountID + " is not in the rules of this registry")
		}
		f.table.mu.Lock()
		defer f.table.mu.Unlock()
		f.table.routes[a.Path] = a.MountID
		f.table.writes++
		return invoke.Result{"path": a.Path, "mount_id": a.MountID}, nil
	}))
	set.Add("route_remove", "").Mutating().Shared().Handle(invoke.Typed(func(_ context.Context, a pathArgs) (invoke.Result, error) {
		f.table.mu.Lock()
		defer f.table.mu.Unlock()
		if _, ok := f.table.routes[a.Path]; !ok {
			return nil, errtypes.NotFound("no route for " + a.Path)
		}
		delete(f.table.routes, a.Path)
		f.table.writes++
		return invoke.Result{"path": a.Path}, nil
	}))
	set.Add("route_reload", "").Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) {
		if f.failLoad {
			return nil, status.Error(codes.Unavailable, "the routing table is not reachable")
		}
		f.table.mu.Lock()
		defer f.table.mu.Unlock()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.live = maps.Clone(f.table.routes)
		return invoke.Result{"reloaded": true}, nil
	}))
	return set
}

func (f *fakeStorageRegistry) resolves(p string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[p]
}

// storageFleet starts three storage registry instances on one control
// channel, the third of which fails to reload, and registers storage
// providers for the mounts eoshome-a (two instances) and eoshome-b.
func storageFleet(t *testing.T) (*storageAdmin, *routeTable, []*fakeStorageRegistry) {
	t.Helper()
	srv := grpc.NewServer()
	controlpb.RegisterControlServer(srv, control.NewServer())
	ctrl := serveGRPC(t, "tcp", "127.0.0.1:0", srv)

	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	// The first registry set in the process stays, so add to that one.
	service.SetGlobalRegistry(reg)
	reg = service.GlobalRegistry()

	table := &routeTable{routes: map[string]string{"/eos/user/a": "eoshome-a"}}
	var regs []*fakeStorageRegistry
	var nodes []registry.Node
	for i, host := range []string{"reg1:9999", "reg2:9999", "reg3:9999"} {
		f := &fakeStorageRegistry{table: table, live: maps.Clone(table.routes), failLoad: i == 2}
		id := host + "/" + storageRegistryService
		invoke.RegisterInstance(id, storageRegistryService, nil, f.set(), nil)
		meta := invoke.Metadata(id)
		meta[registry.MetaState] = registry.StateReady
		meta[registry.MetaControl] = ctrl
		nodes = append(nodes, registry.NewNode(id, host, meta))
		regs = append(regs, f)
	}
	providers := []registry.Node{
		registry.NewNode("sp1:9001/storageprovider", "sp1:9001", map[string]string{registry.MetaMountID: "eoshome-a"}),
		registry.NewNode("sp2:9001/storageprovider", "sp2:9001", map[string]string{registry.MetaMountID: "eoshome-a"}),
		registry.NewNode("sp3:9001/storageprovider", "sp3:9001", map[string]string{registry.MetaMountID: "eoshome-a", registry.MetaState: registry.StateOffline}),
		registry.NewNode("sp4:9001/storageprovider", "sp4:9001", map[string]string{registry.MetaMountID: "eoshome-b"}),
	}
	for name, ns := range map[string][]registry.Node{storageRegistryService: nodes, storageProviderService: providers} {
		if err := reg.Add(registry.NewService(name, ns)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reg.Remove(registry.NewService(name, nil)) })
	}
	return &storageAdmin{s: &svc{}}, table, regs
}

func route(path, mountID string) *registrypb.ProviderInfo {
	return &registrypb.ProviderInfo{ProviderPath: path, ProviderId: mountID}
}

// A route is written once, every registry reloads it, and the one that
// cannot reload is reported as stale.
func TestAddRouteReportsTheStaleRegistries(t *testing.T) {
	a, table, regs := storageFleet(t)
	ctx := context.Background()

	res, err := a.AddRoute(ctx, &adminpb.AddRouteRequest{Route: route("/eos/user/b", "eoshome-b")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Route.GetProviderPath() != "/eos/user/b" || res.Route.GetProviderId() != "eoshome-b" {
		t.Errorf("route = %v", res.Route)
	}
	if table.writes != 1 {
		t.Errorf("the route was written %d times, want once", table.writes)
	}
	if len(res.Stale) != 1 || res.Stale[0].Node != "reg3:9999/storageregistry" || res.Stale[0].GetStatus().GetMessage() == "" {
		t.Fatalf("stale = %v, want only reg3", res.Stale)
	}
	if c := res.Stale[0].GetStatus().GetCode(); c != rpc.Code_CODE_UNAVAILABLE {
		t.Errorf("stale code = %v, want the code of the failed reload", c)
	}
	for i, want := range []string{"eoshome-b", "eoshome-b", ""} {
		if got := regs[i].resolves("/eos/user/b"); got != want {
			t.Errorf("registry %d resolves to %q, want %q", i+1, got, want)
		}
	}

	routes, err := a.ListRoutes(ctx, &adminpb.ListRoutesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range routes.Routes {
		got = append(got, r.ProviderPath+"="+r.ProviderId)
	}
	if want := []string{"/eos/user/a=eoshome-a", "/eos/user/b=eoshome-b"}; !slices.Equal(got, want) {
		t.Errorf("routes = %v, want %v", got, want)
	}

	rm, err := a.RemoveRoute(ctx, &adminpb.RemoveRouteRequest{Path: "/eos/user/a"})
	if err != nil {
		t.Fatal(err)
	}
	if rm.Path != "/eos/user/a" || len(rm.Stale) != 1 || rm.Stale[0].Node != "reg3:9999/storageregistry" {
		t.Errorf("remove = %v, want reg3 stale", rm)
	}
	if got := regs[0].resolves("/eos/user/a"); got != "" {
		t.Errorf("registry 1 still resolves the removed route to %q", got)
	}
}

func TestResolvePathGivesTheProviderNodes(t *testing.T) {
	a, _, _ := storageFleet(t)
	res, err := a.ResolvePath(context.Background(), &adminpb.ResolvePathRequest{Path: "/eos/user/a/alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Providers) != 1 {
		t.Fatalf("providers = %v", res.Providers)
	}
	m := res.Providers[0]
	if p := m.GetProvider(); p.GetProviderId() != "eoshome-a" || p.GetProviderPath() != "/eos/user/a" || p.GetAddress() != "lb-eoshome-a:9000" {
		t.Errorf("provider = %v", m)
	}
	slices.Sort(m.ProviderNodes)
	if want := []string{"sp1:9001/storageprovider", "sp2:9001/storageprovider"}; !slices.Equal(m.ProviderNodes, want) {
		t.Errorf("provider nodes = %v, want the live instances %v", m.ProviderNodes, want)
	}
}

// The errtypes errors of the registry reach the caller with their gRPC code.
func TestRouteErrorCodes(t *testing.T) {
	a, table, _ := storageFleet(t)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		call func() error
		want codes.Code
	}{
		"unknown mount": {func() error {
			_, err := a.AddRoute(ctx, &adminpb.AddRouteRequest{Route: route("/eos/user/c", "nowhere")})
			return err
		}, codes.InvalidArgument},
		"no mount": {func() error {
			_, err := a.AddRoute(ctx, &adminpb.AddRouteRequest{Route: route("/eos/user/c", "")})
			return err
		}, codes.InvalidArgument},
		"no route": {func() error {
			_, err := a.AddRoute(ctx, &adminpb.AddRouteRequest{})
			return err
		}, codes.InvalidArgument},
		"an address": {func() error {
			r := route("/eos/user/c", "eoshome-c")
			r.Address = "lb:9000"
			_, err := a.AddRoute(ctx, &adminpb.AddRouteRequest{Route: r})
			return err
		}, codes.InvalidArgument},
		"remove a missing route": {func() error {
			_, err := a.RemoveRoute(ctx, &adminpb.RemoveRouteRequest{Path: "/eos/user/z"})
			return err
		}, codes.NotFound},
		"resolve an unrouted path": {func() error {
			_, err := a.ResolvePath(ctx, &adminpb.ResolvePathRequest{Path: "/eos/project/x"})
			return err
		}, codes.NotFound},
		"resolve a relative path": {func() error {
			_, err := a.ResolvePath(ctx, &adminpb.ResolvePathRequest{Path: "eos"})
			return err
		}, codes.InvalidArgument},
	} {
		if err := tc.call(); status.Code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
	if table.writes != 0 {
		t.Errorf("a failed call wrote the table %d times", table.writes)
	}
}

// A stale instance keeps the code of its failure, so that the operator can
// tell an instance that is down from one that failed to load the table.
func TestRPCCodeKeepsTheGRPCCode(t *testing.T) {
	for in, want := range map[codes.Code]rpc.Code{
		codes.OK:               rpc.Code_CODE_UNKNOWN,
		codes.Unknown:          rpc.Code_CODE_UNKNOWN,
		codes.InvalidArgument:  rpc.Code_CODE_INVALID_ARGUMENT,
		codes.DeadlineExceeded: rpc.Code_CODE_DEADLINE_EXCEEDED,
		codes.NotFound:         rpc.Code_CODE_NOT_FOUND,
		codes.Unimplemented:    rpc.Code_CODE_UNIMPLEMENTED,
		codes.Internal:         rpc.Code_CODE_INTERNAL,
		codes.Unavailable:      rpc.Code_CODE_UNAVAILABLE,
		codes.DataLoss:         rpc.Code_CODE_DATA_LOSS,
		codes.Unauthenticated:  rpc.Code_CODE_UNAUTHENTICATED,
		codes.Code(99):         rpc.Code_CODE_UNKNOWN,
	} {
		if got := rpcCode(in); got != want {
			t.Errorf("rpcCode(%v) = %v, want %v", in, got, want)
		}
	}
}
