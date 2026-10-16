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
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
)

// The user share provider is an invoke.Invokable: through the Admin API an
// operator can list, create, remove and repair the shares of any user. Every
// mutation is audited, and acts on the storage as the share's initiator or
// owner, through the gateway, so the grants end up exactly as they would for
// a share made from the web interface.

// adminManager is what the invocations need from the share manager beyond the
// CS3 share.Manager. The SQL driver implements it; any other driver makes the
// invocations answer that they are unsupported.
type adminManager interface {
	ListModelShares(u *userpb.User, filters []*collaboration.Filter, hideOrphans bool) ([]model.Share, error)
	GetShareUnfiltered(ctx context.Context, ref *collaboration.ShareReference) (*model.Share, error)
	SetExpiration(ctx context.Context, ref *collaboration.ShareReference, expiration *time.Time) error
	TransferShare(ctx context.Context, ref *collaboration.ShareReference, newInitiator string) error
	MarkAsOrphaned(ctx context.Context, ref *collaboration.ShareReference) error
	MoveShare(ctx context.Context, ref *collaboration.ShareReference, newLocation *provider.ResourceId, newOwner string) error
}

// grantsFunc returns the grant API of the storage provider hosting storageID.
type grantsFunc func(ctx context.Context, storageID string) (provider.ProviderAPIClient, error)

const (
	// maxResolvedPaths bounds resolve_paths: every row costs a gateway
	// round-trip, and an invocation has a few seconds.
	maxResolvedPaths = 500
	// defaultListLimit is the list limit when none is given.
	defaultListLimit = 1000
	// resolveWorkers is the concurrency of path resolution.
	resolveWorkers = 8

	projectPathPrefix = "/eos/project/"
	homePathPrefix    = "/eos/user/"
)

func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("list", "List shares, orphaned and expired ones included on request. At least one selector is required.").
		OptArg("owner", "shares owned by this user").
		OptArg("initiator", "shares created by this user").
		OptArg("sharee", "shares received by this user or group").
		OptArg("id", "one share, by id").
		OptArg("resource", "shares of one resource, as <storage>:<inode>").
		OptArg("space", "shares in a space, by space id").
		OptArg("path", "shares of a path, resolved as the user given in as, or as the owner of a home under "+homePathPrefix).
		OptArg("as", "the user to resolve path and the share paths as").
		OptArg("include_orphans", "include orphaned shares (default false)").
		OptArg("include_expired", "include expired shares (default true)").
		OptArg("resolve_paths", "add the current path of every share (default false, at most "+strconv.Itoa(maxResolvedPaths)+" shares)").
		OptArg("limit", "return at most this many shares (default "+strconv.Itoa(defaultListLimit)+")").
		OptArg("offset", "skip this many shares").
		Handle(s.invokeList)
	s.Set.Add("get", "Show one share, orphaned or expired or not.").
		Arg("id", "the share id").
		OptArg("resolve_path", "add the current path of the share").
		Handle(s.invokeGet)
	s.Set.Add("create", "Share a path with a user or a group as its initiator, applying the storage grant like the web interface does. Address one instance by node id.").
		Mutating().
		Arg("path", "the path to share").
		Arg("initiator", "the user creating the share; must be allowed to share the path").
		Arg("sharee", "the user or group receiving the share").
		Arg("permissions", "viewer (1) or editor (15)").
		OptArg("type", "user or group; a user is assumed unless the name is not a user").
		OptArg("expires_at", "a date or RFC 3339 time").
		OptArg("ignore_existing", "return the existing share instead of failing when it already exists").
		OptArg("force", "delete child shares the new share makes redundant instead of failing").
		Handle(s.invokeCreate)
	s.Set.Add("remove", "Remove a share and its storage grant, as the share's initiator. An orphaned share, or db_only, only deletes the record. Address one instance by node id.").
		Mutating().
		Arg("id", "the share id").
		OptArg("db_only", "delete the record only, leave the storage grant alone").
		Handle(s.invokeRemove)
	s.Set.Add("set_expiry", "Set or clear the expiration of a share. A past date also removes the storage grant unless sync is false. Address one instance by node id.").
		Mutating().
		Arg("id", "the share id").
		Arg("expires_at", "a date, an RFC 3339 time, or none").
		OptArg("sync", "remove the storage grant when the share is expired (default true)").
		Handle(s.invokeSetExpiry)
	s.Set.Add("sync_expiry", "Remove the storage grant of an expired share, keeping its record. Address one instance by node id.").
		Mutating().
		Arg("id", "the share id").
		Handle(s.invokeSyncExpiry)
	s.Set.Add("transfer", "Hand a share on a project over to a new initiator, who must administer the project. Address one instance by node id.").
		Mutating().
		Arg("id", "the share id").
		Arg("initiator", "the new initiator").
		Handle(s.invokeTransfer)
	s.Set.Add("move", "Point a share at another path, updating its owner. The grant on the old path is not touched; apply_grant=true also grants the sharee on the new path. Address one instance by node id.").
		Dangerous().
		Arg("id", "the share id").
		Arg("path", "the new path").
		OptArg("owner", "the owner of the new path; derived for a home under "+homePathPrefix).
		OptArg("apply_grant", "also apply the share's grant on the new path, as its owner (default false)").
		Handle(s.invokeMove)
	s.Set.Add("mark_orphan", "Mark a share orphaned, hiding it from every listing. Address one instance by node id.").
		Mutating().
		Arg("id", "the share id").
		Handle(s.invokeMarkOrphan)
}

