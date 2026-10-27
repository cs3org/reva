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
	"maps"
	"path"
	"strconv"
	"strings"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	userShareService   = "usershareprovider"
	publicShareService = "publicshareprovider"
)

func (a *sharesAdmin) ListShares(req *adminpb.ListSharesRequest, stream adminpb.SharesAdmin_ListSharesServer) error {
	ctx := stream.Context()
	args, err := shareFilterArgs(req.GetFilters())
	if err != nil {
		return err
	}
	if err := a.s.addSpace(ctx, args, req.GetSpace()); err != nil {
		return err
	}
	maps.Copy(args, shareArgs(map[string]string{
		"path": req.GetPath(), "as": admin.UserArg(req.GetAs()), "resolve_paths": shareBool(req.GetResolvePaths()),
	}))
	addSelection(args, req.GetOrphans(), req.GetExpired())
	eps, err := a.s.serviceEndpoints(userShareService)
	if err != nil {
		return err
	}
	keep := keepSelected(req.GetOrphans(), req.GetExpired())
	return relayStream(ctx, eps, "list", args, func(m *adminpb.AdminShare) error {
		if !keep(m.GetOrphan(), m.GetShare().GetExpiration()) {
			return nil
		}
		return stream.Send(m)
	})
}

func (a *sharesAdmin) GetShare(ctx context.Context, req *adminpb.GetShareRequest) (*adminpb.AdminShare, error) {
	args, err := shareRefArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	out := &adminpb.AdminShare{}
	if err := a.s.invokeDecoded(ctx, userShareService, "get", args, out); err != nil {
		return nil, err
	}
	return out, nil
}

// The mutations of the share provider take an id, so shareID resolves a key with get first.
func (a *sharesAdmin) shareID(ctx context.Context, ref *collaboration.ShareReference) (string, error) {
	if id := ref.GetId().GetOpaqueId(); id != "" {
		return id, nil
	}
	s, err := a.GetShare(ctx, &adminpb.GetShareRequest{Ref: ref})
	if err != nil {
		return "", err
	}
	if id := s.GetShare().GetId().GetOpaqueId(); id != "" {
		return id, nil
	}
	return "", status.Error(codes.Internal, "admin: the share provider returned a share without an id")
}

func (a *sharesAdmin) UpdateShare(ctx context.Context, req *adminpb.UpdateShareRequest) (*adminpb.AdminShare, error) {
	role, exp, err := shareUpdateArgs(req.GetFields())
	if err != nil {
		return nil, err
	}
	id, err := a.shareID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	args := shareArgs(map[string]string{"id": id, "role": role, "expires_at": exp, "fallback": shareBool(req.GetFallback())})
	return a.shareMutation(ctx, "update_share", id, req.GetFallback(), "update", args,
		map[string]string{"role": role, "expires_at": exp})
}

func (a *sharesAdmin) RemoveShare(ctx context.Context, req *adminpb.RemoveShareRequest) (*adminpb.AdminShare, error) {
	id, err := a.shareID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	args := shareArgs(map[string]string{"id": id, "db_only": shareBool(req.GetDbOnly()), "fallback": shareBool(req.GetFallback())})
	return a.shareMutation(ctx, "remove_share", id, req.GetFallback(), "remove", args,
		map[string]string{"db_only": shareBool(req.GetDbOnly())})
}

func (a *sharesAdmin) SyncShareExpiration(ctx context.Context, req *adminpb.SyncShareExpirationRequest) (*adminpb.AdminShare, error) {
	id, err := a.shareID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return a.shareMutation(ctx, "sync_share_expiration", id, false, "sync_expiry", map[string]string{"id": id}, nil)
}

func (a *sharesAdmin) TransferShare(ctx context.Context, req *adminpb.TransferShareRequest) (*adminpb.AdminShare, error) {
	initiator := admin.UserArg(req.GetInitiator())
	if initiator == "" {
		return nil, shareArgRequired("initiator")
	}
	id, err := a.shareID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return a.shareMutation(ctx, "transfer_share", id, false, "transfer",
		map[string]string{"id": id, "initiator": initiator}, map[string]string{"to": req.GetInitiator().GetOpaqueId()})
}

