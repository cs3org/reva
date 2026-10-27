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
	"slices"
	"strings"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// spacesRegistry is the service that holds the space catalogue.
const spacesRegistry = "spacesregistry"

func (a *spacesAdmin) ListSpaces(req *adminpb.ListSpacesRequest, stream adminpb.SpacesAdmin_ListSpacesServer) error {
	if req.Profile != "" {
		return status.Error(codes.Unimplemented, "admin: the profile filter is not implemented yet: the catalogue does not store profiles")
	}
	args, keep, err := listArgs(req.Filters)
	if err != nil {
		return err
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return err
	}
	if req.Status != "" {
		args["status"] = req.Status
	}
	if t := req.CreatedAfter; t != nil {
		args["created_after"] = timestamp(t).Format(time.RFC3339Nano)
	}
	if t := req.CreatedBefore; t != nil {
		args["created_before"] = timestamp(t).Format(time.RFC3339Nano)
	}
	if req.IncludeArchived {
		args["include_archived"] = "true"
	}
	return relayStream(stream.Context(), eps, "list", args, func(m *adminpb.AdminSpace) error {
		if !keep(m.GetSpace()) {
			return nil
		}
		return stream.Send(m)
	})
}

// listArgs maps the CS3 filters onto the arguments of the list invocation.
// The invocation selects by one type, one owner and one member, so the
// filters by id and by path, which it does not take, are kept here.
func listArgs(filters []*provider.ListStorageSpacesRequest_Filter) (map[string]string, func(*provider.StorageSpace) bool, error) {
	args := map[string]string{}
	var ids, paths []string
	one := func(key, value string, t provider.ListStorageSpacesRequest_Filter_Type) error {
		if value == "" {
			return status.Errorf(codes.InvalidArgument, "admin: the filter %s has no value", t)
		}
		if _, ok := args[key]; ok {
			return status.Errorf(codes.InvalidArgument, "admin: the catalogue takes at most one filter %s", t)
		}
		args[key] = value
		return nil
	}
	for _, f := range filters {
		var err error
		switch f.GetType() {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			err = one("type", f.GetSpaceType(), f.GetType())
		case provider.ListStorageSpacesRequest_Filter_TYPE_OWNER:
			err = one("owner", admin.UserArg(f.GetOwner()), f.GetType())
		case provider.ListStorageSpacesRequest_Filter_TYPE_USER:
			err = one("member", admin.UserArg(f.GetUser()), f.GetType())
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			if f.GetId().GetOpaqueId() == "" {
				return nil, nil, status.Errorf(codes.InvalidArgument, "admin: the filter %s has no value", f.GetType())
			}
			ids = append(ids, f.GetId().GetOpaqueId())
		case provider.ListStorageSpacesRequest_Filter_TYPE_PATH:
			if !path.IsAbs(f.GetPath()) {
				return nil, nil, status.Errorf(codes.InvalidArgument, "admin: the filter %s needs an absolute path, not %q", f.GetType(), f.GetPath())
			}
			paths = append(paths, path.Clean(f.GetPath()))
		default:
			return nil, nil, status.Errorf(codes.InvalidArgument, "admin: the catalogue cannot filter by %s", f.GetType())
		}
		if err != nil {
			return nil, nil, err
		}
	}
	keep := func(s *provider.StorageSpace) bool {
		if len(ids) > 0 && !slices.ContainsFunc(ids, func(id string) bool { return sameSpaceID(id, s.GetId().GetOpaqueId()) }) {
			return false
		}
		root := s.GetRootInfo().GetPath()
		return len(paths) == 0 || root != "" && slices.Contains(paths, path.Clean(root))
	}
	return args, keep, nil
}

// sameSpaceID matches a <storage id>$<space id>, or a space id alone, as the
// catalogue selects by id.
func sameSpaceID(want, id string) bool {
	if want == id {
		return true
	}
	_, spaceID, ok := strings.Cut(id, "$")
	return ok && !strings.Contains(want, "$") && want == spaceID
}

