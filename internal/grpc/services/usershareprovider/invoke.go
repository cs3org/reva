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

package usershareprovider

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/reconciliation"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
)

// adminManager is what the admin invocations need from the share manager, in
// addition to share.Manager. Only the sql driver has it.
type adminManager interface {
	reconciliation.ExpiredShareStore
	ListModelShares(u *userpb.User, filters []*collaboration.Filter, hideOrphans bool) ([]model.Share, error)
	GetShareUnfiltered(ctx context.Context, ref *collaboration.ShareReference) (*model.Share, error)
	TransferShare(ctx context.Context, ref *collaboration.ShareReference, newInitiator string) error
	MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error
	MoveShare(ctx context.Context, ref *collaboration.ShareReference, newLocation *provider.ResourceId, newOwner string) error
}

const (
	// listChunk is the number of shares whose paths are resolved before they
	// are sent, so that a long list starts to arrive early.
	listChunk      = 100
	resolveWorkers = 8
)

type listArgs struct {
	Owner          string `arg:"owner" help:"shares owned by this user"`
	Initiator      string `arg:"initiator" help:"shares created by this user"`
	Sharee         string `arg:"sharee" help:"shares received by this user or group"`
	ID             string `arg:"id" help:"one share, by id; the other selectors are ignored"`
	Space          string `arg:"space" help:"shares in this space, by space id"`
	Path           string `arg:"path" help:"shares of this path, which is resolved as the user in as, owner or initiator"`
	As             string `arg:"as" help:"the user to resolve path and the share paths as; by default the initiator, then the owner of each share"`
	IncludeOrphans bool   `arg:"include_orphans" help:"include orphaned shares"`
	IncludeExpired *bool  `arg:"include_expired" help:"include expired shares (default true)"`
	ResolvePaths   bool   `arg:"resolve_paths" help:"add the current path of each share"`
}

type idArgs struct {
	ID string `arg:"id,required" help:"the share id"`
}

type updateArgs struct {
	ID        string `arg:"id,required" help:"the share id"`
	Role      string `arg:"role" help:"the new role, for example viewer or editor"`
	ExpiresAt string `arg:"expires_at" help:"the new expiration: a date (2006-01-02), an RFC 3339 time, or none"`
	Fallback  bool   `arg:"fallback" help:"the caller fell back to admin rights after a user command, for the audit"`
}

type removeArgs struct {
	ID       string `arg:"id,required" help:"the share id"`
	DBOnly   bool   `arg:"db_only" help:"delete the record only and keep the storage grant"`
	Fallback bool   `arg:"fallback" help:"the caller fell back to admin rights after a user command, for the audit"`
}

type transferArgs struct {
	ID        string `arg:"id,required" help:"the share id"`
	Initiator string `arg:"initiator,required" help:"the new initiator, who must be allowed to share the resource"`
}

type moveArgs struct {
	ID         string `arg:"id,required" help:"the share id"`
	Path       string `arg:"path,required" help:"the new path of the shared resource"`
	As         string `arg:"as" help:"the user to resolve path as; by default the owner, then the initiator of the share"`
	ApplyGrant bool   `arg:"apply_grant" help:"also add the grant of the share on the new resource"`
}

func (s *service) registerInvocations() {
	s.Set.Add("list", "Stream the shares of any user. Give at least one of owner, initiator, sharee, id, space or path.").
		Shared().
		HandleStream(invoke.TypedStream(s.list))
	s.Set.Add("get", "Show one share, also when it is orphaned or expired.").
		Shared().
		Handle(invoke.Typed(s.get))
	s.Set.Add("update", "Change the role or the expiration of a share through the gateway, as its initiator or owner.").
		Mutating().Shared().
		Handle(invoke.Typed(s.update))
	s.Set.Add("remove", "Remove a share and its storage grant through the gateway, as its initiator or owner. For an orphan, or with db_only, only the record is deleted.").
		Mutating().Shared().
		Handle(invoke.Typed(s.remove))
	s.Set.Add("sync_expiry", "Remove the storage grant of an expired share now, and keep its record.").
		Mutating().Shared().
		Handle(invoke.Typed(s.syncExpiry))
	s.Set.Add("transfer", "Give a share to a new initiator, who must be allowed to add a grant on the resource.").
		Mutating().Shared().
		Handle(invoke.Typed(s.transfer))
	s.Set.Add("move", "Point a share at a new path after a data move. The owner of the new path becomes the owner of the share.").
		Dangerous().Shared().
		Handle(invoke.Typed(s.move))
	s.Set.Add("mark_orphan", "Mark a share orphaned, which hides it from users and from the reconciliation jobs.").
		Mutating().Shared().
		Handle(invoke.Typed(s.markOrphan))
}

