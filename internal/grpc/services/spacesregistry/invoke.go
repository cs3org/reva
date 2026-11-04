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

package spacesregistry

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/stats"
	"github.com/cs3org/reva/v3/pkg/utils"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
)

// check resolves this many projects at a time before it sends their
// findings, so that a long stream starts to arrive early.
const (
	checkChunk   = 64
	checkWorkers = 16
)

type spaceByID interface {
	SpaceByID(ctx context.Context, spaceID string) (*provider.StorageSpace, error)
}

type spaceArgs struct {
	ID string `arg:"id,required" help:"the space id"`
}

type refArgs struct {
	ID   string `arg:"id" help:"the space id; give id or name"`
	Name string `arg:"name" help:"the project name; give id or name"`
}

type listArgs struct {
	Type            string                 `arg:"type" help:"only spaces of this type; the catalogue has only project spaces"`
	Status          projects.ProjectStatus `arg:"status" help:"only projects in this status: creating, active, archiving or archived"`
	Owner           string                 `arg:"owner" help:"only projects owned by this user"`
	Member          string                 `arg:"member" help:"only projects that this user owns or can access through a role group"`
	CreatedAfter    time.Time              `arg:"created_after" help:"only projects created at or after this RFC 3339 time, or this long ago (e.g. 720h)"`
	CreatedBefore   time.Time              `arg:"created_before" help:"only projects created at or before this RFC 3339 time, or this long ago"`
	IncludeArchived bool                   `arg:"include_archived" help:"include archived projects"`
}

type updateArgs struct {
	ID          string  `arg:"id" help:"the space id; give id or name"`
	Name        string  `arg:"name" help:"the project name; give id or name"`
	Description *string `arg:"description" help:"the new description"`
	Readme      *string `arg:"readme" help:"the path of the new readme"`
	Thumbnail   *string `arg:"thumbnail" help:"the path of the new thumbnail"`
	Fallback    bool    `arg:"fallback" help:"the caller fell back to admin rights after a user command, for the audit"`
}

type setStatusArgs struct {
	ID     string                 `arg:"id" help:"the space id; give id or name"`
	Name   string                 `arg:"name" help:"the project name; give id or name"`
	Status projects.ProjectStatus `arg:"status,required" help:"creating, active, archiving or archived"`
}

type setOwnerArgs struct {
	ID    string `arg:"id" help:"the space id; give id or name"`
	Name  string `arg:"name" help:"the project name; give id or name"`
	Owner string `arg:"owner,required" help:"the new owner"`
}

type createArgs struct {
	Name        string                 `arg:"name,required" help:"the project name"`
	Owner       string                 `arg:"owner,required" help:"the owner"`
	Path        string                 `arg:"path" help:"the path of the space; required if the catalogue has no path template"`
	StorageID   string                 `arg:"storage_id" help:"the storage that serves the space; required if the catalogue has no storage id template"`
	SpaceID     string                 `arg:"space_id" help:"the space id (default: derived from the path)"`
	Readers     string                 `arg:"readers" help:"the readers group; required if the catalogue has no template for it"`
	Writers     string                 `arg:"writers" help:"the writers group; required if the catalogue has no template for it"`
	Admins      string                 `arg:"admins" help:"the admins group; required if the catalogue has no template for it"`
	Description string                 `arg:"description" help:"the description shown to users"`
	Status      projects.ProjectStatus `arg:"status" help:"creating, active, archiving or archived (default active)"`
}

type deleteArgs struct {
	ID   string `arg:"id" help:"the space id; give id or name"`
	Name string `arg:"name" help:"the project name; give id or name"`
	Hard bool   `arg:"hard" help:"remove the row; by default it is soft-deleted and keeps its name and space id taken"`
}

type checkArgs struct {
	Status projects.ProjectStatus `arg:"status" help:"only projects in this status"`
	Owner  string                 `arg:"owner" help:"only projects owned by this user"`
}

type finding struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}