func (a *sharesAdmin) MarkShareOrphan(ctx context.Context, req *adminpb.MarkShareOrphanRequest) (*adminpb.AdminShare, error) {
	id, err := a.shareID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return a.shareMutation(ctx, "mark_share_orphan", id, false, "mark_orphan", map[string]string{"id": id}, nil)
}

func (a *sharesAdmin) shareMutation(ctx context.Context, action, id string, fallback bool, invocation string, args, fields map[string]string) (*adminpb.AdminShare, error) {
	out := &adminpb.AdminShare{}
	err := a.s.invokeDecoded(ctx, userShareService, invocation, args, out)
	auditShares(ctx, action, "share:"+id, fallback, err, fields)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *sharesAdmin) ListPublicLinks(req *adminpb.ListPublicLinksRequest, stream adminpb.SharesAdmin_ListPublicLinksServer) error {
	ctx := stream.Context()
	args, err := linkFilterArgs(req.GetFilters())
	if err != nil {
		return err
	}
	if err := a.s.addSpace(ctx, args, req.GetSpace()); err != nil {
		return err
	}
	if req.GetResolvePaths() {
		args["resolve_paths"] = "true"
	}
	addSelection(args, req.GetOrphans(), req.GetExpired())
	eps, err := a.s.serviceEndpoints(publicShareService)
	if err != nil {
		return err
	}
	keep := keepSelected(req.GetOrphans(), req.GetExpired())
	return relayStream(ctx, eps, "list", args, func(m *adminpb.AdminPublicShare) error {
		if !keep(m.GetOrphan(), m.GetPublicShare().GetExpiration()) {
			return nil
		}
		return stream.Send(m)
	})
}