// admin returns the manager surface the invocations need.
func (s *service) admin() (adminManager, error) {
	m, ok := s.sm.(adminManager)
	if !ok {
		return nil, fmt.Errorf("usershareprovider: the %s driver does not support admin invocations", s.conf.Driver)
	}
	return m, nil
}

func (s *service) invokeList(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	includeOrphans := a.Bool("include_orphans")
	includeExpired := !a.Has("include_expired") || a.Bool("include_expired")

	var shares []model.Share
	var filters []*collaboration.Filter
	selected := false
	if id := a.String("id"); id != "" {
		sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
		if err != nil {
			return nil, fmt.Errorf("share %s: %w", id, err)
		}
		shares = []model.Share{*sh}
		selected = true
	} else {
		if owner := a.String("owner"); owner != "" {
			filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_OWNER, Term: &collaboration.Filter_Owner{Owner: &userpb.UserId{OpaqueId: owner}}})
			selected = true
		}
		if initiator := a.String("initiator"); initiator != "" {
			filters = append(filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_CREATOR, Term: &collaboration.Filter_Creator{Creator: &userpb.UserId{OpaqueId: initiator}}})
			selected = true
		}
		if sharee := a.String("sharee"); sharee != "" {
			// the share_with column holds the name of a user or a group alike
			filters = append(filters, revashare.GranteeFilter(userGrantee(&userpb.UserId{OpaqueId: sharee})))
			selected = true
		}
		if space := a.String("space"); space != "" {
			filters = append(filters, revashare.SpaceIDFilter(space))
			selected = true
		}
		if res := a.String("resource"); res != "" {
			id, err := parseResource(res)
			if err != nil {
				return nil, err
			}
			filters = append(filters, revashare.ResourceIDFilter(id))
			selected = true
		}
		if path := a.String("path"); path != "" {
			as := firstNonEmpty(a.String("as"), a.String("owner"), a.String("initiator"), homeOwner(path))
			if as == "" {
				return nil, fmt.Errorf("resolving %q needs a user: pass as=<user>", path)
			}
			uctx, _, err := s.identity.ActAs(ctx, as)
			if err != nil {
				return nil, err
			}
			info, err := statPath(uctx, path)
			if err != nil {
				return nil, err
			}
			filters = append(filters, revashare.ResourceIDFilter(info.GetId()))
			selected = true
		}
		if !selected {
			return nil, errors.New("list needs at least one of owner, initiator, sharee, id, resource, space or path")
		}
		if shares, err = m.ListModelShares(nil, filters, !includeOrphans); err != nil {
			return nil, err
		}
	}

	if !includeExpired {
		shares = slices.DeleteFunc(shares, func(sh model.Share) bool { return isExpired(sh) })
	}
	if !includeOrphans {
		shares = slices.DeleteFunc(shares, func(sh model.Share) bool { return sh.IsOrphan() })
	}
	slices.SortFunc(shares, func(x, y model.Share) int { return int(x.Id) - int(y.Id) })

	total := len(shares)
	offset := max(a.Int("offset"), 0)
	limit := defaultListLimit
	if a.Has("limit") {
		limit = a.Int("limit")
	}
	if offset > len(shares) {
		offset = len(shares)
	}
	shares = shares[offset:]
	truncated := false
	if limit > 0 && len(shares) > limit {
		shares = shares[:limit]
		truncated = true
	}

	var paths map[uint]string
	if a.Bool("resolve_paths") {
		if len(shares) > maxResolvedPaths {
			return nil, fmt.Errorf("resolve_paths is limited to %d shares, narrow the selection or pass limit", maxResolvedPaths)
		}
		paths = s.resolvePaths(ctx, shares, a.String("as"))
	}
	out := make([]invoke.Result, 0, len(shares))
	for _, sh := range shares {
		r := shareResult(&sh)
		if p, ok := paths[sh.Id]; ok {
			r["path"] = p
		}
		out = append(out, r)
	}
	return invoke.Result{"shares": out, "count": len(out), "total": total, "truncated": truncated}, nil
}

