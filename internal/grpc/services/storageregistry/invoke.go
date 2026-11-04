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
	"path"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/storage"
)

type pathArgs struct {
	Path string `arg:"path,required" help:"an absolute path"`
}

type routeArgs struct {
	Path    string `arg:"path,required"     help:"the absolute path to route"`
	MountID string `arg:"mount_id,required" help:"the mount that serves the path, one of the rules of the registry"`
}

// The routing table is shared by all the instances, but each instance has its
// own copy in memory. Thus route_add and route_remove run on one instance and
// route_reload must run on all of them.
func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("routes", "List the routing table.").
		Shared().
		Handle(invoke.Typed(s.routes))
	s.Set.Add("resolve", "Show the mounts that serve a path, as this instance resolves it.").
		Handle(invoke.Typed(s.resolve))
	s.Set.Add("route_add", "Route a path to a mount, or change the mount of a routed path. Run route_reload after it to apply the change.").
		Mutating().
		Shared().
		Handle(invoke.Typed(s.routeAdd))
	s.Set.Add("route_remove", "Remove the route of a path. Run route_reload after it to apply the change.").
		Mutating().
		Shared().
		Handle(invoke.Typed(s.routeRemove))
	s.Set.Add("route_reload", "Load the routing table again into the memory of this instance.").
		Handle(invoke.Typed(s.routeReload))
}

func (s *service) routeAdmin() (storage.RouteAdmin, error) {
	ra, ok := s.reg.(storage.RouteAdmin)
	if !ok {
		return nil, errtypes.NotSupported("the " + s.driver + " storage registry has no routing table")
	}
	return ra, nil
}

func (s *service) routes(ctx context.Context, _ struct{}) (invoke.Result, error) {
	ra, err := s.routeAdmin()
	if err != nil {
		return nil, err
	}
	routes, err := ra.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	return invoke.Result{"routes": routes}, nil
}

func (s *service) resolve(ctx context.Context, a pathArgs) (invoke.Result, error) {
	ra, err := s.routeAdmin()
	if err != nil {
		return nil, err
	}
	mounts, err := ra.ResolvePath(ctx, a.Path)
	if err != nil {
		return nil, err
	}
	return invoke.Result{"path": a.Path, "mounts": mounts}, nil
}

func (s *service) routeAdd(ctx context.Context, a routeArgs) (invoke.Result, error) {
	ra, err := s.routeAdmin()
	if err != nil {
		return nil, err
	}
	err = ra.AddRoute(ctx, a.Path, a.MountID)
	audit(ctx, "route_add", a.Path, err, map[string]string{"mount_id": a.MountID})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"path": path.Clean(a.Path), "mount_id": a.MountID}, nil
}

func (s *service) routeRemove(ctx context.Context, a pathArgs) (invoke.Result, error) {
	ra, err := s.routeAdmin()
	if err != nil {
		return nil, err
	}
	err = ra.RemoveRoute(ctx, a.Path)
	audit(ctx, "route_remove", a.Path, err, nil)
	if err != nil {
		return nil, err
	}
	return invoke.Result{"path": path.Clean(a.Path)}, nil
}

func (s *service) routeReload(ctx context.Context, _ struct{}) (invoke.Result, error) {
	ra, err := s.routeAdmin()
	if err != nil {
		return nil, err
	}
	if err := ra.Reload(ctx); err != nil {
		return nil, err
	}
	return invoke.Result{"reloaded": true}, nil
}

// audit records the route that changed, which the audit of the admin service
// does not show because it has no arguments.
func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	var actor string
	if u, ok := appctx.ContextGetUser(ctx); ok {
		actor = u.GetUsername()
	}
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "storageregistry." + action,
		Actor:   actor,
		Target:  target,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}
