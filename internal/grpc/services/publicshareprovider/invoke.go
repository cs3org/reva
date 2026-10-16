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

package publicshareprovider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
)

// The public share provider is an invoke.Invokable: through the Admin API an
// operator can list, create, expire and repair the public links of any user.
// Every mutation is audited; creation acts on the storage as the link's
// initiator, through the gateway, like a link made from the web interface.

// adminManager is what the invocations need from the public share manager
// beyond the CS3 publicshare.Manager. The SQL driver implements it; any other
// driver makes the invocations answer that they are unsupported.
type adminManager interface {
	ListPublicLinks(u *userpb.User, filters []*link.ListPublicSharesRequest_Filter, expiry *sqlmgr.ExpiryRange, removeOrphan bool) ([]model.PublicLink, error)
	ListPublicLinksInSpace(spaceID string, includeOrphans bool) ([]model.PublicLink, error)
	GetPublicLink(ctx context.Context, ref *link.PublicShareReference, filter bool) (*model.PublicLink, error)
	TransferPublicShare(ctx context.Context, ref *link.PublicShareReference, newInitiator string) error
	MovePublicShare(ctx context.Context, ref *link.PublicShareReference, newLocation *provider.ResourceId, newOwner string) error
	MarkAsOrphaned(ctx context.Context, ref *link.PublicShareReference) error
}

const (
	maxResolvedPaths = 500
	defaultListLimit = 1000
	resolveWorkers   = 8

	projectPathPrefix = "/eos/project/"
	homePathPrefix    = "/eos/user/"
)

func (s *service) registerInvocations() {
	s.Set = invoke.NewSet()
	s.Set.Add("list", "List public links, orphaned and expired ones included on request. At least one selector is required.").
		OptArg("owner", "links on resources owned by this user").
		OptArg("initiator", "links created by this user").
		OptArg("id", "one link, by id").
		OptArg("token", "one link, by token").
		OptArg("resource", "links of one resource, as <storage>:<inode>").
		OptArg("space", "links in a space, by space id").
		OptArg("path", "links of a path, resolved as the user given in as, or as the owner of a home under "+homePathPrefix).
		OptArg("as", "the user to resolve path and the link paths as").
		OptArg("expiring_from", "only links expiring at or after this date or RFC 3339 time").
		OptArg("expiring_to", "only links expiring at or before this date or RFC 3339 time").
		OptArg("include_orphans", "include orphaned links (default false)").
		OptArg("include_expired", "include expired links (default true)").
		OptArg("resolve_paths", "add the current path of every link (default false, at most "+strconv.Itoa(maxResolvedPaths)+" links)").
		OptArg("limit", "return at most this many links (default "+strconv.Itoa(defaultListLimit)+")").
		OptArg("offset", "skip this many links").
		Handle(s.invokeList)
	s.Set.Add("get", "Show one public link, orphaned or expired or not.").
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		OptArg("resolve_path", "add the current path of the link").
		Handle(s.invokeGet)
	s.Set.Add("create", "Create a public link on a path as its initiator, like the web interface does. Address one instance by node id.").
		Mutating().
		Arg("path", "the path to link").
		Arg("initiator", "the user creating the link; must be allowed to share the path").
		Arg("permissions", "viewer (1), uploader (4) or editor (15)").
		OptArg("password", "protect the link with this password").
		OptArg("expires_at", "a date or RFC 3339 time; required for an editor link unless force is set").
		OptArg("name", "the name shown for the link").
		OptArg("description", "a description").
		OptArg("internal", "create an internal link").
		OptArg("force", "allow an editor link without expiration").
		Handle(s.invokeCreate)
	s.Set.Add("remove", "Delete a public link. Address one instance by node id.").
		Mutating().
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		Handle(s.invokeRemove)
	s.Set.Add("set_expiry", "Set or clear the expiration of a public link. Address one instance by node id.").
		Mutating().
		Arg("expires_at", "a date, an RFC 3339 time, or none").
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		OptArg("force", "allow clearing the expiration of an editor link").
		Handle(s.invokeSetExpiry)
	s.Set.Add("set_password", "Set or, with an empty password, remove the password of a public link. Address one instance by node id.").
		Mutating().
		Arg("password", "the new password, empty to remove it").
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		Handle(s.invokeSetPassword)
	s.Set.Add("transfer", "Hand a public link on a project over to a new initiator, who must administer the project. Address one instance by node id.").
		Mutating().
		Arg("initiator", "the new initiator").
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		Handle(s.invokeTransfer)
	s.Set.Add("move", "Point a public link at another path, updating its owner. Address one instance by node id.").
		Dangerous().
		Arg("path", "the new path").
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		OptArg("owner", "the owner of the new path; derived for a home under "+homePathPrefix).
		Handle(s.invokeMove)
	s.Set.Add("mark_orphan", "Mark a public link orphaned, hiding it from every listing. Address one instance by node id.").
		Mutating().
		OptArg("id", "the link id").
		OptArg("token", "the link token").
		Handle(s.invokeMarkOrphan)
}

