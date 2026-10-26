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
	"fmt"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	storageRegistryService = "storageregistry"
	storageProviderService = "storageprovider"
)

func (a *storageAdmin) ListRoutes(ctx context.Context, _ *adminpb.ListRoutesRequest) (*adminpb.ListRoutesResponse, error) {
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	res := &adminpb.ListRoutesResponse{}
	if err := invokeRegistry(ctx, eps, "routes", nil, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (a *storageAdmin) ResolvePath(ctx context.Context, req *adminpb.ResolvePathRequest) (*adminpb.ResolvePathResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: path is required")
	}
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	res := &adminpb.ResolvePathResponse{}
	if err := invokeRegistry(ctx, eps, "resolve", map[string]string{"path": req.Path}, res); err != nil {
		return nil, err
	}
	reg, err := a.s.registryHandle()
	if err != nil {
		return nil, err
	}
	for _, m := range res.Mounts {
		// A mount with no live instance keeps no nodes: its address still
		// shows where the registry sends the calls.
		peps, err := client.ResolveMetadata(reg, storageProviderService, map[string]string{registry.MetaMountID: m.MountId})
		if err != nil {
			continue
		}
		for _, ep := range peps {
			m.ProviderNodes = append(m.ProviderNodes, ep.Node)
		}
	}
	return res, nil
}

func (a *storageAdmin) AddRoute(ctx context.Context, req *adminpb.AddRouteRequest) (*adminpb.AddRouteResponse, error) {
	if req.Path == "" || req.MountId == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: path and mount_id are required")
	}
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	route := &adminpb.Route{}
	if err := invokeRegistry(ctx, eps, "route_add", map[string]string{"path": req.Path, "mount_id": req.MountId}, route); err != nil {
		return nil, err
	}
	stale := reloadRoutes(ctx, eps)
	auditRoute(ctx, "add_route", route.Path, stale, map[string]string{"mount_id": route.MountId})
	return &adminpb.AddRouteResponse{Route: route, Stale: stale}, nil
}

func (a *storageAdmin) RemoveRoute(ctx context.Context, req *adminpb.RemoveRouteRequest) (*adminpb.RemoveRouteResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: path is required")
	}
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	res := &adminpb.RemoveRouteResponse{}
	if err := invokeRegistry(ctx, eps, "route_remove", map[string]string{"path": req.Path}, res); err != nil {
		return nil, err
	}
	res.Stale = reloadRoutes(ctx, eps)
	auditRoute(ctx, "remove_route", res.Path, res.Stale, map[string]string{})
	return res, nil
}

// invokeRegistry runs an invocation on one storage registry instance and
// decodes its result into out.
func invokeRegistry(ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string, out proto.Message) error {
	res, _ := invokeOne(ctx, eps, invocation, args)
	if err := nodeStatus(invocation, errtypeCode(res.Code, res.Error), res.Error); err != nil {
		return err
	}
	return decode(res.ResultJSON, out)
}

// reloadRoutes runs route_reload on every storage registry instance and
// returns the instances that failed. The change is written already, thus a
// failure is not an error of the RPC: the CLI must tell the operator which
// instances still serve the old table.
func reloadRoutes(ctx context.Context, eps []client.Endpoint) []*adminpb.StaleInstance {
	// A caller that goes away after the write must not leave every instance
	// stale. PerNodeTimeout still bounds each reload.
	rs, _ := invokeAll(context.WithoutCancel(ctx), eps, "route_reload", nil)
	var stale []*adminpb.StaleInstance
	for _, r := range rs {
		if r.Error != "" {
			stale = append(stale, &adminpb.StaleInstance{Node: r.Node, Error: r.Error, Code: int32(errtypeCode(r.Code, r.Error))})
		}
	}
	return stale
}

// auditRoute records the stale instances, which the audit of the storage
// registry cannot show.
func auditRoute(ctx context.Context, action, path string, stale []*adminpb.StaleInstance, fields map[string]string) {
	ev := admin.AuditEvent{Action: action, Actor: actorName(ctx), Target: path, Fields: fields}
	if len(stale) > 0 {
		nodes := make([]string, len(stale))
		for i, st := range stale {
			nodes[i] = st.Node
		}
		fields["stale"] = strings.Join(nodes, ",")
		ev.Err = fmt.Errorf("%d storage registry instances did not reload the routes", len(stale))
	}
	admin.Audit(ctx, ev)
}

// errtypeCode is the gRPC code of an invocation error. The control channel
// sends an errtypes error as Unknown, because errtypes has no gRPC code, so
// the code comes from the prefix of the message.
func errtypeCode(code codes.Code, msg string) codes.Code {
	if code != codes.OK && code != codes.Unknown {
		return code
	}
	for _, e := range []struct {
		prefix string
		code   codes.Code
	}{
		{errtypes.NotFound("").Error(), codes.NotFound},
		{errtypes.BadRequest("").Error(), codes.InvalidArgument},
		{errtypes.NotSupported("").Error(), codes.Unimplemented},
	} {
		if strings.HasPrefix(msg, e.prefix) {
			return e.code
		}
	}
	return code
}