func (s *service) invocations() *invoke.Set {
	set := invoke.NewSet()
	if _, ok := s.projects.(stats.Reporter); ok {
		set = stats.NewInvokeSet(s)
	}
	set.Add("space", "Show a project space and the users and groups of its roles, whoever the caller is.").
		Handle(invoke.Typed(s.space))
	set.Add("list", "Stream the projects of the catalogue, of every owner.").
		Shared().
		HandleStream(invoke.TypedStream(s.list))
	set.Add("get", "Show one project of the catalogue, archived or not.").
		Shared().
		Handle(invoke.Typed(s.get))
	set.Add("update", "Change the description, the readme or the thumbnail of a project, as its owner.").
		Mutating().Shared().
		Handle(invoke.Typed(s.update))
	set.Add("set_status", "Set the status of a project. Then run flush_cache on every instance.").
		Mutating().Shared().
		Handle(invoke.Typed(s.setStatus))
	set.Add("set_owner", "Change the owner of a project. Then run flush_cache on every instance.").
		Mutating().Shared().
		Handle(invoke.Typed(s.setOwner))
	set.Add("create", "Add a project to the catalogue. The space must exist on the storage. Then run flush_cache on every instance.").
		Dangerous().Shared().
		Handle(invoke.Typed(s.create))
	set.Add("delete", "Remove a project from the catalogue: soft by default, or the row with hard. Then run flush_cache on every instance.").
		Dangerous().Shared().
		Handle(invoke.Typed(s.delete))
	set.Add("check", "Stream the projects whose path does not stat as the owner, whose owner is missing, expiring or blocked, or whose role groups have no members.").
		Shared().
		HandleStream(invoke.TypedStream(s.check))
	// Each instance caches the project list, so this one runs everywhere.
	set.Add("flush_cache", "Drop the project list that this instance caches.").
		Mutating().
		Handle(invoke.Typed(s.flushCache))
	return set
}

func (s *service) catalogue() (projects.Admin, error) {
	c, ok := s.projects.(projects.Admin)
	if !ok {
		return nil, errtypes.NotSupported(fmt.Sprintf("the %s driver does not support admin invocations", s.c.Driver))
	}
	return c, nil
}

func (s *service) space(ctx context.Context, a spaceArgs) (invoke.Result, error) {
	c, ok := s.projects.(spaceByID)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot look up a space by id", s.c.Driver)
	}
	space, err := c.SpaceByID(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	roles := make([]map[string]any, 0, len(space.GetRoles()))
	for _, r := range space.GetRoles() {
		users, groups := []string{}, []string{}
		for _, g := range r.GetRecipients() {
			userID, groupID := utils.ExtractGranteeID(g)
			if userID != nil {
				users = append(users, userID.GetOpaqueId())
			}
			if groupID != nil {
				groups = append(groups, groupID.GetOpaqueId())
			}
		}
		roles = append(roles, map[string]any{"name": r.GetRoleName(), "users": users, "groups": groups})
	}
	return invoke.Result{"id": space.GetId().GetOpaqueId(), "name": space.GetName(), "type": space.GetSpaceType(), "roles": roles}, nil
}

func (s *service) list(ctx context.Context, a listArgs, emit invoke.StreamEmit) error {
	c, err := s.catalogue()
	if err != nil {
		return err
	}
	if a.Type != "" && a.Type != spaces.SpaceTypeProject.AsString() {
		return errtypes.BadRequest(fmt.Sprintf("the catalogue has only spaces of type %s, not %s", spaces.SpaceTypeProject, a.Type))
	}
	f := projects.Filter{
		Status:          a.Status,
		Owner:           a.Owner,
		CreatedAfter:    a.CreatedAfter,
		CreatedBefore:   a.CreatedBefore,
		IncludeArchived: a.IncludeArchived,
	}
	if a.Member != "" {
		if f.Member, err = member(ctx, a.Member); err != nil {
			return err
		}
	}
	recs, err := c.ListProjectRecords(ctx, f)
	if err != nil {
		return err
	}
	for _, r := range recs {
		res, err := adminSpace(r)
		if err != nil {
			return err
		}
		if err := emit(res); err != nil {
			return err
		}
	}
	return nil
}

// member returns the user with the groups that the userprovider gives.
func member(ctx context.Context, username string) (*userpb.User, error) {
	u, err := admin.Lookup(ctx, username)
	if err != nil {
		return nil, err
	}
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUserGroups(ctx, &userpb.GetUserGroupsRequest{UserId: u.GetId()})
	if err != nil {
		return nil, err
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("the groups of %s: %w", username, status.NewErrtypeFromStatus(res.GetStatus()))
	}
	return &userpb.User{Id: u.GetId(), Username: u.GetUsername(), Groups: res.GetGroups()}, nil
}

func (s *service) get(ctx context.Context, a refArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	r, err := c.ProjectRecord(ctx, projects.Ref{SpaceID: a.ID, Name: a.Name})
	if err != nil {
		return nil, err
	}
	return adminSpace(r)
}