func (s *service) admin() (adminManager, error) {
	m, ok := s.sm.(adminManager)
	if !ok {
		return nil, fmt.Errorf("publicshareprovider: the %s driver does not support admin invocations", s.conf.Driver)
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

	var links []model.PublicLink
	switch {
	case a.String("id") != "" || a.String("token") != "":
		ref, err := linkRefArgs(a)
		if err != nil {
			return nil, err
		}
		l, err := m.GetPublicLink(ctx, ref, false)
		if err != nil {
			return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
		}
		links = []model.PublicLink{*l}
	case a.String("space") != "":
		if links, err = m.ListPublicLinksInSpace(a.String("space"), includeOrphans); err != nil {
			return nil, err
		}
	default:
		var filters []*link.ListPublicSharesRequest_Filter
		selected := false
		if owner := a.String("owner"); owner != "" {
			filters = append(filters, &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_OWNER, Term: &link.ListPublicSharesRequest_Filter_Owner{Owner: &userpb.UserId{OpaqueId: owner}}})
			selected = true
		}
		if initiator := a.String("initiator"); initiator != "" {
			filters = append(filters, &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_CREATOR, Term: &link.ListPublicSharesRequest_Filter_Creator{Creator: &userpb.UserId{OpaqueId: initiator}}})
			selected = true
		}
		if res := a.String("resource"); res != "" {
			id, err := parseResource(res)
			if err != nil {
				return nil, err
			}
			filters = append(filters, publicshare.ResourceIDFilter(id))
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
			filters = append(filters, publicshare.ResourceIDFilter(info.GetId()))
			selected = true
		}
		var expiry *sqlmgr.ExpiryRange
		if a.Has("expiring_from") || a.Has("expiring_to") {
			from, _, err := admin.ParseTime(a.String("expiring_from"))
			if err != nil {
				return nil, err
			}
			to, none, err := admin.ParseTime(a.String("expiring_to"))
			if err != nil {
				return nil, err
			}
			if none {
				to = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
			}
			expiry = &sqlmgr.ExpiryRange{From: from, To: to}
			selected = true
		}
		if !selected {
			return nil, errors.New("list needs at least one of owner, initiator, id, token, resource, space, path or an expiring window")
		}
		if links, err = m.ListPublicLinks(nil, filters, expiry, !includeOrphans); err != nil {
			return nil, err
		}
	}

	if !includeExpired {
		links = slices.DeleteFunc(links, func(l model.PublicLink) bool { return isExpired(l) })
	}
	if !includeOrphans {
		links = slices.DeleteFunc(links, func(l model.PublicLink) bool { return l.IsOrphan() })
	}
	slices.SortFunc(links, func(x, y model.PublicLink) int { return int(x.Id) - int(y.Id) })

	total := len(links)
	offset := max(a.Int("offset"), 0)
	limit := defaultListLimit
	if a.Has("limit") {
		limit = a.Int("limit")
	}
	if offset > len(links) {
		offset = len(links)
	}
	links = links[offset:]
	truncated := false
	if limit > 0 && len(links) > limit {
		links = links[:limit]
		truncated = true
	}

	var paths map[uint]string
	if a.Bool("resolve_paths") {
		if len(links) > maxResolvedPaths {
			return nil, fmt.Errorf("resolve_paths is limited to %d links, narrow the selection or pass limit", maxResolvedPaths)
		}
		paths = s.resolvePaths(ctx, links, a.String("as"))
	}
	out := make([]invoke.Result, 0, len(links))
	for _, l := range links {
		r := linkResult(&l)
		if p, ok := paths[l.Id]; ok {
			r["path"] = p
		}
		out = append(out, r)
	}
	return invoke.Result{"links": out, "count": len(out), "total": total, "truncated": truncated}, nil
}

