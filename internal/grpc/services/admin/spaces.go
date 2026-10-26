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
	"cmp"
	"context"
	"maps"
	"path"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// spacesRegistry is the service that holds the space catalogue.
const spacesRegistry = "spacesregistry"

func (a *spacesAdmin) ListSpaces(req *adminpb.ListSpacesRequest, stream adminpb.SpacesAdmin_ListSpacesServer) error {
	if req.Profile != "" {
		return status.Error(codes.Unimplemented, "admin: the profile filter is not implemented yet: the catalogue does not store profiles")
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return err
	}
	args := map[string]string{"type": req.Type, "owner": req.Owner, "member": req.Member, "status": req.Status}
	maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
	if t := req.CreatedAfter; t != nil {
		args["created_after"] = t.AsTime().Format(time.RFC3339Nano)
	}
	if t := req.CreatedBefore; t != nil {
		args["created_before"] = t.AsTime().Format(time.RFC3339Nano)
	}
	if req.IncludeArchived {
		args["include_archived"] = "true"
	}
	return relayStream(stream.Context(), eps, "list", args, stream.Send)
}

func (a *spacesAdmin) GetSpace(ctx context.Context, req *adminpb.GetSpaceRequest) (*adminpb.AdminSpace, error) {
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	res, err := invokeOne(ctx, eps, "get", args)
	if err != nil {
		return nil, err
	}
	var m adminpb.AdminSpace
	if err := decode(res.ResultJSON, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (a *spacesAdmin) UpdateSpace(ctx context.Context, req *adminpb.UpdateSpaceRequest) (*adminpb.AdminSpace, error) {
	if req.Description == nil && req.Readme == nil && req.Thumbnail == nil {
		return nil, status.Error(codes.InvalidArgument, "admin: update needs a description, a readme or a thumbnail")
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	// An empty value is a change: it clears the field.
	for k, v := range map[string]*string{"description": req.Description, "readme": req.Readme, "thumbnail": req.Thumbnail} {
		if v != nil {
			args[k] = *v
		}
	}
	if req.Fallback {
		args["fallback"] = "true"
	}
	return mutateSpace(ctx, eps, "update", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

func (a *spacesAdmin) SetSpaceStatus(ctx context.Context, req *adminpb.SetSpaceStatusRequest) (*adminpb.AdminSpace, error) {
	if req.Status == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: status is required")
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	args["status"] = req.Status
	return mutateSpace(ctx, eps, "set_status", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

func (a *spacesAdmin) SetSpaceOwner(ctx context.Context, req *adminpb.SetSpaceOwnerRequest) (*adminpb.AdminSpace, error) {
	if req.Owner == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: owner is required")
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	args["owner"] = req.Owner
	return mutateSpace(ctx, eps, "set_owner", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

func (a *spacesAdmin) RegisterSpace(ctx context.Context, req *adminpb.RegisterSpaceRequest) (*adminpb.AdminSpace, error) {
	if req.Name == "" || req.Owner == "" {
		return nil, status.Error(codes.InvalidArgument, "admin: name and owner are required")
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args := map[string]string{
		"name":        req.Name,
		"owner":       req.Owner,
		"path":        req.Path,
		"storage_id":  req.StorageId,
		"space_id":    req.SpaceId,
		"readers":     req.Readers,
		"writers":     req.Writers,
		"admins":      req.Admins,
		"description": req.Description,
		"status":      req.Status,
	}
	maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
	return mutateSpace(ctx, eps, "create", req.Name, args)
}

func (a *spacesAdmin) UnregisterSpace(ctx context.Context, req *adminpb.UnregisterSpaceRequest) (*adminpb.AdminSpace, error) {
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	if req.Hard {
		args["hard"] = "true"
	}
	return mutateSpace(ctx, eps, "delete", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

func (a *spacesAdmin) CheckSpaces(req *adminpb.CheckSpacesRequest, stream adminpb.SpacesAdmin_CheckSpacesServer) error {
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return err
	}
	args := map[string]string{"status": req.Status, "owner": req.Owner}
	maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
	return relayStream(stream.Context(), eps, "check", args, stream.Send)
}

// mutateSpace runs a change of the catalogue on one instance, and then
// flush_cache on every instance, because each one caches the project list.
func mutateSpace(ctx context.Context, eps []client.Endpoint, invocation, target string, args map[string]string) (*adminpb.AdminSpace, error) {
	res, err := invokeOne(ctx, eps, invocation, args)
	fields := maps.Clone(args)
	if err == nil {
		// The change is made, so it is not an error. An instance that keeps
		// its cache shows the old value until the cache expires (cache_ttl).
		if _, ferr := invokeAll(ctx, eps, "flush_cache", nil); ferr != nil {
			fields["flush_cache"] = ferr.Error()
		}
	}
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "spaces." + invocation,
		Actor:   actorName(ctx),
		Target:  "space:" + target,
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
	if err != nil {
		return nil, err
	}
	var m adminpb.AdminSpace
	if err := decode(res.ResultJSON, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// refArgs returns the arguments that select ref in the catalogue. The
// catalogue selects by id or by name only, so a path is resolved here.
func refArgs(ctx context.Context, eps []client.Endpoint, ref *adminpb.SpaceRef) (map[string]string, error) {
	switch {
	case ref.GetId().GetOpaqueId() != "":
		return map[string]string{"id": ref.GetId().GetOpaqueId()}, nil
	case ref.GetName() != "":
		return map[string]string{"name": ref.GetName()}, nil
	case ref.GetPath() != "":
		m, err := spaceAt(ctx, eps, ref.GetPath())
		if err != nil {
			return nil, err
		}
		return map[string]string{"id": m.GetSpace().GetId().GetOpaqueId()}, nil
	}
	return nil, status.Error(codes.InvalidArgument, "admin: a space id, path or name is required")
}

// spaceAt returns the space that holds p: the one with the deepest root, and
// one that is not archived before one that is. The catalogue has no index on
// the path, so this reads the whole list.
func spaceAt(ctx context.Context, eps []client.Endpoint, p string) (*adminpb.AdminSpace, error) {
	p = path.Clean(p)
	var found *adminpb.AdminSpace
	var depth int
	err := relayStream(ctx, eps, "list", map[string]string{"include_archived": "true"}, func(m *adminpb.AdminSpace) error {
		root := m.GetSpace().GetRootInfo().GetPath()
		if root == "" {
			return nil
		}
		root = path.Clean(root)
		if p != root && !strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
			return nil
		}
		if found == nil || len(root) > depth || (len(root) == depth && found.Status == "archived" && m.Status != "archived") {
			found, depth = m, len(root)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "admin: no space holds %s", p)
	}
	return found, nil
}