func (s *service) adminManager() (adminManager, error) {
	m, ok := s.sm.(adminManager)
	if !ok {
		return nil, errtypes.NotSupported(fmt.Sprintf("the %s driver does not support admin invocations", s.conf.Driver))
	}
	return m, nil
}

func (s *service) list(ctx context.Context, a listArgs, emit invoke.StreamEmit) error {
	m, err := s.adminManager()
	if err != nil {
		return err
	}
	shares, err := s.selectShares(ctx, m, a)
	if err != nil {
		return err
	}
	if a.IncludeExpired != nil && !*a.IncludeExpired {
		shares = slices.DeleteFunc(shares, func(sh model.Share) bool { return isExpired(&sh) })
	}
	slices.SortFunc(shares, func(x, y model.Share) int { return cmp.Compare(x.Id, y.Id) })

	users := &userContexts{ctx: ctx, ctxs: map[string]context.Context{}}
	for chunk := range slices.Chunk(shares, listChunk) {
		paths := make([]string, len(chunk))
		if a.ResolvePaths {
			var g errgroup.Group
			g.SetLimit(resolveWorkers)
			for i := range chunk {
				g.Go(func() error {
					paths[i] = resolvePath(users, &chunk[i], a.As)
					return nil
				})
			}
			_ = g.Wait()
		}
		for i := range chunk {
			r, err := adminShare(&chunk[i], paths[i])
			if err != nil {
				return err
			}
			if err := emit(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *service) selectShares(ctx context.Context, m adminManager, a listArgs) ([]model.Share, error) {
	if a.ID != "" {
		sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
		if isNotFound(err) || (err == nil && sh.IsOrphan() && !a.IncludeOrphans) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []model.Share{*sh}, nil
	}

	var filters []*collaboration.Filter
	if a.Owner != "" {
		filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_OWNER, Term: &collaboration.Filter_Owner{Owner: &userpb.UserId{OpaqueId: a.Owner}}})
	}
	if a.Initiator != "" {
		filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_CREATOR, Term: &collaboration.Filter_Creator{Creator: &userpb.UserId{OpaqueId: a.Initiator}}})
	}
	if a.Sharee != "" {
		// share_with holds the name of a user or of a group alike.
		filters = append(filters, revashare.GranteeFilter(userGrantee(&userpb.UserId{OpaqueId: a.Sharee})))
	}
	if a.Space != "" {
		filters = append(filters, revashare.SpaceIDFilter(a.Space))
	}
	if a.Path != "" {
		as := nonEmpty(a.As)
		if len(as) == 0 {
			as = nonEmpty(a.Owner, a.Initiator)
		}
		if len(as) == 0 {
			return nil, errtypes.BadRequest("path needs as, owner or initiator: the path is resolved as that user")
		}
		var info *provider.ResourceInfo
		_, err := actAs(ctx, as, func(uctx context.Context) (*rpc.Status, error) {
			res, err := statAs(uctx, &provider.Reference{Path: a.Path})
			info = res.GetInfo()
			return res.GetStatus(), err
		})
		if err != nil {
			return nil, err
		}
		filters = append(filters, revashare.ResourceIDFilter(info.GetId()))
	}
	// The table is too large to read without a filter.
	if len(filters) == 0 {
		return nil, errtypes.BadRequest("list needs at least one of owner, initiator, sharee, id, space or path")
	}
	return m.ListModelShares(nil, filters, !a.IncludeOrphans)
}