func (a *sharesAdmin) GetPublicLink(ctx context.Context, req *adminpb.GetPublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	out := &adminpb.AdminPublicShare{}
	if err := a.s.invokeDecoded(ctx, publicShareService, "get", args, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *sharesAdmin) UpdatePublicLink(ctx context.Context, req *adminpb.UpdatePublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	role, exp, password, err := linkUpdateArgs(req.GetUpdates())
	if err != nil {
		return nil, err
	}
	maps.Copy(args, shareArgs(map[string]string{"role": role, "expires_at": exp, "fallback": shareBool(req.GetFallback())}))
	// The audit says only whether the password changed.
	fields := map[string]string{"role": role, "expires_at": exp}
	if password != nil {
		args["password"] = *password
		fields["password"] = "set"
		if *password == "" {
			fields["password"] = "removed"
		}
	}
	return a.linkMutation(ctx, "update_public_link", req.GetRef(), req.GetFallback(), "update", args, fields)
}

func (a *sharesAdmin) RemovePublicLink(ctx context.Context, req *adminpb.RemovePublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	maps.Copy(args, shareArgs(map[string]string{"db_only": shareBool(req.GetDbOnly()), "fallback": shareBool(req.GetFallback())}))
	return a.linkMutation(ctx, "remove_public_link", req.GetRef(), req.GetFallback(), "remove", args,
		map[string]string{"db_only": shareBool(req.GetDbOnly())})
}

func (a *sharesAdmin) TransferPublicLink(ctx context.Context, req *adminpb.TransferPublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	initiator := admin.UserArg(req.GetInitiator())
	if initiator == "" {
		return nil, shareArgRequired("initiator")
	}
	args["initiator"] = initiator
	return a.linkMutation(ctx, "transfer_public_link", req.GetRef(), false, "transfer", args, map[string]string{"to": req.GetInitiator().GetOpaqueId()})
}

func (a *sharesAdmin) MarkPublicLinkOrphan(ctx context.Context, req *adminpb.MarkPublicLinkOrphanRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	return a.linkMutation(ctx, "mark_public_link_orphan", req.GetRef(), false, "mark_orphan", args, nil)
}

func (a *sharesAdmin) linkMutation(ctx context.Context, action string, ref *link.PublicShareReference, fallback bool, invocation string, args, fields map[string]string) (*adminpb.AdminPublicShare, error) {
	out := &adminpb.AdminPublicShare{}
	err := a.s.invokeDecoded(ctx, publicShareService, invocation, args, out)
	// The token of a link opens it, so the audit names the link by id only.
	target := "link:" + ref.GetId().GetOpaqueId()
	if id := out.GetPublicShare().GetId().GetOpaqueId(); id != "" {
		target = "link:" + id
	} else if ref.GetId().GetOpaqueId() == "" {
		target = "link:<token>"
	}
	auditShares(ctx, action, target, fallback, err, fields)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MoveShares finds the candidates with the list invocations, because the share tables keep
// the resource id and not the path.
func (a *sharesAdmin) MoveShares(req *adminpb.MoveSharesRequest, stream adminpb.SharesAdmin_MoveSharesServer) error {
	ctx := stream.Context()
	from, to := path.Clean(req.GetFromPath()), path.Clean(req.GetToPath())
	switch {
	case req.GetFromPath() == "" || req.GetToPath() == "":
		return shareArgRequired("from_path and to_path")
	case !path.IsAbs(from) || !path.IsAbs(to) || from == "/":
		return status.Error(codes.InvalidArgument, "admin: from_path and to_path must be absolute, and from_path must not be /")
	case from == to:
		return status.Error(codes.InvalidArgument, "admin: from_path and to_path are the same")
	case req.GetOwner().GetOpaqueId() == "" && req.GetSpace() == nil:
		return shareArgRequired("owner or space")
	}
	listArgs := shareArgs(map[string]string{"owner": admin.UserArg(req.GetOwner()), "resolve_paths": "true"})
	if err := a.s.addSpace(ctx, listArgs, req.GetSpace()); err != nil {
		return err
	}
	newPath := func(current string, extra map[string]string) (string, string, bool) {
		p := current
		if p == "" {
			p = extra["initial_path"]
		}
		if p != from && !strings.HasPrefix(p, from+"/") {
			return "", "", false
		}
		return p, path.Join(to, strings.TrimPrefix(p, from)), true
	}

	var items []*adminpb.MovedShare
	shareEps, err := a.s.serviceEndpoints(userShareService)
	if err != nil {
		return err
	}
	shareListArgs := shareArgs(map[string]string{"as": admin.UserArg(req.GetAs())})
	maps.Copy(shareListArgs, listArgs)
	if err := relayStream(ctx, shareEps, "list", shareListArgs, func(m *adminpb.AdminShare) error {
		if p, np, ok := newPath(m.GetPath(), m.GetExtra()); ok {
			items = append(items, &adminpb.MovedShare{Item: &adminpb.MovedShare_Share{Share: m}, FromPath: p, ToPath: np})
		}
		return nil
	}); err != nil {
		return err
	}
	linkEps, err := a.s.serviceEndpoints(publicShareService)
	if err != nil {
		return err
	}
	if err := relayStream(ctx, linkEps, "list", listArgs, func(m *adminpb.AdminPublicShare) error {
		if p, np, ok := newPath(m.GetPath(), m.GetExtra()); ok {
			items = append(items, &adminpb.MovedShare{Item: &adminpb.MovedShare_PublicShare{PublicShare: m}, FromPath: p, ToPath: np})
		}
		return nil
	}); err != nil {
		return err
	}

	// One failure does not stop the others: the operator sees every item that failed.
	var firstErr error
	failed := 0
	for _, it := range items {
		if !req.GetDryRun() {
			if err := a.moveOne(ctx, it, req); err != nil {
				it.Error = status.Convert(err).Message()
				if firstErr == nil {
					firstErr = err
				}
				failed++
			}
		}
		if err := stream.Send(it); err != nil {
			return err
		}
	}
	if !req.GetDryRun() {
		auditShares(ctx, "move_shares", "path:"+from, false, firstErr, map[string]string{
			"to_path": to, "moved": strconv.Itoa(len(items) - failed), "failed": strconv.Itoa(failed),
		})
	}
	if firstErr != nil {
		return status.Errorf(status.Code(firstErr), "admin: %d of %d moves failed, the first: %s", failed, len(items), status.Convert(firstErr).Message())
	}
	return nil
}

func (a *sharesAdmin) moveOne(ctx context.Context, it *adminpb.MovedShare, req *adminpb.MoveSharesRequest) error {
	args := shareArgs(map[string]string{"path": it.GetToPath(), "as": admin.UserArg(req.GetAs())})
	var service string
	var out proto.Message
	switch {
	case it.GetShare() != nil:
		service, out = userShareService, &adminpb.AdminShare{}
		args["id"] = it.GetShare().GetShare().GetId().GetOpaqueId()
		if req.GetApplyGrant() {
			args["apply_grant"] = "true"
		}
	default:
		service, out = publicShareService, &adminpb.AdminPublicShare{}
		args["id"] = it.GetPublicShare().GetPublicShare().GetId().GetOpaqueId()
	}
	if err := a.s.invokeDecoded(ctx, service, "move", args, out); err != nil {
		return err
	}
	switch m := out.(type) {
	case *adminpb.AdminShare:
		it.Item = &adminpb.MovedShare_Share{Share: m}
	case *adminpb.AdminPublicShare:
		it.Item = &adminpb.MovedShare_PublicShare{PublicShare: m}
	}
	return nil
}

func (s *svc) invokeDecoded(ctx context.Context, service, invocation string, args map[string]string, m proto.Message) error {
	eps, err := s.serviceEndpoints(service)
	if err != nil {
		return err
	}
	res, err := invokeOne(ctx, eps, invocation, args)
	if err != nil {
		return err
	}
	return decode(res.ResultJSON, m)
}

// A SPACE_ID filter sets the space argument too. Both together are refused, because a space has one id.
func (s *svc) addSpace(ctx context.Context, args map[string]string, ref *adminpb.SpaceRef) error {
	if ref == nil {
		return nil
	}
	if _, ok := args["space"]; ok {
		return status.Error(codes.InvalidArgument, "admin: give the space in a SPACE_ID filter or in space, not both")
	}
	id, err := s.shareSpaceID(ctx, ref)
	if err != nil {
		return err
	}
	args["space"] = id
	return nil
}

// shareSpaceID returns the space id that the share tables keep: the part after the storage id.
func (s *svc) shareSpaceID(ctx context.Context, ref *adminpb.SpaceRef) (string, error) {
	var id string
	switch r := ref.GetRef().(type) {
	case nil:
		if ref != nil {
			return "", shareArgRequired("a space id or name")
		}
		return "", nil
	case *adminpb.SpaceRef_Id:
		id = r.Id.GetOpaqueId()
	case *adminpb.SpaceRef_Name:
		if r.Name == "" {
			return "", shareArgRequired("a space name")
		}
		eps, err := s.serviceEndpoints(spacesRegistry)
		if err != nil {
			return "", err
		}
		res, err := invokeOne(ctx, eps, "get", map[string]string{"name": r.Name})
		if err != nil {
			return "", err
		}
		var out struct {
			Space json.RawMessage `json:"space"`
		}
		if err := json.Unmarshal([]byte(res.ResultJSON), &out); err != nil {
			return "", status.Errorf(codes.Internal, "admin: decoding the space %s: %v", r.Name, err)
		}
		var space provider.StorageSpace
		if err := decode(string(out.Space), &space); err != nil {
			return "", err
		}
		id = space.GetId().GetOpaqueId()
	case *adminpb.SpaceRef_Path:
		eps, err := s.serviceEndpoints(spacesRegistry)
		if err != nil {
			return "", err
		}
		m, err := spaceAt(ctx, eps, r.Path)
		if err != nil {
			return "", err
		}
		id = m.GetSpace().GetId().GetOpaqueId()
	}
	if id == "" {
		return "", shareArgRequired("a space id")
	}
	return tableSpaceID(id), nil
}

func tableSpaceID(id string) string {
	if _, spaceID, ok := strings.Cut(id, "$"); ok {
		return spaceID
	}
	return id
}

// The invocations have no ONLY selection: keepSelected applies it.
func addSelection(args map[string]string, orphans, expired adminpb.ShareSelection) {
	if orphans == adminpb.ShareSelection_SHARE_SELECTION_INCLUDE || orphans == adminpb.ShareSelection_SHARE_SELECTION_ONLY {
		args["include_orphans"] = "true"
	}
	if expired == adminpb.ShareSelection_SHARE_SELECTION_EXCLUDE {
		args["include_expired"] = "false"
	}
}

func keepSelected(orphans, expired adminpb.ShareSelection) func(orphan bool, expiration *typespb.Timestamp) bool {
	now := time.Now()
	return func(orphan bool, expiration *typespb.Timestamp) bool {
		if orphans == adminpb.ShareSelection_SHARE_SELECTION_ONLY && !orphan {
			return false
		}
		if expired == adminpb.ShareSelection_SHARE_SELECTION_ONLY {
			return expiration.GetSeconds() != 0 && !time.Unix(int64(expiration.GetSeconds()), 0).After(now)
		}
		return true
	}
}

func shareRefArgs(ref *collaboration.ShareReference) (map[string]string, error) {
	switch r := ref.GetSpec().(type) {
	case *collaboration.ShareReference_Id:
		if id := r.Id.GetOpaqueId(); id != "" {
			return map[string]string{"id": id}, nil
		}
	case *collaboration.ShareReference_Key:
		if r.Key != nil {
			b, err := protojson.Marshal(r.Key)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "admin: the share key: %v", err)
			}
			return map[string]string{"key": string(b)}, nil
		}
	}
	return nil, shareArgRequired("the id or the key of the share")
}