func (s *service) invokeGet(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(a.String("id")))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", a.String("id"), err)
	}
	r := shareResult(sh)
	if a.Bool("resolve_path") {
		if p, ok := s.resolvePaths(ctx, []model.Share{*sh}, "")[sh.Id]; ok {
			r["path"] = p
		}
	}
	return r, nil
}

func (s *service) invokeCreate(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	perms, err := sharePermissions(a.String("permissions"))
	if err != nil {
		return nil, err
	}
	expiration, err := expirationArg(a.String("expires_at"))
	if err != nil {
		return nil, err
	}
	initiator := a.String("initiator")
	uctx, _, err := s.identity.ActAs(ctx, initiator)
	if err != nil {
		return nil, err
	}
	info, err := statPath(uctx, a.String("path"))
	if err != nil {
		return nil, err
	}
	grantee, err := s.grantee(ctx, a.String("sharee"), a.String("type"))
	if err != nil {
		return nil, err
	}

	// an existing share is reported rather than duplicated
	existing, err := m.ListModelShares(nil, []*collaboration.Filter{revashare.ResourceIDFilter(info.GetId()), revashare.GranteeFilter(grantee)}, true)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		if a.Bool("ignore_existing") {
			r := shareResult(&existing[0])
			r["created"] = false
			return r, nil
		}
		return nil, fmt.Errorf("%s is already shared with %s: share %d", info.GetPath(), a.String("sharee"), existing[0].Id)
	}

	req := &collaboration.CreateShareRequest{
		ResourceInfo: info,
		Grant: &collaboration.ShareGrant{
			Grantee:     grantee,
			Permissions: &collaboration.SharePermissions{Permissions: perms.AsCS3Permissions()},
			Expiration:  expiration,
		},
	}
	if a.Bool("force") {
		req.Opaque = &typespb.Opaque{Map: map[string]*typespb.OpaqueEntry{"force": {Decoder: "plain", Value: []byte("true")}}}
	}
	gw, err := revaservice.Gateway(uctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.CreateShare(uctx, req)
	if err == nil && res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		err = fmt.Errorf("creating the share: %s: %s", res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	audit(ctx, "create", initiator, err, map[string]string{"path": info.GetPath(), "sharee": a.String("sharee"), "permissions": strconv.Itoa(int(perms))})
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(res.GetShare().GetId().GetOpaqueId()))
	if err != nil {
		return nil, err
	}
	r := shareResult(sh)
	r["created"] = true
	r["path"] = info.GetPath()
	return r, nil
}

