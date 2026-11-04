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

package storageregistry

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/storage"
)

// fakeRegistry keeps the routing table in a map, and applies it to live only
// on Reload, as the dynamic driver does.
type fakeRegistry struct {
	storage.Registry
	table, live map[string]string
}

func (f *fakeRegistry) ListRoutes(context.Context) ([]storage.Route, error) {
	out := []storage.Route{}
	for _, p := range slices.Sorted(maps.Keys(f.table)) {
		out = append(out, storage.Route{Path: p, MountID: f.table[p]})
	}
	return out, nil
}

func (f *fakeRegistry) ResolvePath(_ context.Context, p string) ([]storage.Mount, error) {
	m, ok := f.live[p]
	if !ok {
		return nil, errtypes.NotFound(p)
	}
	return []storage.Mount{{MountID: m, MountPath: p, Address: "host:" + m}}, nil
}

func (f *fakeRegistry) AddRoute(_ context.Context, p, mountID string) error {
	if mountID == "bad" {
		return errtypes.BadRequest("unknown mount")
	}
	f.table[p] = mountID
	return nil
}

func (f *fakeRegistry) RemoveRoute(_ context.Context, p string) error {
	if _, ok := f.table[p]; !ok {
		return errtypes.NotFound(p)
	}
	delete(f.table, p)
	return nil
}

func (f *fakeRegistry) Reload(context.Context) error {
	f.live = maps.Clone(f.table)
	return nil
}

func TestRouteInvocations(t *testing.T) {
	reg := &fakeRegistry{table: map[string]string{"/eos/user/a": "eoshome-a"}, live: map[string]string{"/eos/user/a": "eoshome-a"}}
	s := &service{driver: "dynamic", reg: reg}
	s.registerInvocations()
	ctx := context.Background()

	res, err := s.Invoke(ctx, "route_add", map[string]any{"path": "/eos/user/b", "mount_id": "eoshome-b"})
	if err != nil || res["path"] != "/eos/user/b" || res["mount_id"] != "eoshome-b" {
		t.Fatalf("route_add: %v %v", res, err)
	}
	if _, err := s.Invoke(ctx, "route_add", map[string]any{"path": "/eos/user/c", "mount_id": "bad"}); err == nil {
		t.Error("route_add to an unknown mount must fail")
	}
	if _, err := s.Invoke(ctx, "route_add", map[string]any{"path": "/eos/user/c"}); err == nil {
		t.Error("route_add without a mount_id must fail")
	}

	res, err = s.Invoke(ctx, "routes", nil)
	if want := []storage.Route{{Path: "/eos/user/a", MountID: "eoshome-a"}, {Path: "/eos/user/b", MountID: "eoshome-b"}}; err != nil || !slices.Equal(res["routes"].([]storage.Route), want) {
		t.Fatalf("routes: %v %v", res, err)
	}

	if _, err := s.Invoke(ctx, "resolve", map[string]any{"path": "/eos/user/b"}); err == nil {
		t.Error("an added route must not resolve before route_reload")
	}
	if _, err := s.Invoke(ctx, "route_reload", nil); err != nil {
		t.Fatal(err)
	}
	res, err = s.Invoke(ctx, "resolve", map[string]any{"path": "/eos/user/b"})
	if err != nil || res["mounts"].([]storage.Mount)[0].Address != "host:eoshome-b" {
		t.Fatalf("resolve: %v %v", res, err)
	}

	if _, err := s.Invoke(ctx, "route_remove", map[string]any{"path": "/eos/user/a"}); err != nil || len(reg.table) != 1 {
		t.Fatalf("route_remove: %v %v", reg.table, err)
	}
	var nf errtypes.IsNotFound
	if _, err := s.Invoke(ctx, "route_remove", map[string]any{"path": "/eos/user/a"}); !errors.As(err, &nf) {
		t.Errorf("route_remove of a path with no route: want not found, got %v", err)
	}
}

// The table is shared, so a change runs on one instance; each instance has its
// own tree in memory, so a reload must run on all of them.
func TestRouteInvocationScopes(t *testing.T) {
	s := &service{driver: "dynamic", reg: &fakeRegistry{}}
	s.registerInvocations()
	want := map[string]invoke.InvocationScope{"route_add": invoke.ScopeShared, "route_remove": invoke.ScopeShared, "route_reload": "", "resolve": ""}
	for _, spec := range s.Invocations() {
		if scope, ok := want[spec.Name]; ok && spec.Scope != scope {
			t.Errorf("%s: scope %q, want %q", spec.Name, spec.Scope, scope)
		}
	}
}

func TestStaticRegistryHasNoRoutes(t *testing.T) {
	s := &service{driver: "static", reg: struct{ storage.Registry }{}}
	s.registerInvocations()
	args := map[string]any{"path": "/eos/user/a", "mount_id": "eoshome-a"}
	var ns errtypes.IsNotSupported
	for _, name := range []string{"routes", "resolve", "route_add", "route_remove", "route_reload"} {
		if _, err := s.Invoke(context.Background(), name, args); !errors.As(err, &ns) {
			t.Errorf("%s on a static registry: want not supported, got %v", name, err)
		}
	}
}