func linkArgs(ref *link.PublicShareReference) (map[string]string, error) {
	switch r := ref.GetSpec().(type) {
	case *link.PublicShareReference_Id:
		if id := r.Id.GetOpaqueId(); id != "" {
			return map[string]string{"id": id}, nil
		}
	case *link.PublicShareReference_Token:
		if r.Token != "" {
			return map[string]string{"token": r.Token}, nil
		}
	}
	return nil, shareArgRequired("the id or the token of the link")
}

// The invocations take one value of each argument, so a second filter of a type is
// refused instead of read as the OR of CS3.
type filterArgs map[string]string

func (f filterArgs) set(typ fmt.Stringer, name, value string) error {
	if value == "" {
		return status.Errorf(codes.InvalidArgument, "admin: the %s filter has no value", typ)
	}
	if _, ok := f[name]; ok {
		return status.Errorf(codes.InvalidArgument, "admin: give at most one %s filter", typ)
	}
	f[name] = value
	return nil
}

func shareFilterArgs(filters []*collaboration.Filter) (map[string]string, error) {
	args := filterArgs{}
	for _, f := range filters {
		var err error
		switch t := f.GetType(); t {
		case collaboration.Filter_TYPE_OWNER:
			err = args.set(t, "owner", admin.UserArg(f.GetOwner()))
		case collaboration.Filter_TYPE_CREATOR:
			err = args.set(t, "initiator", admin.UserArg(f.GetCreator()))
		case collaboration.Filter_TYPE_GRANTEE:
			err = args.set(t, "sharee", admin.GranteeArg(f.GetGrantee()))
		case collaboration.Filter_TYPE_RESOURCE_ID:
			err = args.set(t, "resource_id", resourceIDArg(f.GetResourceId()))
		case collaboration.Filter_TYPE_SPACE_ID:
			err = args.set(t, "space", tableSpaceID(f.GetSpaceId()))
		default:
			err = status.Errorf(codes.InvalidArgument, "admin: the share provider cannot filter by %s", t)
		}
		if err != nil {
			return nil, err
		}
	}
	return args, nil
}