func (s *service) invokeRemove(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id := a.String("id")
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	grantRemoved := false
	if a.Bool("db_only") || sh.IsOrphan() {
		err = s.sm.Unshare(ctx, shareRef(id))
	} else {
		err = s.removeThroughGateway(ctx, sh)
		grantRemoved = err == nil
	}
	audit(ctx, "remove", sh.UIDOwner, err, map[string]string{"share": id, "sharee": sh.ShareWith, "grant_removed": strconv.FormatBool(grantRemoved)})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"removed": id, "grant_removed": grantRemoved}, nil
}

func (s *service) invokeSetExpiry(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id := a.String("id")
	t, none, err := admin.ParseTime(a.String("expires_at"))
	if err != nil {
		return nil, err
	}
	var expiration *time.Time
	if !none {
		expiration = &t
	}
	err = m.SetExpiration(ctx, shareRef(id), expiration)
	audit(ctx, "set_expiry", "", err, map[string]string{"share": id, "expires_at": a.String("expires_at")})
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, err
	}
	r := shareResult(sh)
	sync := !a.Has("sync") || a.Bool("sync")
	if sync && isExpired(*sh) && !sh.IsOrphan() {
		if err := s.removeExpiredGrant(ctx, sh); err != nil {
			return nil, fmt.Errorf("the expiration is set, but removing the storage grant failed: %w", err)
		}
		r["grant_removed"] = true
	}
	return r, nil
}

func (s *service) invokeSyncExpiry(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id := a.String("id")
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	if !isExpired(*sh) {
		return nil, fmt.Errorf("share %s is not expired", id)
	}
	if sh.IsOrphan() {
		return nil, fmt.Errorf("share %s is orphaned, its resource is gone", id)
	}
	if err := s.removeExpiredGrant(ctx, sh); err != nil {
		return nil, err
	}
	return invoke.Result{"synced": id, "grant_removed": true}, nil
}

// removeExpiredGrant removes the storage grant of an expired share, as the
// share's initiator, addressing the storage provider the way the gateway does.
// The hierarchy of the grantee's other shares is left alone: the share is
// expired, not removed, and the record stays as it is.
func (s *service) removeExpiredGrant(ctx context.Context, sh *model.Share) error {
	uctx, _, err := s.identity.ActAs(ctx, actorOf(sh))
	if err != nil {
		return err
	}
	grantee, err := s.granteeOf(uctx, sh)
	if err != nil {
		return err
	}
	client, err := s.grantsAt(uctx, sh.Instance)
	if err != nil {
		return err
	}
	res, err := client.RemoveGrant(uctx, &provider.RemoveGrantRequest{
		Ref:   &provider.Reference{ResourceId: resourceOf(sh)},
		Grant: &provider.Grant{Grantee: grantee, Permissions: permissions.OcsPermissions(sh.Permissions).AsCS3Permissions()},
	})
	if err == nil && res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		err = fmt.Errorf("%s: %s", res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	audit(ctx, "sync_expiry", sh.UIDOwner, err, map[string]string{"share": strconv.Itoa(int(sh.Id)), "sharee": sh.ShareWith})
	if err != nil {
		return fmt.Errorf("removing the storage grant of share %d: %w", sh.Id, err)
	}
	return nil
}

func (s *service) invokeTransfer(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id, initiator := a.String("id"), a.String("initiator")
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	project, ok := projectOf(sh.InitialPath)
	if !ok {
		return nil, fmt.Errorf("share %s is not on a project: only shares on projects can be transferred", id)
	}
	u, err := admin.Lookup(ctx, initiator)
	if err != nil {
		return nil, err
	}
	if group := projectAdminGroup(project); !slices.Contains(u.GetGroups(), group) {
		return nil, fmt.Errorf("%s is not in %s: only administrators of the project can hold its shares", initiator, group)
	}
	err = m.TransferShare(ctx, shareRef(id), initiator)
	audit(ctx, "transfer", initiator, err, map[string]string{"share": id, "from": sh.UIDInitiator})
	if err != nil {
		return nil, err
	}
	sh, err = m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, err
	}
	return shareResult(sh), nil
}

