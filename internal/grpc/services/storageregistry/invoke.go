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
	"fmt"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/storage/registry/dynamic"
)

// The storage registry is an invoke.Invokable: through the Admin API an
// operator lists and resolves routes and, with the dynamic driver, changes the
// routing table and reloads the in-memory tree every instance built at
// start-up. A change goes to one instance, then route_reload fans out.

func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("providers", "List the storage providers this registry knows, as the registry reports them.").
		Handle(s.invokeProviders)
	s.Set.Add("resolve", "Resolve a path to the storage providers serving it.").
		Arg("path", "the path to resolve").
		Handle(s.invokeResolve)
	s.Set.Add("routes", "List the routing table (dynamic registry).").
		Handle(s.invokeRoutes)
	s.Set.Add("route_add", "Add or update the route of a path to a mount configured in this registry, reloading this instance (dynamic registry). Address one instance, then route_reload the others.").
		Mutating().
		Arg("path", "the path to route").
		Arg("mount_id", "the mount serving it, one of the configured rules").
		Handle(s.invokeRouteAdd)
	s.Set.Add("route_remove", "Remove the route of a path, reloading this instance (dynamic registry). Address one instance, then route_reload the others.").
		Mutating().
		Arg("path", "the routed path").
		Handle(s.invokeRouteRemove)
	s.Set.Add("route_reload", "Rebuild this instance's routing tree from the table (dynamic registry). Safe to fan out.").
		Mutating().
		Handle(s.invokeRouteReload)
}

func (s *service) admin() (dynamic.RouteAdmin, error) {
	ra, ok := s.reg.(dynamic.RouteAdmin)
	if !ok {
		return nil, fmt.Errorf("storageregistry: the %s driver has no routing table to administer", s.conf.Driver)
	}
	return ra, nil
}

func (s *service) invokeProviders(ctx context.Context, _ invoke.Args) (invoke.Result, error) {
	ps, err := s.reg.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]invoke.Result, 0, len(ps))
	for _, p := range ps {
		out = append(out, invoke.Result{"provider_id": p.GetProviderId(), "provider_path": p.GetProviderPath(), "address": p.GetAddress()})
	}
	return invoke.Result{"providers": out, "count": len(out)}, nil
}

func (s *service) invokeResolve(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	path := strings.TrimSpace(a.String("path"))
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path must be absolute, not %q", path)
	}
	ps, err := s.reg.FindProviders(ctx, &provider.Reference{Path: path})
	if err != nil {
		return nil, err
	}
	out := make([]invoke.Result, 0, len(ps))
	for _, p := range ps {
		out = append(out, invoke.Result{"provider_id": p.GetProviderId(), "provider_path": p.GetProviderPath(), "address": p.GetAddress()})
	}
	return invoke.Result{"path": path, "providers": out}, nil
}

func (s *service) invokeRoutes(ctx context.Context, _ invoke.Args) (invoke.Result, error) {
	ra, err := s.admin()
	if err != nil {
		return nil, err
	}
	routes, err := ra.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]invoke.Result, 0, len(routes))
	for _, r := range routes {
		out = append(out, invoke.Result{"path": r.Path, "mount_id": r.MountID})
	}
	return invoke.Result{"routes": out, "count": len(out)}, nil
}

func (s *service) invokeRouteAdd(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	ra, err := s.admin()
	if err != nil {
		return nil, err
	}
	path, mountID := a.String("path"), a.String("mount_id")
	err = ra.AddRoute(ctx, path, mountID)
	audit(ctx, "route_add", path, err, map[string]string{"mount_id": mountID})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"path": path, "mount_id": mountID, "reloaded": true}, nil
}

func (s *service) invokeRouteRemove(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	ra, err := s.admin()
	if err != nil {
		return nil, err
	}
	path := a.String("path")
	err = ra.RemoveRoute(ctx, path)
	audit(ctx, "route_remove", path, err, nil)
	if err != nil {
		return nil, err
	}
	return invoke.Result{"removed": path, "reloaded": true}, nil
}

func (s *service) invokeRouteReload(ctx context.Context, _ invoke.Args) (invoke.Result, error) {
	ra, err := s.admin()
	if err != nil {
		return nil, err
	}
	if err := ra.Reload(ctx); err != nil {
		return nil, err
	}
	return invoke.Result{"reloaded": true}, nil
}

func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "storageregistry." + action,
		Actor:   admin.Actor(ctx),
		Target:  target,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}