func (s *service) get(ctx context.Context, a idArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	users := &userContexts{ctx: ctx, ctxs: map[string]context.Context{}}
	return adminShare(sh, resolvePath(users, sh, ""))
}

func (s *service) update(ctx context.Context, a updateArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	var fields []*collaboration.UpdateShareRequest_UpdateField
	if a.Role != "" {
		role := permissions.RoleFromName(a.Role)
		if role.Name == permissions.RoleUnknown {
			return nil, errtypes.BadRequest(fmt.Sprintf("unknown role %q", a.Role))
		}
		fields = append(fields, &collaboration.UpdateShareRequest_UpdateField{Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
			Permissions: &collaboration.SharePermissions{Permissions: role.CS3ResourcePermissions()},
		}})
	}
	if a.ExpiresAt != "" {
		exp, err := parseExpiry(a.ExpiresAt)
		if err != nil {
			return nil, err
		}
		fields = append(fields, &collaboration.UpdateShareRequest_UpdateField{Field: &collaboration.UpdateShareRequest_UpdateField_Expiration{Expiration: exp}})
	}
	if len(fields) == 0 {
		return nil, errtypes.BadRequest("update needs role or expires_at")
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	if sh.IsOrphan() {
		return nil, errtypes.BadRequest(fmt.Sprintf("share %s is orphaned: its resource is gone", a.ID))
	}

	actor, err := actAs(ctx, shareUsers(sh), func(uctx context.Context) (*rpc.Status, error) {
		gw, err := revaservice.Gateway(uctx)
		if err != nil {
			return nil, err
		}
		for _, f := range fields {
			res, err := gw.UpdateShare(uctx, &collaboration.UpdateShareRequest{Ref: shareRef(a.ID), Field: f})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
				return res.GetStatus(), err
			}
		}
		return status.NewOK(uctx), nil
	})
	audit(ctx, "update", sh, err, a.Fallback, map[string]string{"acted_as": actor, "role": a.Role, "expires_at": a.ExpiresAt})
	if err != nil {
		return nil, fmt.Errorf("updating share %s: %w", a.ID, err)
	}
	if sh, err = m.GetShareUnfiltered(ctx, shareRef(a.ID)); err != nil {
		return nil, err
	}
	return adminShare(sh, "")
}

func (s *service) remove(ctx context.Context, a removeArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	var actor string
	// An orphan has no resource left, so the gateway cannot remove it.
	if a.DBOnly || sh.IsOrphan() {
		err = s.sm.Unshare(ctx, shareRef(a.ID))
	} else {
		actor, err = actAs(ctx, shareUsers(sh), func(uctx context.Context) (*rpc.Status, error) {
			gw, err := revaservice.Gateway(uctx)
			if err != nil {
				return nil, err
			}
			res, err := gw.RemoveShare(uctx, &collaboration.RemoveShareRequest{Ref: shareRef(a.ID)})
			return res.GetStatus(), err
		})
	}
	audit(ctx, "remove", sh, err, a.Fallback, map[string]string{"acted_as": actor, "db_only": strconv.FormatBool(a.DBOnly || sh.IsOrphan())})
	if err != nil {
		return nil, fmt.Errorf("removing share %s: %w", a.ID, err)
	}
	return adminShare(sh, "")
}

func (s *service) syncExpiry(ctx context.Context, a idArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	if sh.IsOrphan() {
		return nil, errtypes.BadRequest(fmt.Sprintf("share %s is orphaned: its resource is gone", a.ID))
	}
	if !isExpired(sh) {
		return nil, errtypes.BadRequest(fmt.Sprintf("share %s is not expired", a.ID))
	}
	job := &reconciliation.ExpiredJob{
		ShareStore: m,
		Grants: func(ctx context.Context, storageID string) (reconciliation.ExpiredGrantStore, error) {
			return s.grantsAt(ctx, storageID)
		},
	}
	// The job reports a refusal as an error, not as a status, so only the
	// first user that can be looked up is tried.
	var actor string
	err = errtypes.BadRequest("no user to act as")
	for _, u := range shareUsers(sh) {
		uctx, _, lerr := admin.ActAs(ctx, u)
		if lerr != nil {
			err = lerr
			continue
		}
		actor, err = u, job.SyncShare(uctx, sh.AsCS3Share(granteeOf(sh)))
		break
	}
	audit(ctx, "sync_expiry", sh, err, false, map[string]string{"acted_as": actor})
	if err != nil {
		return nil, fmt.Errorf("syncing share %s: %w", a.ID, err)
	}
	if sh, err = m.GetShareUnfiltered(ctx, shareRef(a.ID)); err != nil {
		return nil, err
	}
	return adminShare(sh, "")
}