func (s *service) update(ctx context.Context, a updateArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	var fields []*provider.UpdateStorageSpaceRequest_UpdateField
	if a.Description != nil {
		fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Description{Description: *a.Description}})
	}
	if a.Readme != nil {
		fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{Metadata: &provider.SpaceMetadata{Type: provider.SpaceMetadata_TYPE_README, Id: *a.Readme}}})
	}
	if a.Thumbnail != nil {
		fields = append(fields, &provider.UpdateStorageSpaceRequest_UpdateField{Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{Metadata: &provider.SpaceMetadata{Type: provider.SpaceMetadata_TYPE_THUMBNAIL, Id: *a.Thumbnail}}})
	}
	if len(fields) == 0 {
		return nil, errtypes.BadRequest("update needs description, readme or thumbnail")
	}
	ref := projects.Ref{SpaceID: a.ID, Name: a.Name}
	r, err := c.ProjectRecord(ctx, ref)
	if err != nil {
		return nil, err
	}
	// A user call selects the live project of a name.
	if r.Status == projects.ProjectStatusArchived {
		return nil, errtypes.BadRequest(fmt.Sprintf("project %s is archived", r.Space.GetName()))
	}

	// The catalogue lets the owner change a project, so the change is made
	// as the owner, with the checks of a user call.
	owner := r.Space.GetOwner().GetId().GetOpaqueId()
	uctx, _, err := admin.ActAs(ctx, owner)
	for _, f := range fields {
		if err != nil {
			break
		}
		var res *provider.UpdateStorageSpaceResponse
		res, err = s.UpdateStorageSpace(uctx, &provider.UpdateStorageSpaceRequest{StorageSpace: r.Space, Field: f})
		if err == nil && res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			err = status.NewErrtypeFromStatus(res.GetStatus())
		}
	}
	logged := map[string]string{"acted_as": owner}
	if a.Description != nil {
		logged["description"] = *a.Description
	}
	if a.Readme != nil {
		logged["readme"] = *a.Readme
	}
	if a.Thumbnail != nil {
		logged["thumbnail"] = *a.Thumbnail
	}
	if a.Fallback {
		logged["fallback"] = "true"
	}
	audit(ctx, "update", r.Space.GetName(), err, logged)
	if err != nil {
		return nil, fmt.Errorf("updating project %s as its owner %s: %w", r.Space.GetName(), owner, err)
	}
	if r, err = c.ProjectRecord(ctx, ref); err != nil {
		return nil, err
	}
	return adminSpace(r)
}

func (s *service) setStatus(ctx context.Context, a setStatusArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	r, err := c.SetProjectStatus(ctx, projects.Ref{SpaceID: a.ID, Name: a.Name}, a.Status)
	audit(ctx, "set_status", cmp.Or(a.Name, a.ID), err, map[string]string{"status": string(a.Status)})
	if err != nil {
		return nil, err
	}
	return adminSpace(r)
}

func (s *service) setOwner(ctx context.Context, a setOwnerArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	ref := projects.Ref{SpaceID: a.ID, Name: a.Name}
	r, err := c.ProjectRecord(ctx, ref)
	if err != nil {
		return nil, err
	}
	from := r.Space.GetOwner().GetId().GetOpaqueId()
	u, err := admin.Lookup(ctx, a.Owner)
	if err == nil {
		r, err = c.SetProjectOwner(ctx, ref, u.GetId().GetOpaqueId())
	}
	audit(ctx, "set_owner", cmp.Or(a.Name, a.ID), err, map[string]string{"owner": a.Owner, "from": from})
	if err != nil {
		return nil, err
	}
	return adminSpace(r)
}

func (s *service) create(ctx context.Context, a createArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	var r *projects.Record
	u, err := admin.Lookup(ctx, a.Owner)
	if err == nil {
		r, err = c.RegisterProject(ctx, projects.Registration{
			Name:        a.Name,
			Owner:       u.GetId().GetOpaqueId(),
			Path:        a.Path,
			StorageID:   a.StorageID,
			SpaceID:     a.SpaceID,
			Readers:     a.Readers,
			Writers:     a.Writers,
			Admins:      a.Admins,
			Description: a.Description,
			Status:      a.Status,
		})
	}
	if err != nil {
		audit(ctx, "create", a.Name, err, map[string]string{"owner": a.Owner})
		return nil, err
	}
	audit(ctx, "create", a.Name, nil, map[string]string{"owner": a.Owner, "path": r.Space.GetRootInfo().GetPath(), "space_id": r.Space.GetId().GetOpaqueId()})
	return adminSpace(r)
}

func (s *service) delete(ctx context.Context, a deleteArgs) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	r, err := c.UnregisterProject(ctx, projects.Ref{SpaceID: a.ID, Name: a.Name}, a.Hard)
	audit(ctx, "delete", cmp.Or(a.Name, a.ID), err, map[string]string{"hard": strconv.FormatBool(a.Hard)})
	if err != nil {
		return nil, err
	}
	return adminSpace(r)
}