func (s *service) invokeMove(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id, path := a.String("id"), a.String("path")
	owner := firstNonEmpty(a.String("owner"), homeOwner(path))
	if owner == "" {
		return nil, fmt.Errorf("the owner of %q cannot be derived: pass owner=<user>", path)
	}
	if _, err := m.GetShareUnfiltered(ctx, shareRef(id)); err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	uctx, _, err := s.identity.ActAs(ctx, owner)
	if err != nil {
		return nil, err
	}
	info, err := statPath(uctx, path)
	if err != nil {
		return nil, err
	}
	err = m.MoveShare(ctx, shareRef(id), info.GetId(), owner)
	audit(ctx, "move", owner, err, map[string]string{"share": id, "path": info.GetPath(), "apply_grant": strconv.FormatBool(a.Bool("apply_grant"))})
	if err != nil {
		return nil, err
	}
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, err
	}
	r := shareResult(sh)
	r["path"] = info.GetPath()
	if a.Bool("apply_grant") {
		if err := s.applyGrant(uctx, sh); err != nil {
			return nil, fmt.Errorf("the share now points at %s, but granting %s there failed: %w", info.GetPath(), sh.ShareWith, err)
		}
		r["grant_applied"] = true
	}
	return r, nil
}

// applyGrant adds the storage grant of a share on its (new) resource, as the
// user of uctx, addressing the storage provider the way the gateway does.
func (s *service) applyGrant(uctx context.Context, sh *model.Share) error {
	grantee, err := s.granteeOf(uctx, sh)
	if err != nil {
		return err
	}
	client, err := s.grantsAt(uctx, sh.Instance)
	if err != nil {
		return err
	}
	res, err := client.AddGrant(uctx, &provider.AddGrantRequest{
		Ref:   &provider.Reference{ResourceId: resourceOf(sh)},
		Grant: &provider.Grant{Grantee: grantee, Permissions: permissions.OcsPermissions(sh.Permissions).AsCS3Permissions()},
	})
	if err == nil && res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		err = fmt.Errorf("%s: %s", res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	return err
}

func (s *service) invokeMarkOrphan(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	id := a.String("id")
	sh, err := m.GetShareUnfiltered(ctx, shareRef(id))
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", id, err)
	}
	err = m.MarkAsOrphaned(ctx, shareRef(id))
	audit(ctx, "mark_orphan", sh.UIDOwner, err, map[string]string{"share": id})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"orphaned": id}, nil
}