func (s *service) transfer(ctx context.Context, a transferArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	// The storage decides who may hold a share: the new initiator must be
	// allowed to add a grant on the resource.
	uctx, u, err := admin.ActAs(ctx, a.Initiator)
	if err == nil {
		var res *provider.StatResponse
		res, err = statAs(uctx, &provider.Reference{ResourceId: resourceOf(sh)})
		switch {
		case err != nil:
		case res.GetStatus().GetCode() != rpc.Code_CODE_OK:
			err = status.NewErrtypeFromStatus(res.GetStatus())
		case !res.GetInfo().GetPermissionSet().GetAddGrant():
			err = errtypes.PermissionDenied(a.Initiator + " is not allowed to share the resource")
		}
	}
	if err == nil {
		err = m.TransferShare(ctx, shareRef(a.ID), u.GetId().GetOpaqueId())
	}
	audit(ctx, "transfer", sh, err, false, map[string]string{"from": sh.UIDInitiator, "to": a.Initiator})
	if err != nil {
		return nil, fmt.Errorf("transferring share %s: %w", a.ID, err)
	}
	if sh, err = m.GetShareUnfiltered(ctx, shareRef(a.ID)); err != nil {
		return nil, err
	}
	return adminShare(sh, "")
}

func (s *service) move(ctx context.Context, a moveArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	as := nonEmpty(a.As)
	if len(as) == 0 {
		as = nonEmpty(sh.UIDOwner, sh.UIDInitiator)
	}
	var info *provider.ResourceInfo
	var actor string
	actor, err = actAs(ctx, as, func(uctx context.Context) (*rpc.Status, error) {
		res, err := statAs(uctx, &provider.Reference{Path: a.Path})
		info = res.GetInfo()
		return res.GetStatus(), err
	})
	owner := info.GetOwner().GetOpaqueId()
	if err == nil && owner == "" {
		err = fmt.Errorf("%s has no owner", a.Path)
	}
	if err == nil {
		err = m.MoveShare(ctx, shareRef(a.ID), info.GetId(), owner)
	}
	if err == nil && a.ApplyGrant {
		err = s.applyGrant(ctx, actor, info.GetId(), sh)
	}
	audit(ctx, "move", sh, err, false, map[string]string{"acted_as": actor, "path": a.Path, "new_owner": owner, "apply_grant": strconv.FormatBool(a.ApplyGrant)})
	if err != nil {
		return nil, fmt.Errorf("moving share %s: %w", a.ID, err)
	}
	if sh, err = m.GetShareUnfiltered(ctx, shareRef(a.ID)); err != nil {
		return nil, err
	}
	return adminShare(sh, info.GetPath())
}

// applyGrant adds the grant of sh on id as username, on the storage provider
// directly, as the gateway does: the gateway has no grant API.
func (s *service) applyGrant(ctx context.Context, username string, id *provider.ResourceId, sh *model.Share) error {
	uctx, _, err := admin.ActAs(ctx, username)
	if err != nil {
		return err
	}
	grantee := granteeOf(sh)
	if !sh.SharedWithIsGroup {
		// The storage needs the type of the user, for example lightweight.
		u, err := admin.Lookup(uctx, sh.ShareWith)
		if err != nil {
			return err
		}
		grantee = userGrantee(u.GetId())
	}
	c, err := s.grantsAt(uctx, id.GetStorageId())
	if err != nil {
		return err
	}
	res, err := c.AddGrant(uctx, &provider.AddGrantRequest{
		Ref:   &provider.Reference{ResourceId: id},
		Grant: &provider.Grant{Grantee: grantee, Permissions: permissions.OcsPermissions(sh.Permissions).AsCS3Permissions()},
	})
	if err != nil {
		return err
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return status.NewErrtypeFromStatus(res.GetStatus())
	}
	return nil
}

