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
	"encoding/json"
	"fmt"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	var out struct {
		Routes []storage.Route `json:"routes"`
	}
	if err := invokeRegistry(ctx, eps, "routes", nil, &out); err != nil {
		return nil, err
	}
	res := &adminpb.ListRoutesResponse{}
	for _, r := range out.Routes {
		res.Routes = append(res.Routes, routeInfo(r))
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
	var out struct {
		Path   string          `json:"path"`
		Mounts []storage.Mount `json:"mounts"`
	}
	if err := invokeRegistry(ctx, eps, "resolve", map[string]string{"path": req.Path}, &out); err != nil {
		return nil, err
	}
	reg, err := a.s.registryHandle()
	if err != nil {
		return nil, err
	}
	res := &adminpb.ResolvePathResponse{Path: out.Path}
	for _, m := range out.Mounts {
		p := &adminpb.ResolvedProvider{Provider: &registrypb.ProviderInfo{
			ProviderId:   m.MountID,
			ProviderPath: m.MountPath,
			Address:      m.Address,
		}}
		// A mount with no live instance keeps no nodes: its address still
		// shows where the registry sends the calls.
		if peps, err := client.ResolveMetadata(reg, storageProviderService, map[string]string{registry.MetaMountID: m.MountID}); err == nil {
			for _, ep := range peps {
				p.ProviderNodes = append(p.ProviderNodes, ep.Node)
			}
		}
		res.Providers = append(res.Providers, p)
	}
	return res, nil
}

func (a *storageAdmin) AddRoute(ctx context.Context, req *adminpb.AddRouteRequest) (*adminpb.AddRouteResponse, error) {
	in := req.GetRoute()
	if in.GetProviderPath() == "" || in.GetProviderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: provider_path and provider_id are required")
	}
	// The routing table keeps only the mount id, so an address would be
	// dropped without a word.
	if in.GetAddress() != "" {
		return nil, status.Error(codes.InvalidArgument, "admin: a route has no address: the rules of the storage registry give it")
	}
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	var out storage.Route
	if err := invokeRegistry(ctx, eps, "route_add", map[string]string{"path": in.ProviderPath, "mount_id": in.ProviderId}, &out); err != nil {
		return nil, err
	}
	stale := reloadRoutes(ctx, eps)
	auditRoute(ctx, "add_route", out.Path, stale, map[string]string{"mount_id": out.MountID})
	return &adminpb.AddRouteResponse{Route: routeInfo(out), Stale: stale}, nil
}

func (a *storageAdmin) RemoveRoute(ctx context.Context, req *adminpb.RemoveRouteRequest) (*adminpb.RemoveRouteResponse, error) {
	if req.Path == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: path is required")
	}
	eps, err := a.s.serviceEndpoints(storageRegistryService)
	if err != nil {
		return nil, err
	}
	var out struct {
		Path string `json:"path"`
	}
	if err := invokeRegistry(ctx, eps, "route_remove", map[string]string{"path": req.Path}, &out); err != nil {
		return nil, err
	}
	res := &adminpb.RemoveRouteResponse{Path: out.Path, Stale: reloadRoutes(ctx, eps)}
	auditRoute(ctx, "remove_route", res.Path, res.Stale, map[string]string{})
	return res, nil
}

func routeInfo(r storage.Route) *registrypb.ProviderInfo {
	return &registrypb.ProviderInfo{ProviderPath: r.Path, ProviderId: r.MountID}
}

// invokeRegistry runs an invocation on one storage registry instance and
// decodes its JSON result into out. The results are the plain JSON of the
// storage package, not protojson, so they are not decoded into a message.
func invokeRegistry(ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string, out any) error {
	res, err := invokeOne(ctx, eps, invocation, args)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(res.ResultJSON), out); err != nil {
		return status.Errorf(codes.Internal, "admin: decoding the result of %s: %v", invocation, err)
	}
	return nil
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
			stale = append(stale, &adminpb.StaleInstance{Node: r.Node, Status: &rpc.Status{Code: rpcCode(r.Code), Message: r.Error}})
		}
	}
	return stale
}

// rpcCode is the CS3 code of a gRPC code. The two enums have the same names
// but not the same numbers. pkg/rgrpc/status maps only errors, and turns most
// codes into CODE_INTERNAL, which would hide an instance that is down
// (Unavailable) or slow (DeadlineExceeded).
func rpcCode(c codes.Code) rpc.Code {
	switch c {
	case codes.OK:
		// A node that fails without a code sends 0, as in nodeStatus.
		return rpc.Code_CODE_UNKNOWN
	case codes.Canceled:
		return rpc.Code_CODE_CANCELLED
	case codes.InvalidArgument:
		return rpc.Code_CODE_INVALID_ARGUMENT
	case codes.DeadlineExceeded:
		return rpc.Code_CODE_DEADLINE_EXCEEDED
	case codes.NotFound:
		return rpc.Code_CODE_NOT_FOUND
	case codes.AlreadyExists:
		return rpc.Code_CODE_ALREADY_EXISTS
	case codes.PermissionDenied:
		return rpc.Code_CODE_PERMISSION_DENIED
	case codes.ResourceExhausted:
		return rpc.Code_CODE_RESOURCE_EXHAUSTED
	case codes.FailedPrecondition:
		return rpc.Code_CODE_FAILED_PRECONDITION
	case codes.Aborted:
		return rpc.Code_CODE_ABORTED
	case codes.OutOfRange:
		return rpc.Code_CODE_OUT_OF_RANGE
	case codes.Unimplemented:
		return rpc.Code_CODE_UNIMPLEMENTED
	case codes.Internal:
		return rpc.Code_CODE_INTERNAL
	case codes.Unavailable:
		return rpc.Code_CODE_UNAVAILABLE
	case codes.DataLoss:
		return rpc.Code_CODE_DATA_LOSS
	case codes.Unauthenticated:
		return rpc.Code_CODE_UNAUTHENTICATED
	default:
		return rpc.Code_CODE_UNKNOWN
	}
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