// resolvePaths returns the current path of each share, looked up as the
// share's initiator (or as `as` when given), a few at a time. A share whose
// path cannot be resolved is left out.
func (s *service) resolvePaths(ctx context.Context, shares []model.Share, as string) map[uint]string {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		paths = make(map[uint]string, len(shares))
		ctxs  = map[string]context.Context{}
		sem   = make(chan struct{}, resolveWorkers)
	)
	userCtx := func(username string) (context.Context, error) {
		mu.Lock()
		defer mu.Unlock()
		if c, ok := ctxs[username]; ok {
			return c, nil
		}
		c, _, err := s.identity.ActAs(ctx, username)
		if err != nil {
			return nil, err
		}
		ctxs[username] = c
		return c, nil
	}
	for _, sh := range shares {
		wg.Add(1)
		sem <- struct{}{}
		go func(sh model.Share) {
			defer wg.Done()
			defer func() { <-sem }()
			uctx, err := userCtx(firstNonEmpty(as, actorOf(&sh)))
			if err != nil {
				return
			}
			gw, err := revaservice.Gateway(uctx)
			if err != nil {
				return
			}
			res, err := gw.GetPath(uctx, &provider.GetPathRequest{ResourceId: resourceOf(&sh)})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
				return
			}
			mu.Lock()
			paths[sh.Id] = res.GetPath()
			mu.Unlock()
		}(sh)
	}
	wg.Wait()
	return paths
}

// grantee builds the grantee of a share from a name: a user, looked up so
// that its type (primary, lightweight...) is right, or a group.
func (s *service) grantee(ctx context.Context, name, kind string) (*provider.Grantee, error) {
	switch strings.ToLower(kind) {
	case "group":
		return groupGrantee(name), nil
	case "", "user":
		u, err := admin.Lookup(ctx, name)
		if err != nil {
			if kind == "" {
				return nil, fmt.Errorf("%w (pass type=group to share with a group)", err)
			}
			return nil, err
		}
		return userGrantee(u.GetId()), nil
	default:
		return nil, fmt.Errorf("type must be user or group, not %q", kind)
	}
}

// granteeOf rebuilds the grantee of a stored share, looking the user up so
// that a lightweight account is handled as such by the storage.
func (s *service) granteeOf(ctx context.Context, sh *model.Share) (*provider.Grantee, error) {
	if sh.SharedWithIsGroup {
		return groupGrantee(sh.ShareWith), nil
	}
	u, err := admin.Lookup(ctx, sh.ShareWith)
	if err != nil {
		return nil, err
	}
	return userGrantee(u.GetId()), nil
}

// grantsAtStorage finds the storage provider hosting storageID through the
// registry, as the gateway does for its own grant calls.
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
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("looking up the storage provider of %q: %s: %s", storageID, code, res.GetStatus().GetMessage())
	}
	if len(res.GetProviders()) == 0 {
		return nil, fmt.Errorf("no storage provider for %q", storageID)
	}
	return revaservice.StorageProviderAt(res.GetProviders()[0].GetAddress())
}

// removeThroughGateway removes a share the way the web interface does: as
// the share's initiator, through the gateway, which removes the storage grant,
// re-applies the grants of the grantee's remaining shares and deletes the
// record.
func (s *service) removeThroughGateway(ctx context.Context, sh *model.Share) error {
	uctx, _, err := s.identity.ActAs(ctx, actorOf(sh))
	if err != nil {
		return err
	}
	gw, err := revaservice.Gateway(uctx)
	if err != nil {
		return err
	}
	res, err := gw.RemoveShare(uctx, &collaboration.RemoveShareRequest{Ref: shareRef(strconv.Itoa(int(sh.Id)))})
	if err != nil {
		return err
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return fmt.Errorf("removing the share: %s: %s", res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	return nil
}

// audit records one mutation under the usershareprovider namespace.
func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "usershareprovider." + action,
		Actor:   admin.Actor(ctx),
		Target:  target,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

// shareResult renders a stored share with stable snake_case keys.
func shareResult(sh *model.Share) invoke.Result {
	granteeType := "user"
	if sh.SharedWithIsGroup {
		granteeType = "group"
	}
	r := invoke.Result{
		"id":           strconv.Itoa(int(sh.Id)),
		"owner":        sh.UIDOwner,
		"initiator":    sh.UIDInitiator,
		"grantee":      map[string]any{"type": granteeType, "id": sh.ShareWith},
		"permissions":  int(sh.Permissions),
		"role":         permissions.RoleFromOCSPermissions(permissions.OcsPermissions(sh.Permissions)).Name,
		"resource":     map[string]any{"storage": sh.Instance, "inode": sh.Inode, "space_id": sh.SpaceID},
		"item_type":    sh.ItemType.String(),
		"initial_path": sh.InitialPath,
		"created_at":   admin.FormatTime(sh.CreatedAt),
		"updated_at":   admin.FormatTime(sh.UpdatedAt),
		"orphan":       sh.IsOrphan(),
	}
	if sh.Description != "" {
		r["description"] = sh.Description
	}
	if sh.Expiration.Valid {
		r["expires_at"] = admin.FormatTime(sh.Expiration.V)
		r["expired"] = isExpired(*sh)
	}
	if sh.OrphanedAt.Valid {
		r["orphaned_at"] = admin.FormatTime(sh.OrphanedAt.V)
	}
	return r
}

func shareRef(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: strings.TrimSpace(id)}}}
}