func (s *service) markOrphan(ctx context.Context, a idArgs) (invoke.Result, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.ID))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.ID, err)
	}
	err = m.MarkAsOrphaned(ctx, shareRef(a.ID))
	audit(ctx, "mark_orphan", sh, err, false, map[string]string{})
	if err != nil {
		return nil, fmt.Errorf("marking share %s orphaned: %w", a.ID, err)
	}
	if sh, err = m.GetShareUnfiltered(ctx, shareRef(a.ID)); err != nil {
		return nil, err
	}
	return adminShare(sh, "")
}

// actAs calls fn as each user in turn, until fn returns an OK status. A user
// that cannot be looked up, or that the call refuses with not found or
// permission denied, is skipped: the initiator can have lost the right on the
// resource, and the owner can be a deleted account. actAs returns the user
// that fn was called as for the last time.
func actAs(ctx context.Context, users []string, fn func(uctx context.Context) (*rpc.Status, error)) (string, error) {
	var err error = errtypes.BadRequest("no user to act as")
	for _, u := range users {
		uctx, _, lerr := admin.ActAs(ctx, u)
		if lerr != nil {
			err = lerr
			continue
		}
		st, cerr := fn(uctx)
		if cerr != nil {
			return u, cerr
		}
		switch st.GetCode() {
		case rpc.Code_CODE_OK:
			return u, nil
		case rpc.Code_CODE_NOT_FOUND, rpc.Code_CODE_PERMISSION_DENIED:
			err = fmt.Errorf("as %s: %w", u, status.NewErrtypeFromStatus(st))
		default:
			return u, fmt.Errorf("as %s: %w", u, status.NewErrtypeFromStatus(st))
		}
	}
	return "", err
}

func statAs(uctx context.Context, ref *provider.Reference) (*provider.StatResponse, error) {
	gw, err := revaservice.Gateway(uctx)
	if err != nil {
		return nil, err
	}
	return gw.Stat(uctx, &provider.StatRequest{Ref: ref})
}

// userContexts keeps one context per user for the length of one invocation,
// because a list often acts as the same few users many times.
type userContexts struct {
	ctx  context.Context
	mu   sync.Mutex
	ctxs map[string]context.Context
}

// as returns the context acting as username, or nil if that user cannot be
// looked up.
func (c *userContexts) as(username string) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if uctx, ok := c.ctxs[username]; ok {
		return uctx
	}
	uctx, _, err := admin.ActAs(c.ctx, username)
	if err != nil {
		uctx = nil
	}
	c.ctxs[username] = uctx
	return uctx
}

// resolvePath returns the current path of the resource of sh, or "" if no
// user can resolve it.
func resolvePath(users *userContexts, sh *model.Share, as string) string {
	candidates := nonEmpty(as)
	if len(candidates) == 0 {
		candidates = shareUsers(sh)
	}
	for _, u := range candidates {
		uctx := users.as(u)
		if uctx == nil {
			continue
		}
		gw, err := revaservice.Gateway(uctx)
		if err != nil {
			return ""
		}
		res, err := gw.GetPath(uctx, &provider.GetPathRequest{ResourceId: resourceOf(sh)})
		if err == nil && res.GetStatus().GetCode() == rpc.Code_CODE_OK {
			return res.GetPath()
		}
	}
	return ""
}

// grantsAtStorage returns the grant API of the storage provider of storageID.
func grantsAtStorage(ctx context.Context, storageID string) (provider.ProviderAPIClient, error) {
	reg, err := revaservice.StorageRegistry(ctx)
	if err != nil {
		return nil, err
	}
	res, err := reg.GetStorageProviders(ctx, &registrypb.GetStorageProvidersRequest{
		Ref: &provider.Reference{ResourceId: &provider.ResourceId{StorageId: storageID}},
	})
	if err != nil {
		return nil, fmt.Errorf("looking up the storage provider of %q: %w", storageID, err)
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("looking up the storage provider of %q: %w", storageID, status.NewErrtypeFromStatus(res.GetStatus()))
	}
	if len(res.GetProviders()) == 0 {
		return nil, errtypes.NotFound("storage provider of " + storageID)
	}
	return revaservice.StorageProviderAt(res.GetProviders()[0].GetAddress())
}