func timestamp(t *typespb.Timestamp) time.Time {
	return time.Unix(int64(t.GetSeconds()), int64(t.GetNanos())).UTC()
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
	fields, err := updateArgs(req.Fields)
	if err != nil {
		return nil, err
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	args, err := refArgs(ctx, eps, req.Ref)
	if err != nil {
		return nil, err
	}
	maps.Copy(args, fields)
	if req.Fallback {
		args["fallback"] = "true"
	}
	return mutateSpace(ctx, eps, "update", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

// updateArgs maps the CS3 update fields onto the arguments of the update
// invocation, which changes only what the catalogue stores.
func updateArgs(fields []*provider.UpdateStorageSpaceRequest_UpdateField) (map[string]string, error) {
	if len(fields) == 0 {
		return nil, status.Error(codes.InvalidArgument, "admin: update needs a description, a readme or a thumbnail")
	}
	args := map[string]string{}
	for _, f := range fields {
		var key, value string
		switch f.GetField().(type) {
		case *provider.UpdateStorageSpaceRequest_UpdateField_Description:
			key, value = "description", f.GetDescription()
		case *provider.UpdateStorageSpaceRequest_UpdateField_Metadata:
			switch t := f.GetMetadata().GetType(); t {
			case provider.SpaceMetadata_TYPE_README:
				key = "readme"
			case provider.SpaceMetadata_TYPE_THUMBNAIL:
				key = "thumbnail"
			default:
				return nil, status.Errorf(codes.InvalidArgument, "admin: the catalogue has no metadata %s", t)
			}
			value = f.GetMetadata().GetId()
		case *provider.UpdateStorageSpaceRequest_UpdateField_Quota:
			return nil, status.Error(codes.InvalidArgument, "admin: update does not set a quota: use SetSpaceQuota")
		case *provider.UpdateStorageSpaceRequest_UpdateField_Name:
			return nil, status.Error(codes.InvalidArgument, "admin: the catalogue cannot rename a space")
		case *provider.UpdateStorageSpaceRequest_UpdateField_Alias:
			return nil, status.Error(codes.InvalidArgument, "admin: the catalogue has no alias of a space")
		default:
			return nil, status.Error(codes.InvalidArgument, "admin: an update field has no value")
		}
		if _, ok := args[key]; ok {
			return nil, status.Errorf(codes.InvalidArgument, "admin: update has the %s twice", key)
		}
		// An empty value is a change: it clears the field.
		args[key] = value
	}
	return args, nil
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
	if req.GetOwner().GetOpaqueId() == "" {
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
	args["owner"] = admin.UserArg(req.GetOwner())
	return mutateSpace(ctx, eps, "set_owner", cmp.Or(req.Ref.GetId().GetOpaqueId(), req.Ref.GetName(), req.Ref.GetPath()), args)
}

func (a *spacesAdmin) RegisterSpace(ctx context.Context, req *adminpb.RegisterSpaceRequest) (*adminpb.AdminSpace, error) {
	args, err := registerArgs(req.GetSpace())
	if err != nil {
		return nil, err
	}
	if req.Status != "" {
		args["status"] = req.Status
	}
	eps, err := a.s.serviceEndpoints(spacesRegistry)
	if err != nil {
		return nil, err
	}
	return mutateSpace(ctx, eps, "create", args["name"], args)
}

// registerArgs maps a CS3 space onto the arguments of the create
// invocation. A value that the catalogue would not store is refused, so that
// a registration never drops it.
func registerArgs(s *provider.StorageSpace) (map[string]string, error) {
	invalid := func(format string, a ...any) error {
		return status.Errorf(codes.InvalidArgument, "admin: register: "+format, a...)
	}
	owner := admin.UserArg(s.GetOwner().GetId())
	if s.GetName() == "" || owner == "" {
		return nil, invalid("name and owner are required")
	}
	if t := s.GetSpaceType(); t != "" && t != spaces.SpaceTypeProject.AsString() {
		return nil, invalid("the catalogue has only spaces of type %s, not %s", spaces.SpaceTypeProject, t)
	}
	switch {
	case s.GetQuota() != nil:
		return nil, invalid("a quota is set with SetSpaceQuota")
	case s.GetReadmeId() != "" || s.GetThumbnailId() != "":
		return nil, invalid("a readme or a thumbnail is set with UpdateSpace")
	}
	args := map[string]string{
		"name":        s.GetName(),
		"owner":       owner,
		"path":        s.GetRootInfo().GetPath(),
		"description": s.GetDescription(),
	}
	storageID, spaceID := s.GetRoot().GetStorageId(), s.GetRoot().GetSpaceId()
	if id := s.GetId().GetOpaqueId(); id != "" {
		st, sp, ok := strings.Cut(id, "$")
		if !ok {
			st, sp = "", id
		}
		if storageID != "" && st != "" && storageID != st || spaceID != "" && spaceID != sp {
			return nil, invalid("the id %s and the root %s/%s differ", id, storageID, spaceID)
		}
		storageID, spaceID = cmp.Or(st, storageID), sp
	}
	args["storage_id"], args["space_id"] = storageID, spaceID
	for _, r := range s.GetRoles() {
		key, ok := roleArgs[r.GetRoleName()]
		if !ok {
			return nil, invalid("the catalogue has no role %q: use reader, writer or admin", r.GetRoleName())
		}
		if _, ok := args[key]; ok {
			return nil, invalid("the role %s is given twice", r.GetRoleName())
		}
		rs := r.GetRecipients()
		if len(rs) != 1 || rs[0].GetGroupId().GetOpaqueId() == "" {
			return nil, invalid("the role %s needs one group as its recipient", r.GetRoleName())
		}
		args[key] = rs[0].GetGroupId().GetOpaqueId()
	}
	maps.DeleteFunc(args, func(_, v string) bool { return v == "" })
	return args, nil
}

// roleArgs are the role names of the catalogue, as AdminSpace returns them,
// and the arguments of create that take their groups.
var roleArgs = map[string]string{"reader": "readers", "writer": "writers", "admin": "admins"}

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
	args := map[string]string{"status": req.Status, "owner": admin.UserArg(req.GetOwner())}
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
