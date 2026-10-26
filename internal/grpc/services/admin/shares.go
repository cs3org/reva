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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// SharesAdmin is a facade over the invocations of the usershareprovider and
// of the publicshareprovider. They act as the identity that the share checks
// accept, so the admin service needs no CS3 call of its own.
const (
	userShareService   = "usershareprovider"
	publicShareService = "publicshareprovider"
)

func (a *sharesAdmin) ListShares(req *adminpb.ListSharesRequest, stream adminpb.SharesAdmin_ListSharesServer) error {
	ctx := stream.Context()
	spaceID, err := a.s.shareSpaceID(ctx, req.GetSpace())
	if err != nil {
		return err
	}
	args := shareArgs(map[string]string{
		"owner": req.GetOwner(), "initiator": req.GetInitiator(), "sharee": req.GetSharee(), "id": req.GetId(),
		"space": spaceID, "path": req.GetPath(), "as": req.GetAs(), "resolve_paths": shareBool(req.GetResolvePaths()),
	})
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
	if req.GetId() == "" {
		return nil, shareArgRequired("id")
	}
	out := &adminpb.AdminShare{}
	if err := a.s.invokeDecoded(ctx, userShareService, "get", map[string]string{"id": req.GetId()}, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *sharesAdmin) UpdateShare(ctx context.Context, req *adminpb.UpdateShareRequest) (*adminpb.AdminShare, error) {
	if req.GetId() == "" {
		return nil, shareArgRequired("id")
	}
	exp, err := expiresArg(req.GetExpiration(), req.GetRemoveExpiration())
	if err != nil {
		return nil, err
	}
	args := shareArgs(map[string]string{"id": req.GetId(), "role": req.GetRole(), "expires_at": exp, "fallback": shareBool(req.GetFallback())})
	return a.shareMutation(ctx, "update_share", req.GetId(), req.GetFallback(), "update", args,
		map[string]string{"role": req.GetRole(), "expires_at": exp})
}

func (a *sharesAdmin) RemoveShare(ctx context.Context, req *adminpb.RemoveShareRequest) (*adminpb.AdminShare, error) {
	if req.GetId() == "" {
		return nil, shareArgRequired("id")
	}
	args := shareArgs(map[string]string{"id": req.GetId(), "db_only": shareBool(req.GetDbOnly()), "fallback": shareBool(req.GetFallback())})
	return a.shareMutation(ctx, "remove_share", req.GetId(), req.GetFallback(), "remove", args,
		map[string]string{"db_only": shareBool(req.GetDbOnly())})
}

func (a *sharesAdmin) SyncShareExpiration(ctx context.Context, req *adminpb.SyncShareExpirationRequest) (*adminpb.AdminShare, error) {
	if req.GetId() == "" {
		return nil, shareArgRequired("id")
	}
	return a.shareMutation(ctx, "sync_share_expiration", req.GetId(), false, "sync_expiry", map[string]string{"id": req.GetId()}, nil)
}

func (a *sharesAdmin) TransferShare(ctx context.Context, req *adminpb.TransferShareRequest) (*adminpb.AdminShare, error) {
	if req.GetId() == "" || req.GetInitiator() == "" {
		return nil, shareArgRequired("id and initiator")
	}
	return a.shareMutation(ctx, "transfer_share", req.GetId(), false, "transfer",
		map[string]string{"id": req.GetId(), "initiator": req.GetInitiator()}, map[string]string{"to": req.GetInitiator()})
}

func (a *sharesAdmin) MarkShareOrphan(ctx context.Context, req *adminpb.MarkShareOrphanRequest) (*adminpb.AdminShare, error) {
	if req.GetId() == "" {
		return nil, shareArgRequired("id")
	}
	return a.shareMutation(ctx, "mark_share_orphan", req.GetId(), false, "mark_orphan", map[string]string{"id": req.GetId()}, nil)
}

// shareMutation runs a mutating share invocation and audits it.
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
	spaceID, err := a.s.shareSpaceID(ctx, req.GetSpace())
	if err != nil {
		return err
	}
	args := shareArgs(map[string]string{
		"id": req.GetRef().GetId(), "token": req.GetRef().GetToken(), "owner": req.GetOwner(), "initiator": req.GetInitiator(),
		"space": spaceID, "resolve_paths": shareBool(req.GetResolvePaths()),
	})
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
	exp, err := expiresArg(req.GetExpiration(), req.GetRemoveExpiration())
	if err != nil {
		return nil, err
	}
	maps.Copy(args, shareArgs(map[string]string{"role": req.GetRole(), "expires_at": exp, "fallback": shareBool(req.GetFallback())}))
	// The audit tells only whether the password changed.
	fields := map[string]string{"role": req.GetRole(), "expires_at": exp}
	if req.Password != nil {
		args["password"] = req.GetPassword()
		fields["password"] = "set"
		if req.GetPassword() == "" {
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
	if req.GetDbOnly() {
		args["db_only"] = "true"
	}
	if req.GetFallback() {
		args["fallback"] = "true"
	}
	return a.linkMutation(ctx, "remove_public_link", req.GetRef(), req.GetFallback(), "remove", args,
		map[string]string{"db_only": shareBool(req.GetDbOnly())})
}

func (a *sharesAdmin) TransferPublicLink(ctx context.Context, req *adminpb.TransferPublicLinkRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	if req.GetInitiator() == "" {
		return nil, shareArgRequired("initiator")
	}
	args["initiator"] = req.GetInitiator()
	return a.linkMutation(ctx, "transfer_public_link", req.GetRef(), false, "transfer", args, map[string]string{"to": req.GetInitiator()})
}

func (a *sharesAdmin) MarkPublicLinkOrphan(ctx context.Context, req *adminpb.MarkPublicLinkOrphanRequest) (*adminpb.AdminPublicShare, error) {
	args, err := linkArgs(req.GetRef())
	if err != nil {
		return nil, err
	}
	return a.linkMutation(ctx, "mark_public_link_orphan", req.GetRef(), false, "mark_orphan", args, nil)
}

// linkMutation runs a mutating link invocation and audits it.
func (a *sharesAdmin) linkMutation(ctx context.Context, action string, ref *adminpb.PublicLinkRef, fallback bool, invocation string, args, fields map[string]string) (*adminpb.AdminPublicShare, error) {
	out := &adminpb.AdminPublicShare{}
	err := a.s.invokeDecoded(ctx, publicShareService, invocation, args, out)
	// The token of a link opens it, so the audit names the link by id only.
	target := "link:" + ref.GetId()
	if id := out.GetPublicShare().GetId().GetOpaqueId(); id != "" {
		target = "link:" + id
	} else if ref.GetId() == "" {
		target = "link:<token>"
	}
	auditShares(ctx, action, target, fallback, err, fields)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MoveShares finds its candidates with the list invocations, because the
// share tables keep the resource id and not the path. It moves each one with
// the move invocation, which resolves the new path as a user.
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
	case req.GetOwner() == "" && req.GetSpace() == nil:
		return shareArgRequired("owner or space")
	}
	spaceID, err := a.s.shareSpaceID(ctx, req.GetSpace())
	if err != nil {
		return err
	}
	listArgs := shareArgs(map[string]string{"owner": req.GetOwner(), "space": spaceID, "resolve_paths": "true"})
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
	shareListArgs := shareArgs(map[string]string{"as": req.GetAs()})
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

	// One failure does not stop the others: a half-done move is easier to
	// finish when the operator sees every item that failed.
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

// moveOne moves one item of MoveShares, and puts the moved share or link in
// it.
func (a *sharesAdmin) moveOne(ctx context.Context, it *adminpb.MovedShare, req *adminpb.MoveSharesRequest) error {
	args := shareArgs(map[string]string{"path": it.GetToPath(), "as": req.GetAs()})
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

// invokeDecoded runs a single-target invocation of service and decodes its
// result into m.
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

// shareSpaceID returns the space id that the share tables keep for ref: the
// part after the storage id.
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
	if _, spaceID, ok := strings.Cut(id, "$"); ok {
		return spaceID, nil
	}
	return id, nil
}

// addSelection sets the list arguments of the share invocations for the
// selections. keepSelected applies the ONLY selections, which the invocations
// do not have.
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

// expiresArg returns the expires_at argument of the update invocations.
func expiresArg(exp *typespb.Timestamp, remove bool) (string, error) {
	switch {
	case exp != nil && remove:
		return "", status.Error(codes.InvalidArgument, "admin: give expiration or remove_expiration, not both")
	case remove:
		return "none", nil
	case exp != nil:
		return time.Unix(int64(exp.GetSeconds()), 0).UTC().Format(time.RFC3339), nil
	}
	return "", nil
}

func linkArgs(ref *adminpb.PublicLinkRef) (map[string]string, error) {
	switch r := ref.GetRef().(type) {
	case *adminpb.PublicLinkRef_Id:
		if r.Id != "" {
			return map[string]string{"id": r.Id}, nil
		}
	case *adminpb.PublicLinkRef_Token:
		if r.Token != "" {
			return map[string]string{"token": r.Token}, nil
		}
	}
	return nil, shareArgRequired("the id or the token of the link")
}

// auditShares records a mutation that the admin service asked for. The
// owning service audits it too, with the user it acted as.
func auditShares(ctx context.Context, action, target string, fallback bool, err error, fields map[string]string) {
	out := shareArgs(fields)
	if fallback {
		out["fallback"] = "true"
	}
	admin.Audit(ctx, admin.AuditEvent{Action: action, Actor: actorName(ctx), Target: target, Granted: err == nil, Err: err, Fields: out})
}

// shareArgs drops the empty values, so that the invocation applies its
// defaults.
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