// audit records a mutation of sh. fallback tells that a user command fell back
// to admin rights on a share of another user.
func audit(ctx context.Context, action string, sh *model.Share, err error, fallback bool, fields map[string]string) {
	maps.DeleteFunc(fields, func(_, v string) bool { return v == "" })
	fields["owner"] = sh.UIDOwner
	fields["sharee"] = sh.ShareWith
	if fallback {
		fields["fallback"] = "true"
	}
	u, _ := appctx.ContextGetUser(ctx)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "usershareprovider." + action,
		Actor:   u.GetUsername(),
		Target:  "share:" + strconv.FormatUint(uint64(sh.Id), 10),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

// adminShare returns sh as the protojson of the AdminShare message of the
// typed Admin API: the CS3 share, and what CS3 cannot carry.
func adminShare(sh *model.Share, path string) (invoke.Result, error) {
	b, err := protojson.Marshal(sh.AsCS3Share(granteeOf(sh)))
	if err != nil {
		return nil, err
	}
	var share map[string]any
	if err := json.Unmarshal(b, &share); err != nil {
		return nil, err
	}
	r := invoke.Result{"share": share}
	if sh.IsOrphan() {
		r["orphan"] = true
	}
	if path != "" {
		r["path"] = path
	}
	extra := map[string]string{}
	if sh.InitialPath != "" {
		extra["initial_path"] = sh.InitialPath
	}
	if sh.OrphanedAt.Valid {
		extra["orphaned_at"] = sh.OrphanedAt.V.UTC().Format(time.RFC3339)
	}
	if sh.ExpiredAt.Valid {
		extra["expired_at"] = sh.ExpiredAt.V.UTC().Format(time.RFC3339)
	}
	if len(extra) > 0 {
		r["extra"] = extra
	}
	return r, nil
}

// parseExpiry parses an expiration: a date at midnight local time, an RFC 3339
// time, or none, which the share manager reads as no expiration.
func parseExpiry(v string) (*typespb.Timestamp, error) {
	if v == "none" {
		return &typespb.Timestamp{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		if t, err = time.ParseInLocation(time.DateOnly, v, time.Local); err != nil {
			return nil, errtypes.BadRequest(fmt.Sprintf("expires_at %q is not a date (2006-01-02), an RFC 3339 time or none", v))
		}
	}
	return &typespb.Timestamp{Seconds: uint64(t.Unix())}, nil
}

func isNotFound(err error) bool {
	var nf errtypes.IsNotFound
	return errors.As(err, &nf)
}

func isExpired(sh *model.Share) bool {
	return sh.Expiration.Valid && !sh.Expiration.V.After(time.Now())
}

// shareUsers returns the users a share is acted on as, in order: the initiator
// manages the share, and on a project the owner is a service account.
func shareUsers(sh *model.Share) []string {
	return slices.Compact(nonEmpty(sh.UIDInitiator, sh.UIDOwner))
}

func nonEmpty(vs ...string) []string {
	return slices.DeleteFunc(vs, func(v string) bool { return v == "" })
}

func shareRef(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: id}}}
}

func resourceOf(sh *model.Share) *provider.ResourceId {
	return &provider.ResourceId{StorageId: sh.Instance, OpaqueId: sh.Inode, SpaceId: sh.SpaceID}
}

// granteeOf returns the grantee of sh without its user type, which only a
// lookup can tell.
func granteeOf(sh *model.Share) *provider.Grantee {
	if sh.SharedWithIsGroup {
		return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_GROUP, Id: &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: sh.ShareWith}}}
	}
	return userGrantee(&userpb.UserId{OpaqueId: sh.ShareWith})
}

func userGrantee(id *userpb.UserId) *provider.Grantee {
	return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: id}}
}