func linkFilterArgs(filters []*link.ListPublicSharesRequest_Filter) (map[string]string, error) {
	args := filterArgs{}
	for _, f := range filters {
		var err error
		switch t := f.GetType(); t {
		case link.ListPublicSharesRequest_Filter_TYPE_OWNER:
			err = args.set(t, "owner", admin.UserArg(f.GetOwner()))
		case link.ListPublicSharesRequest_Filter_TYPE_CREATOR:
			err = args.set(t, "initiator", admin.UserArg(f.GetCreator()))
		case link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID:
			err = args.set(t, "resource_id", resourceIDArg(f.GetResourceId()))
		default:
			err = status.Errorf(codes.InvalidArgument, "admin: the link provider cannot filter by %s", t)
		}
		if err != nil {
			return nil, err
		}
	}
	return args, nil
}

func resourceIDArg(id *provider.ResourceId) string {
	if id.GetStorageId() == "" || id.GetOpaqueId() == "" {
		return ""
	}
	return id.GetStorageId() + "!" + id.GetOpaqueId()
}

func shareUpdateArgs(fields []*collaboration.UpdateShareRequest_UpdateField) (role, expiresAt string, err error) {
	for _, f := range fields {
		switch v := f.GetField().(type) {
		case *collaboration.UpdateShareRequest_UpdateField_Permissions:
			if role != "" {
				return "", "", status.Error(codes.InvalidArgument, "admin: give the permissions once")
			}
			if role, err = roleName(v.Permissions.GetPermissions()); err != nil {
				return "", "", err
			}
		case *collaboration.UpdateShareRequest_UpdateField_Expiration:
			if expiresAt != "" {
				return "", "", status.Error(codes.InvalidArgument, "admin: give the expiration once")
			}
			expiresAt = expiresArg(v.Expiration)
		case *collaboration.UpdateShareRequest_UpdateField_DisplayName:
			return "", "", status.Error(codes.InvalidArgument, "admin: the display name of a share cannot be changed")
		default:
			return "", "", status.Error(codes.InvalidArgument, "admin: an update field has no value")
		}
	}
	if role == "" && expiresAt == "" {
		return "", "", shareArgRequired("a change of the permissions or of the expiration")
	}
	return role, expiresAt, nil
}