func (s *service) invokeGet(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	r := linkResult(l)
	if a.Bool("resolve_path") {
		if p, ok := s.resolvePaths(ctx, []model.PublicLink{*l}, "")[l.Id]; ok {
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
	perms, err := linkPermissions(a.String("permissions"))
	if err != nil {
		return nil, err
	}
	expiration, err := expirationArg(a.String("expires_at"))
	if err != nil {
		return nil, err
	}
	if err := editorNeedsExpiry(perms, expiration != nil, a.Bool("force")); err != nil {
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
	if name := a.String("name"); name != "" {
		if info.ArbitraryMetadata == nil {
			info.ArbitraryMetadata = &provider.ArbitraryMetadata{Metadata: map[string]string{}}
		}
		info.ArbitraryMetadata.Metadata["name"] = name
	}
	req := &link.CreatePublicShareRequest{
		ResourceInfo: info,
		Grant: &link.Grant{
			Permissions: &link.PublicSharePermissions{Permissions: perms.AsCS3Permissions()},
			Password:    a.String("password"),
			Expiration:  expiration,
		},
		Description: a.String("description"),
		Internal:    a.Bool("internal"),
	}
	gw, err := revaservice.Gateway(uctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.CreatePublicShare(uctx, req)
	if err == nil && res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		err = fmt.Errorf("creating the public link: %s: %s", res.GetStatus().GetCode(), res.GetStatus().GetMessage())
	}
	audit(ctx, "create", initiator, err, map[string]string{"path": info.GetPath(), "permissions": strconv.Itoa(int(perms)), "password_protected": strconv.FormatBool(a.String("password") != "")})
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, linkRefID(res.GetShare().GetId().GetOpaqueId()), false)
	if err != nil {
		return nil, err
	}
	r := linkResult(l)
	r["created"] = true
	r["path"] = info.GetPath()
	return r, nil
}

func (s *service) invokeRemove(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	u, _ := appctx.ContextGetUser(ctx)
	err = s.sm.RevokePublicShare(ctx, u, linkRefID(strconv.Itoa(int(l.Id))))
	audit(ctx, "remove", l.UIDInitiator, err, map[string]string{"link": strconv.Itoa(int(l.Id)), "token": l.Token})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"removed": strconv.Itoa(int(l.Id)), "token": l.Token}, nil
}

func (s *service) invokeSetExpiry(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	expiration, err := expirationArg(a.String("expires_at"))
	if err != nil {
		return nil, err
	}
	if err := editorNeedsExpiry(permissions.OcsPermissions(l.Permissions), expiration != nil, a.Bool("force")); err != nil {
		return nil, err
	}
	u, _ := appctx.ContextGetUser(ctx)
	_, err = s.sm.UpdatePublicShare(ctx, u, &link.UpdatePublicShareRequest{
		Ref:    linkRefID(strconv.Itoa(int(l.Id))),
		Update: &link.UpdatePublicShareRequest_Update{Type: link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION, Grant: &link.Grant{Expiration: expiration}},
	}, nil)
	audit(ctx, "set_expiry", l.UIDInitiator, err, map[string]string{"link": strconv.Itoa(int(l.Id)), "expires_at": a.String("expires_at")})
	if err != nil {
		return nil, err
	}
	return s.result(ctx, m, l.Id)
}

func (s *service) invokeSetPassword(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	password := a.String("password")
	u, _ := appctx.ContextGetUser(ctx)
	_, err = s.sm.UpdatePublicShare(ctx, u, &link.UpdatePublicShareRequest{
		Ref:    linkRefID(strconv.Itoa(int(l.Id))),
		Update: &link.UpdatePublicShareRequest_Update{Type: link.UpdatePublicShareRequest_Update_TYPE_PASSWORD, Grant: &link.Grant{Password: password}},
	}, nil)
	audit(ctx, "set_password", l.UIDInitiator, err, map[string]string{"link": strconv.Itoa(int(l.Id)), "password_protected": strconv.FormatBool(password != "")})
	if err != nil {
		return nil, err
	}
	return s.result(ctx, m, l.Id)
}

func (s *service) invokeTransfer(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	initiator := a.String("initiator")
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	project, ok := projectOf(l.InitialPath)
	if !ok {
		return nil, fmt.Errorf("public link %d is not on a project: only links on projects can be transferred", l.Id)
	}
	u, err := admin.Lookup(ctx, initiator)
	if err != nil {
		return nil, err
	}
	if group := projectAdminGroup(project); !slices.Contains(u.GetGroups(), group) {
		return nil, fmt.Errorf("%s is not in %s: only administrators of the project can hold its links", initiator, group)
	}
	err = m.TransferPublicShare(ctx, linkRefID(strconv.Itoa(int(l.Id))), initiator)
	audit(ctx, "transfer", initiator, err, map[string]string{"link": strconv.Itoa(int(l.Id)), "from": l.UIDInitiator})
	if err != nil {
		return nil, err
	}
	return s.result(ctx, m, l.Id)
}

func (s *service) invokeMove(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	path := a.String("path")
	owner := firstNonEmpty(a.String("owner"), homeOwner(path))
	if owner == "" {
		return nil, fmt.Errorf("the owner of %q cannot be derived: pass owner=<user>", path)
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	uctx, _, err := s.identity.ActAs(ctx, owner)
	if err != nil {
		return nil, err
	}
	info, err := statPath(uctx, path)
	if err != nil {
		return nil, err
	}
	err = m.MovePublicShare(ctx, linkRefID(strconv.Itoa(int(l.Id))), info.GetId(), owner)
	audit(ctx, "move", owner, err, map[string]string{"link": strconv.Itoa(int(l.Id)), "path": info.GetPath()})
	if err != nil {
		return nil, err
	}
	r, err := s.result(ctx, m, l.Id)
	if err != nil {
		return nil, err
	}
	r["path"] = info.GetPath()
	return r, nil
}

func (s *service) invokeMarkOrphan(ctx context.Context, a invoke.Args) (invoke.Result, error) {
	m, err := s.admin()
	if err != nil {
		return nil, err
	}
	ref, err := linkRefArgs(a)
	if err != nil {
		return nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, fmt.Errorf("public link %s: %w", refString(ref), err)
	}
	err = m.MarkAsOrphaned(ctx, linkRefID(strconv.Itoa(int(l.Id))))
	audit(ctx, "mark_orphan", l.UIDInitiator, err, map[string]string{"link": strconv.Itoa(int(l.Id))})
	if err != nil {
		return nil, err
	}
	return invoke.Result{"orphaned": strconv.Itoa(int(l.Id))}, nil
}

// result re-reads a link after a mutation.
func (s *service) result(ctx context.Context, m adminManager, id uint) (invoke.Result, error) {
	l, err := m.GetPublicLink(ctx, linkRefID(strconv.Itoa(int(id))), false)
	if err != nil {
		return nil, err
	}
	return linkResult(l), nil
}

// resolvePaths returns the current path of each link, looked up as the
// link's initiator (or as `as` when given), a few at a time.
func (s *service) resolvePaths(ctx context.Context, links []model.PublicLink, as string) map[uint]string {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		paths = make(map[uint]string, len(links))
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
	for _, l := range links {
		wg.Add(1)
		sem <- struct{}{}
		go func(l model.PublicLink) {
			defer wg.Done()
			defer func() { <-sem }()
			uctx, err := userCtx(firstNonEmpty(as, l.UIDInitiator, l.UIDOwner))
			if err != nil {
				return
			}
			gw, err := revaservice.Gateway(uctx)
			if err != nil {
				return
			}
			res, err := gw.GetPath(uctx, &provider.GetPathRequest{ResourceId: &provider.ResourceId{StorageId: l.Instance, OpaqueId: l.Inode, SpaceId: l.SpaceID}})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
				return
			}
			mu.Lock()
			paths[l.Id] = res.GetPath()
			mu.Unlock()
		}(l)
	}
	wg.Wait()
	return paths
}

// audit records one mutation under the publicshareprovider namespace.
func audit(ctx context.Context, action, target string, err error, fields map[string]string) {
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "publicshareprovider." + action,
		Actor:   admin.Actor(ctx),
		Target:  target,
		Kind:    string(invoke.KindMutating),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

// linkResult renders a stored link with stable snake_case keys. The password
// hash never leaves the server.
func linkResult(l *model.PublicLink) invoke.Result {
	r := invoke.Result{
		"id":                 strconv.Itoa(int(l.Id)),
		"token":              l.Token,
		"owner":              l.UIDOwner,
		"initiator":          l.UIDInitiator,
		"permissions":        int(l.Permissions),
		"role":               permissions.RoleFromOCSPermissions(permissions.OcsPermissions(l.Permissions)).Name,
		"resource":           map[string]any{"storage": l.Instance, "inode": l.Inode, "space_id": l.SpaceID},
		"item_type":          l.ItemType.String(),
		"initial_path":       l.InitialPath,
		"name":               l.LinkName,
		"quicklink":          l.Quicklink,
		"password_protected": l.Password != "",
		"notify_uploads":     l.NotifyUploads,
		"created_at":         admin.FormatTime(l.CreatedAt),
		"updated_at":         admin.FormatTime(l.UpdatedAt),
		"orphan":             l.IsOrphan(),
	}
	if l.NotifyUploadsExtraRecipients != "" {
		r["notify_uploads_extra_recipients"] = l.NotifyUploadsExtraRecipients
	}
	if l.Expiration.Valid {
		r["expires_at"] = admin.FormatTime(l.Expiration.V)
		r["expired"] = isExpired(*l)
	}
	if l.OrphanedAt.Valid {
		r["orphaned_at"] = admin.FormatTime(l.OrphanedAt.V)
	}
	return r
}

func isExpired(l model.PublicLink) bool {
	return l.Expiration.Valid && time.Now().After(l.Expiration.V)
}

// editorNeedsExpiry enforces the CERNBox rule that a link that lets anyone
// write expires, unless the operator insists.
func editorNeedsExpiry(perms permissions.OcsPermissions, hasExpiry, force bool) error {
	if perms == 15 && !hasExpiry && !force {
		return errors.New("an editor link needs an expiration date; pass force=true to do without")
	}
	return nil
}

// linkPermissions parses the permissions of a public link: a role name or its
// OCS value.
func linkPermissions(v string) (permissions.OcsPermissions, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "viewer", "reader", "1":
		return permissions.NewPermissions(1)
	case "uploader", "4":
		return permissions.NewPermissions(4)
	case "editor", "15":
		return permissions.NewPermissions(15)
	}
	return 0, fmt.Errorf("permissions must be viewer (1), uploader (4) or editor (15), not %q", v)
}

func expirationArg(v string) (*typespb.Timestamp, error) {
	t, none, err := admin.ParseTime(v)
	if err != nil || none {
		return nil, err
	}
	return &typespb.Timestamp{Seconds: uint64(t.Unix())}, nil
}

// linkRefArgs builds the reference of a link from the id or token argument.
func linkRefArgs(a invoke.Args) (*link.PublicShareReference, error) {
	if id := strings.TrimSpace(a.String("id")); id != "" {
		return linkRefID(id), nil
	}
	if token := strings.TrimSpace(a.String("token")); token != "" {
		return &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: token}}, nil
	}
	return nil, errors.New("pass id=<id> or token=<token>")
}

func linkRefID(id string) *link.PublicShareReference {
	return &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}}}
}

func refString(ref *link.PublicShareReference) string {
	if id := ref.GetId(); id != nil {
		return id.GetOpaqueId()
	}
	return ref.GetToken()
}

func parseResource(v string) (*provider.ResourceId, error) {
	storage, inode, ok := strings.Cut(v, ":")
	if !ok || storage == "" || inode == "" {
		return nil, fmt.Errorf("resource must be <storage>:<inode>, not %q", v)
	}
	return &provider.ResourceId{StorageId: storage, OpaqueId: inode}, nil
}

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

func homeOwner(path string) string {
	if rest, ok := strings.CutPrefix(path, homePathPrefix); ok {
		if parts := strings.Split(rest, "/"); len(parts) >= 2 && len(parts[0]) == 1 && parts[1] != "" {
			return parts[1]
		}
	}
	return ""
}

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
