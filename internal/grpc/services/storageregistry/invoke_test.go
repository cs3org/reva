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
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/registry/dynamic"
)

// fakeRegistry is a registry with an administrable routing table in memory.
type fakeRegistry struct {
	storage.Registry
	routes  map[string]string
	reloads int
}

func (f *fakeRegistry) ListProviders(context.Context) ([]*registrypb.ProviderInfo, error) {
	return []*registrypb.ProviderInfo{{ProviderPath: "/eos/user/a", Address: "host:1"}}, nil
}

func (f *fakeRegistry) FindProviders(_ context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	for p, m := range f.routes {
		if ref.GetPath() == p {
			return []*registrypb.ProviderInfo{{ProviderId: m, ProviderPath: p, Address: "host:" + m}}, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeRegistry) ListRoutes(context.Context) ([]dynamic.Route, error) {
	out := []dynamic.Route{}
	for p, m := range f.routes {
		out = append(out, dynamic.Route{Path: p, MountID: m})
	}
	return out, nil
}

func (f *fakeRegistry) AddRoute(_ context.Context, path, mountID string) error {
	if mountID == "bad" {
		return errors.New("unknown mount")
	}
	f.routes[path] = mountID
	f.reloads++
	return nil
}

func (f *fakeRegistry) RemoveRoute(_ context.Context, path string) error {
	if _, ok := f.routes[path]; !ok {
		return errors.New("no route")
	}
	delete(f.routes, path)
	f.reloads++
	return nil
}

func (f *fakeRegistry) Reload(context.Context) error { f.reloads++; return nil }

func TestRouteInvocations(t *testing.T) {
	reg := &fakeRegistry{routes: map[string]string{"/eos/user/a": "eoshome-a"}}
	s := &service{conf: &config{Driver: "dynamic"}, reg: reg}
	s.registerInvocations()
	ctx := context.Background()

	res, err := s.Invoke(ctx, "providers", nil)
	if err != nil || res["count"] != 1 {
		t.Fatalf("providers: %v %v", res, err)
	}
	res, err = s.Invoke(ctx, "resolve", map[string]any{"path": "/eos/user/a"})
	if err != nil || res["providers"].([]invoke.Result)[0]["address"] != "host:eoshome-a" {
		t.Fatalf("resolve: %v %v", res, err)
	}
	if _, err := s.Invoke(ctx, "resolve", map[string]any{"path": "relative"}); err == nil {
		t.Fatal("a relative path must be refused")
	}
	if res, err := s.Invoke(ctx, "route_add", map[string]any{"path": "/eos/user/b", "mount_id": "eoshome-b"}); err != nil || res["reloaded"] != true || reg.routes["/eos/user/b"] != "eoshome-b" {
		t.Fatalf("route_add: %v %v", res, err)
	}
	if _, err := s.Invoke(ctx, "route_add", map[string]any{"path": "/eos/user/c", "mount_id": "bad"}); err == nil {
		t.Fatal("an unknown mount must be reported")
	}
	if res, err := s.Invoke(ctx, "routes", nil); err != nil || res["count"] != 2 {
		t.Fatalf("routes: %v %v", res, err)
	}
	if _, err := s.Invoke(ctx, "route_remove", map[string]any{"path": "/eos/user/b"}); err != nil || len(reg.routes) != 1 {
		t.Fatalf("route_remove: %v", err)
	}
	if _, err := s.Invoke(ctx, "route_reload", nil); err != nil || reg.reloads != 3 {
		t.Fatalf("route_reload: %v, reloads %d", err, reg.reloads)
	}
}

func TestStaticRegistryHasNoRoutes(t *testing.T) {
	s := &service{conf: &config{Driver: "static"}, reg: struct{ storage.Registry }{}}
	s.registerInvocations()
	for _, name := range []string{"routes", "route_reload"} {
		if _, err := s.Invoke(context.Background(), name, nil); err == nil {
			t.Errorf("%s on a static registry must be reported", name)
		}
	}
}