func linkUpdateArgs(updates []*link.UpdatePublicShareRequest_Update) (role, expiresAt string, password *string, err error) {
	seen := map[link.UpdatePublicShareRequest_Update_Type]bool{}
	for _, u := range updates {
		t := u.GetType()
		if seen[t] {
			return "", "", nil, status.Errorf(codes.InvalidArgument, "admin: give at most one %s update", t)
		}
		seen[t] = true
		switch t {
		case link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS:
			if role, err = roleName(u.GetGrant().GetPermissions().GetPermissions()); err != nil {
				return "", "", nil, err
			}
		case link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION:
			expiresAt = expiresArg(u.GetGrant().GetExpiration())
		case link.UpdatePublicShareRequest_Update_TYPE_PASSWORD:
			pw := u.GetGrant().GetPassword()
			password = &pw
		default:
			return "", "", nil, status.Errorf(codes.InvalidArgument, "admin: the link provider cannot apply a %s update", t)
		}
	}
	if len(seen) == 0 {
		return "", "", nil, shareArgRequired("a change of the permissions, of the expiration or of the password")
	}
	return role, expiresAt, password, nil
}

// The providers give a role its own permissions, so permissions of no role cannot be set.
func roleName(p *provider.ResourcePermissions) (string, error) {
	if p == nil {
		return "", status.Error(codes.InvalidArgument, "admin: the permissions update has no permissions")
	}
	switch r := permissions.RoleFromResourcePermissions(p); r.Name {
	case permissions.RoleUnknown, permissions.RoleLegacy:
		return "", status.Error(codes.InvalidArgument, "admin: the permissions are not those of a role")
	default:
		return r.Name, nil
	}
}

// CS3 removes an expiration with an empty one.
func expiresArg(exp *typespb.Timestamp) string {
	if exp.GetSeconds() == 0 {
		return "none"
	}
	return time.Unix(int64(exp.GetSeconds()), 0).UTC().Format(time.RFC3339)
}

// The owning service audits the mutation too, with the user it acted as.
func auditShares(ctx context.Context, action, target string, fallback bool, err error, fields map[string]string) {
	out := shareArgs(fields)
	if fallback {
		out["fallback"] = "true"
	}
	admin.Audit(ctx, admin.AuditEvent{Action: action, Actor: actorName(ctx), Target: target, Granted: err == nil, Err: err, Fields: out})
}

// shareArgs drops the empty values, so that the invocation uses its defaults.
func shareArgs(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func shareBool(b bool) string {
	if b {
		return "true"
	}
	return ""
}

func shareArgRequired(what string) error {
	return status.Error(codes.InvalidArgument, fmt.Sprintf("admin: %s is required", what))
}