func (s *service) check(ctx context.Context, a checkArgs, emit invoke.StreamEmit) error {
	c, err := s.catalogue()
	if err != nil {
		return err
	}
	recs, err := c.ListProjectRecords(ctx, projects.Filter{Status: a.Status, Owner: a.Owner})
	if err != nil {
		return err
	}
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return err
	}
	for chunk := range slices.Chunk(recs, checkChunk) {
		found := make([][]finding, len(chunk))
		var g errgroup.Group
		g.SetLimit(checkWorkers)
		for i, r := range chunk {
			g.Go(func() error {
				found[i] = checkProject(ctx, gw, r)
				return nil
			})
		}
		_ = g.Wait()
		for i, r := range chunk {
			if len(found[i]) == 0 {
				continue
			}
			space, err := adminSpace(r)
			if err != nil {
				return err
			}
			if err := emit(invoke.Result{"space": space, "findings": found[i]}); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkProject(ctx context.Context, gw gateway.GatewayAPIClient, r *projects.Record) []finding {
	var out []finding
	owner := r.Space.GetOwner().GetId().GetOpaqueId()
	u, err := admin.Lookup(ctx, owner)
	var nf errtypes.IsNotFound
	switch {
	case errors.As(err, &nf):
		out = append(out, finding{Kind: "owner_missing", Detail: owner})
	case err != nil:
		out = append(out, finding{Kind: "owner_error", Detail: err.Error()})
	case u.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE:
		out = append(out, finding{Kind: "owner_" + strings.ToLower(strings.TrimPrefix(u.GetStatus().String(), "USER_STATUS_")), Detail: owner})
	}

	// The storage lets the owner see the project, and without an owner
	// there is nobody to stat the path as.
	if u != nil {
		path := r.Space.GetRootInfo().GetPath()
		uctx, err := admin.As(ctx, u)
		var res *provider.StatResponse
		if err == nil {
			res, err = gw.Stat(uctx, &provider.StatRequest{Ref: &provider.Reference{Path: path}})
		}
		switch {
		case err != nil:
			out = append(out, finding{Kind: "path_error", Detail: err.Error()})
		case res.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND:
			out = append(out, finding{Kind: "path_missing", Detail: path})
		case res.GetStatus().GetCode() != rpc.Code_CODE_OK:
			out = append(out, finding{Kind: "path_error", Detail: res.GetStatus().GetCode().String() + ": " + res.GetStatus().GetMessage()})
		}
	}

	members, known := 0, true
	for _, role := range r.Space.GetRoles() {
		for _, g := range role.GetRecipients() {
			gid := g.GetGroupId().GetOpaqueId()
			if gid == "" {
				out = append(out, finding{Kind: "group_unset", Detail: role.GetRoleName()})
				continue
			}
			res, err := gw.GetMembers(ctx, &grouppb.GetMembersRequest{GroupId: &grouppb.GroupId{OpaqueId: gid}})
			switch {
			case err != nil:
				known = false
				out = append(out, finding{Kind: "group_error", Detail: gid + ": " + err.Error()})
			case res.GetStatus().GetCode() == rpc.Code_CODE_NOT_FOUND:
				out = append(out, finding{Kind: "group_missing", Detail: gid})
			case res.GetStatus().GetCode() != rpc.Code_CODE_OK:
				known = false
				out = append(out, finding{Kind: "group_error", Detail: gid + ": " + res.GetStatus().GetCode().String() + ": " + res.GetStatus().GetMessage()})
			default:
				members += len(res.GetMembers())
			}
		}
	}
	if known && members == 0 {
		out = append(out, finding{Kind: "no_members"})
	}
	return out
}

func (s *service) flushCache(_ context.Context, _ struct{}) (invoke.Result, error) {
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	c.InvalidateCache()
	return invoke.Result{"flushed": true}, nil
}

func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	maps.DeleteFunc(fields, func(_, v string) bool { return v == "" })
	u, _ := appctx.ContextGetUser(ctx)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "spacesregistry." + action,
		Actor:   u.GetUsername(),
		Target:  "project:" + target,
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

func adminSpace(r *projects.Record) (invoke.Result, error) {
	b, err := protojson.Marshal(r.Space)
	if err != nil {
		return nil, err
	}
	var space map[string]any
	if err := json.Unmarshal(b, &space); err != nil {
		return nil, err
	}
	res := invoke.Result{"space": space, "status": string(r.Status)}
	if len(r.Extra) > 0 {
		res["extra"] = r.Extra
	}
	return res, nil
}