func userGrantee(id *userpb.UserId) *provider.Grantee {
	return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: id}}
}

func groupGrantee(name string) *provider.Grantee {
	return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_GROUP, Id: &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: name}}}
}

func resourceOf(sh *model.Share) *provider.ResourceId {
	return &provider.ResourceId{StorageId: sh.Instance, OpaqueId: sh.Inode, SpaceId: sh.SpaceID}
}

// actorOf is the user a share is acted on as: its initiator, who is allowed
// to manage it (on a project the owner is a service account), else its owner.
func actorOf(sh *model.Share) string {
	if sh.UIDInitiator != "" {
		return sh.UIDInitiator
	}
	return sh.UIDOwner
}

func isExpired(sh model.Share) bool {
	return sh.Expiration.Valid && time.Now().After(sh.Expiration.V)
}

// sharePermissions parses the permissions of a user share: a role name or
// its OCS value.
func sharePermissions(v string) (permissions.OcsPermissions, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "viewer", "reader", "1":
		return permissions.NewPermissions(1)
	case "editor", "15":
		return permissions.NewPermissions(15)
	}
	return 0, fmt.Errorf("permissions must be viewer (1) or editor (15), not %q", v)
}

// expirationArg parses an optional expires_at argument into a CS3 timestamp.
func expirationArg(v string) (*typespb.Timestamp, error) {
	t, none, err := admin.ParseTime(v)
	if err != nil || none {
		return nil, err
	}
	return &typespb.Timestamp{Seconds: uint64(t.Unix())}, nil
}

func parseResource(v string) (*provider.ResourceId, error) {
	storage, inode, ok := strings.Cut(v, ":")
	if !ok || storage == "" || inode == "" {
		return nil, fmt.Errorf("resource must be <storage>:<inode>, not %q", v)
	}
	return &provider.ResourceId{StorageId: storage, OpaqueId: inode}, nil
}

// statPath stats a path as the user of ctx.
func statPath(ctx context.Context, path string) (*provider.ResourceInfo, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("no path given")
	}
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: path}})
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("stat %s: %s: %s", path, res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	return res.GetInfo(), nil
}

// homeOwner is the owner of a path under a personal home, "" otherwise.
func homeOwner(path string) string {
	if rest, ok := strings.CutPrefix(path, homePathPrefix); ok {
		if parts := strings.Split(rest, "/"); len(parts) >= 2 && len(parts[0]) == 1 && parts[1] != "" {
			return parts[1]
		}
	}
	return ""
}

// projectOf is the name of the project a path belongs to.
func projectOf(path string) (string, bool) {
	if rest, ok := strings.CutPrefix(path, projectPathPrefix); ok {
		if parts := strings.Split(rest, "/"); len(parts) >= 2 && len(parts[0]) == 1 && parts[1] != "" {
			return parts[1], true
		}
	}
	return "", false
}

func projectAdminGroup(project string) string {
	return "cernbox-project-" + project + "-admins"
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
