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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	maxExpiringPage = 1000
	// listChunk is the number of links whose paths are resolved before they
	// are sent, so that a long list starts to arrive early.
	listChunk      = 100
	resolveWorkers = 8
)

type expiryLister interface {
	ExpiringPublicLinks(from, to, afterExpiration time.Time, afterID uint, limit int) ([]model.PublicLink, error)
}

// adminManager is what the admin invocations need from the public share
// manager, in addition to publicshare.Manager. Only the sql driver has it.
type adminManager interface {
	ListPublicLinks(u *userpb.User, filters []*link.ListPublicSharesRequest_Filter, expiry *sqlmgr.ExpiryRange, removeOrphan bool) ([]model.PublicLink, error)
	ListPublicLinksInSpace(spaceID string, filters []*link.ListPublicSharesRequest_Filter, includeOrphans bool) ([]model.PublicLink, error)
	GetPublicLink(ctx context.Context, ref *link.PublicShareReference, filter bool) (*model.PublicLink, error)
	TransferPublicShare(ctx context.Context, ref *link.PublicShareReference, newInitiator string) error
	MovePublicShare(ctx context.Context, ref *link.PublicShareReference, newLocation *provider.ResourceId, newOwner string) error
	MarkAsOrphaned(ctx context.Context, ref *link.PublicShareReference) error
}

type expiringArgs struct {
	From            time.Time `arg:"from,required" help:"start of the window"`
	To              time.Time `arg:"to,required" help:"end of the window, excluded"`
	AfterExpiration time.Time `arg:"after_expiration" help:"continue after the link with this expiration"`
	AfterID         int       `arg:"after_id" help:"continue after the link with this id"`
	Limit           int       `arg:"limit" help:"return at most this many links, default and max 1000"`
}

type listArgs struct {
	ID             string `arg:"id" help:"one link, by id; the other selectors are ignored"`
	Token          string `arg:"token" help:"one link, by token; the other selectors are ignored"`
	Owner          string `arg:"owner" help:"links on resources owned by this user"`
	Initiator      string `arg:"initiator" help:"links created by this user"`
	Space          string `arg:"space" help:"links in this space, by space id"`
	ResourceID     string `arg:"resource_id" help:"links of this resource, by id: storage_id!opaque_id"`
	IncludeOrphans bool   `arg:"include_orphans" help:"include orphaned links"`
	IncludeExpired *bool  `arg:"include_expired" help:"include expired links (default true)"`
	ResolvePaths   bool   `arg:"resolve_paths" help:"add the current path of each link"`
}

type refArgs struct {
	ID    string `arg:"id" help:"the link id; give id or token"`
	Token string `arg:"token" help:"the link token; give id or token"`
}

type updateArgs struct {
	ID        string  `arg:"id" help:"the link id; give id or token"`
	Token     string  `arg:"token" help:"the link token; give id or token"`
	Role      string  `arg:"role" help:"the new role: viewer, uploader or editor"`
	ExpiresAt string  `arg:"expires_at" help:"the new expiration: a date (2006-01-02), an RFC 3339 time, or none"`
	Password  *string `arg:"password" help:"the new password; an empty password removes it"`
	Fallback  bool    `arg:"fallback" help:"the caller fell back to admin rights after a user command, for the audit"`
}

type removeArgs struct {
	ID       string `arg:"id" help:"the link id; give id or token"`
	Token    string `arg:"token" help:"the link token; give id or token"`
	DBOnly   bool   `arg:"db_only" help:"delete the record without the checks of the gateway"`
	Fallback bool   `arg:"fallback" help:"the caller fell back to admin rights after a user command, for the audit"`
}

type transferArgs struct {
	ID        string `arg:"id" help:"the link id; give id or token"`
	Token     string `arg:"token" help:"the link token; give id or token"`
	Initiator string `arg:"initiator,required" help:"the new initiator, who must be allowed to share the resource"`
}

type moveArgs struct {
	ID    string `arg:"id" help:"the link id; give id or token"`
	Token string `arg:"token" help:"the link token; give id or token"`
	Path  string `arg:"path,required" help:"the new path of the linked resource"`
	As    string `arg:"as" help:"the user to resolve path as; by default the owner, then the initiator of the link"`
}

func (s *service) registerInvocations() {
	s.Set.Add("expiring", "List the public links of all users that expire in a window, by expiration and id, orphans excluded.").
		Shared().
		Handle(invoke.Typed(s.expiring))
	s.Set.Add("list", "Stream the public links of any user. Give at least one of id, token, owner, initiator, resource_id or space.").
		Shared().
		HandleStream(invoke.TypedStream(s.list))
	s.Set.Add("get", "Show one public link, also when it is orphaned or expired.").
		Shared().
		Handle(invoke.Typed(s.get))
	s.Set.Add("update", "Change the role, the expiration or the password of a public link through the gateway, as its initiator or owner.").
		Mutating().Shared().
		Handle(invoke.Typed(s.update))
	s.Set.Add("remove", "Remove a public link through the gateway, as its initiator or owner. For an orphan, or with db_only, only the record is deleted.").
		Mutating().Shared().
		Handle(invoke.Typed(s.remove))
	s.Set.Add("transfer", "Give a public link to a new initiator, who must be allowed to add a grant on the resource.").
		Mutating().Shared().
		Handle(invoke.Typed(s.transfer))
	s.Set.Add("move", "Point a public link at a new path after a data move. The owner of the new path becomes the owner of the link.").
		Dangerous().Shared().
		Handle(invoke.Typed(s.move))
	s.Set.Add("mark_orphan", "Mark a public link orphaned, which hides it from users and from the reconciliation jobs.").
		Mutating().Shared().
		Handle(invoke.Typed(s.markOrphan))
}

func (s *service) expiring(_ context.Context, a expiringArgs) (invoke.Result, error) {
	lister, ok := s.sm.(expiryLister)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot list links by expiration", s.conf.Driver)
	}
	if !a.To.After(a.From) {
		return nil, errors.New("to must be after from")
	}
	limit := a.Limit
	if limit <= 0 || limit > maxExpiringPage {
		limit = maxExpiringPage
	}
	links, err := lister.ExpiringPublicLinks(a.From, a.To, a.AfterExpiration, uint(a.AfterID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(links))
	for _, l := range links {
		out = append(out, map[string]any{
			"id":         l.Id,
			"token":      l.Token,
			"owner":      l.UIDOwner,
			"initiator":  l.UIDInitiator,
			"instance":   l.Instance,
			"inode":      l.Inode,
			"item_type":  string(l.ItemType),
			"name":       l.LinkName,
			"expires_at": l.Expiration.V.UTC().Format(time.RFC3339Nano),
			"updated_at": l.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return invoke.Result{"links": out}, nil
}

func (s *service) adminManager() (adminManager, error) {
	m, ok := s.sm.(adminManager)
	if !ok {
		return nil, errtypes.NotSupported(fmt.Sprintf("the %s driver does not support admin invocations", s.conf.Driver))
	}
	return m, nil
}

// lookup returns the link of id or token, also when it is orphaned or
// expired.
func (s *service) lookup(ctx context.Context, id, token string) (adminManager, *model.PublicLink, error) {
	m, err := s.adminManager()
	if err != nil {
		return nil, nil, err
	}
	ref, err := linkRef(id, token)
	if err != nil {
		return nil, nil, err
	}
	l, err := m.GetPublicLink(ctx, ref, false)
	if err != nil {
		return nil, nil, fmt.Errorf("public link %s: %w", cmp.Or(id, token), err)
	}
	return m, l, nil
}

func (s *service) list(ctx context.Context, a listArgs, emit invoke.StreamEmit) error {
	m, err := s.adminManager()
	if err != nil {
		return err
	}
	links, err := selectLinks(ctx, m, a)
	if err != nil {
		return err
	}
	if a.IncludeExpired != nil && !*a.IncludeExpired {
		links = slices.DeleteFunc(links, func(l model.PublicLink) bool { return isExpired(&l) })
	}
	slices.SortFunc(links, func(x, y model.PublicLink) int { return cmp.Compare(x.Id, y.Id) })

	users := &admin.Users{Ctx: ctx}
	for chunk := range slices.Chunk(links, listChunk) {
		paths := make([]string, len(chunk))
		if a.ResolvePaths {
			var g errgroup.Group
			g.SetLimit(resolveWorkers)
			for i := range chunk {
				g.Go(func() error {
					paths[i] = resolvePath(users, &chunk[i])
					return nil
				})
			}
			_ = g.Wait()
		}
		for i := range chunk {
			r, err := s.adminLink(&chunk[i], paths[i])
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

func selectLinks(ctx context.Context, m adminManager, a listArgs) ([]model.PublicLink, error) {
	if a.ID != "" || a.Token != "" {
		ref, _ := linkRef(a.ID, a.Token)
		l, err := m.GetPublicLink(ctx, ref, false)
		if isNotFound(err) || (err == nil && l.IsOrphan() && !a.IncludeOrphans) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []model.PublicLink{*l}, nil
	}

	var filters []*link.ListPublicSharesRequest_Filter
	if a.Owner != "" {
		filters = append(filters, &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_OWNER, Term: &link.ListPublicSharesRequest_Filter_Owner{Owner: &userpb.UserId{OpaqueId: a.Owner}}})
	}
	if a.Initiator != "" {
		filters = append(filters, &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_CREATOR, Term: &link.ListPublicSharesRequest_Filter_Creator{Creator: &userpb.UserId{OpaqueId: a.Initiator}}})
	}
	if a.ResourceID != "" {
		id, err := parseResourceID(a.ResourceID)
		if err != nil {
			return nil, err
		}
		filters = append(filters, &link.ListPublicSharesRequest_Filter{Type: link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID, Term: &link.ListPublicSharesRequest_Filter_ResourceId{ResourceId: id}})
	}
	if a.Space != "" {
		return m.ListPublicLinksInSpace(a.Space, filters, a.IncludeOrphans)
	}
	// The table is too large to read without a filter.
	if len(filters) == 0 {
		return nil, errtypes.BadRequest("list needs at least one of id, token, owner, initiator, resource_id or space")
	}
	return m.ListPublicLinks(nil, filters, nil, !a.IncludeOrphans)
}

func (s *service) get(ctx context.Context, a refArgs) (invoke.Result, error) {
	_, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	return s.adminLink(l, resolvePath(&admin.Users{Ctx: ctx}, l))
}

func (s *service) update(ctx context.Context, a updateArgs) (invoke.Result, error) {
	m, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	if l.IsOrphan() {
		return nil, errtypes.BadRequest(fmt.Sprintf("public link %d is orphaned: its resource is gone", l.Id))
	}

	perms := permissions.OcsPermissions(l.Permissions).AsCS3Permissions()
	exp := l.AsCS3PublicShare().GetExpiration()
	var updates []*link.UpdatePublicShareRequest_Update
	if a.Role != "" {
		role, err := linkRole(a.Role)
		if err != nil {
			return nil, err
		}
		perms = role.CS3ResourcePermissions()
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type:  link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS,
			Grant: &link.Grant{Permissions: &link.PublicSharePermissions{Permissions: perms}},
		})
	}
	if a.ExpiresAt != "" {
		if exp, err = admin.ParseExpiry(a.ExpiresAt); err != nil {
			return nil, err
		}
		// The link manager clears the expiration on a nil timestamp only.
		if exp.GetSeconds() == 0 {
			exp = nil
		}
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type:  link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION,
			Grant: &link.Grant{Expiration: exp},
		})
	}
	if a.Password != nil {
		updates = append(updates, &link.UpdatePublicShareRequest_Update{
			Type:  link.UpdatePublicShareRequest_Update_TYPE_PASSWORD,
			Grant: &link.Grant{Password: *a.Password},
		})
	}
	if len(updates) == 0 {
		return nil, errtypes.BadRequest("update needs role, expires_at or password")
	}
	// A password change alone keeps a legacy link without an expiration.
	if a.Role != "" || a.ExpiresAt != "" {
		if err := s.checkRWExpiration(perms, exp); err != nil {
			return nil, err
		}
	}

	ref := linkRefID(l.Id)
	actor, err := admin.ActAsFirst(ctx, linkUsers(l), func(uctx context.Context) (*rpc.Status, error) {
		gw, err := revaservice.Gateway(uctx)
		if err != nil {
			return nil, err
		}
		for _, u := range updates {
			res, err := gw.UpdatePublicShare(uctx, &link.UpdatePublicShareRequest{Ref: ref, Update: u})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
				return res.GetStatus(), err
			}
		}
		return status.NewOK(uctx), nil
	})
	fields := map[string]string{"acted_as": actor, "role": a.Role, "expires_at": a.ExpiresAt}
	switch {
	case a.Password == nil:
	case *a.Password == "":
		fields["password"] = "removed"
	default:
		fields["password"] = "set"
	}
	audit(ctx, "update", l, err, a.Fallback, fields)
	if err != nil {
		return nil, fmt.Errorf("updating public link %d: %w", l.Id, err)
	}
	if l, err = m.GetPublicLink(ctx, ref, false); err != nil {
		return nil, err
	}
	return s.adminLink(l, "")
}

// checkRWExpiration applies to an editor link the rule of the OCS and Graph
// services: with a maximum set, the link must expire at the latest that many
// seconds after the end of today.
func (s *service) checkRWExpiration(perms *provider.ResourcePermissions, exp *typespb.Timestamp) error {
	if s.conf.PubRWLinkMaxExpiration <= 0 || permissions.RoleFromResourcePermissions(perms).Name != permissions.RoleEditor {
		return nil
	}
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	limit := endOfDay.Add(time.Duration(s.conf.PubRWLinkMaxExpiration) * time.Second)
	if exp.GetSeconds() == 0 || exp.GetSeconds() > uint64(limit.Unix()) {
		return errtypes.BadRequest(fmt.Sprintf("an editor link must expire, at the latest on %s", limit.Format(time.DateOnly)))
	}
	return nil
}

func (s *service) remove(ctx context.Context, a removeArgs) (invoke.Result, error) {
	_, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	ref := linkRefID(l.Id)
	var actor string
	// The gateway stats the resource of the link, and an orphan has none.
	if a.DBOnly || l.IsOrphan() {
		err = s.sm.RevokePublicShare(ctx, nil, ref)
	} else {
		actor, err = admin.ActAsFirst(ctx, linkUsers(l), func(uctx context.Context) (*rpc.Status, error) {
			gw, err := revaservice.Gateway(uctx)
			if err != nil {
				return nil, err
			}
			res, err := gw.RemovePublicShare(uctx, &link.RemovePublicShareRequest{Ref: ref})
			return res.GetStatus(), err
		})
	}
	audit(ctx, "remove", l, err, a.Fallback, map[string]string{"acted_as": actor, "db_only": strconv.FormatBool(a.DBOnly || l.IsOrphan())})
	if err != nil {
		return nil, fmt.Errorf("removing public link %d: %w", l.Id, err)
	}
	return s.adminLink(l, "")
}

func (s *service) transfer(ctx context.Context, a transferArgs) (invoke.Result, error) {
	m, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	// The storage decides who may hold a link: the new initiator must be
	// allowed to add a grant on the resource, as to create the link.
	uctx, u, err := admin.ActAs(ctx, a.Initiator)
	if err == nil {
		var res *provider.StatResponse
		res, err = stat(uctx, &provider.Reference{ResourceId: resourceOf(l)})
		switch {
		case err != nil:
		case res.GetStatus().GetCode() != rpc.Code_CODE_OK:
			err = status.NewErrtypeFromStatus(res.GetStatus())
		case !res.GetInfo().GetPermissionSet().GetAddGrant():
			err = errtypes.PermissionDenied(a.Initiator + " is not allowed to share the resource")
		}
	}
	ref := linkRefID(l.Id)
	if err == nil {
		err = m.TransferPublicShare(ctx, ref, u.GetId().GetOpaqueId())
	}
	audit(ctx, "transfer", l, err, false, map[string]string{"from": l.UIDInitiator, "to": a.Initiator})
	if err != nil {
		return nil, fmt.Errorf("transferring public link %d: %w", l.Id, err)
	}
	if l, err = m.GetPublicLink(ctx, ref, false); err != nil {
		return nil, err
	}
	return s.adminLink(l, "")
}

func (s *service) move(ctx context.Context, a moveArgs) (invoke.Result, error) {
	m, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	as := slices.Compact([]string{l.UIDOwner, l.UIDInitiator})
	if a.As != "" {
		as = []string{a.As}
	}
	var info *provider.ResourceInfo
	actor, err := admin.ActAsFirst(ctx, as, func(uctx context.Context) (*rpc.Status, error) {
		res, err := stat(uctx, &provider.Reference{Path: a.Path})
		info = res.GetInfo()
		return res.GetStatus(), err
	})
	owner := info.GetOwner().GetOpaqueId()
	if err == nil && owner == "" {
		err = fmt.Errorf("%s has no owner", a.Path)
	}
	ref := linkRefID(l.Id)
	if err == nil {
		err = m.MovePublicShare(ctx, ref, info.GetId(), owner)
	}
	audit(ctx, "move", l, err, false, map[string]string{"acted_as": actor, "path": a.Path, "new_owner": owner})
	if err != nil {
		return nil, fmt.Errorf("moving public link %d: %w", l.Id, err)
	}
	if l, err = m.GetPublicLink(ctx, ref, false); err != nil {
		return nil, err
	}
	return s.adminLink(l, info.GetPath())
}

func (s *service) markOrphan(ctx context.Context, a refArgs) (invoke.Result, error) {
	m, l, err := s.lookup(ctx, a.ID, a.Token)
	if err != nil {
		return nil, err
	}
	ref := linkRefID(l.Id)
	err = m.MarkAsOrphaned(ctx, ref)
	audit(ctx, "mark_orphan", l, err, false, map[string]string{})
	if err != nil {
		return nil, fmt.Errorf("marking public link %d orphaned: %w", l.Id, err)
	}
	if l, err = m.GetPublicLink(ctx, ref, false); err != nil {
		return nil, err
	}
	return s.adminLink(l, "")
}

// adminLink returns l as the protojson of the AdminPublicShare message of the
// typed Admin API: the CS3 link, and what CS3 cannot carry. The password and
// its hash never leave the server.
func (s *service) adminLink(l *model.PublicLink, path string) (invoke.Result, error) {
	ps := l.AsCS3PublicShare()
	ps.ResourceId.SpaceId = l.SpaceID
	b, err := protojson.Marshal(ps)
	if err != nil {
		return nil, err
	}
	var public map[string]any
	if err := json.Unmarshal(b, &public); err != nil {
		return nil, err
	}
	r := invoke.Result{"public_share": public}
	if l.IsOrphan() {
		r["orphan"] = true
	}
	if path != "" {
		r["path"] = path
	}
	if l.Password != "" {
		r["password_protected"] = true
	}
	extra := map[string]string{}
	if l.InitialPath != "" {
		extra["initial_path"] = l.InitialPath
	}
	if l.OrphanedAt.Valid {
		extra["orphaned_at"] = l.OrphanedAt.V.UTC().Format(time.RFC3339)
	}
	if s.conf.PublicLinkBaseURL != "" {
		extra["url"] = s.conf.PublicLinkBaseURL + l.Token
	}
	if len(extra) > 0 {
		r["extra"] = extra
	}
	return r, nil
}

// resolvePath returns the current path of the resource of l, or "" if no user
// can resolve it.
func resolvePath(users *admin.Users, l *model.PublicLink) string {
	for _, u := range linkUsers(l) {
		uctx := users.As(u)
		if uctx == nil {
			continue
		}
		gw, err := revaservice.Gateway(uctx)
		if err != nil {
			return ""
		}
		res, err := gw.GetPath(uctx, &provider.GetPathRequest{ResourceId: resourceOf(l)})
		if err == nil && res.GetStatus().GetCode() == rpc.Code_CODE_OK {
			return res.GetPath()
		}
	}
	return ""
}

// audit records a mutation of l. fallback tells that a user command fell back
// to admin rights on a link of another user.
func audit(ctx context.Context, action string, l *model.PublicLink, err error, fallback bool, fields map[string]string) {
	maps.DeleteFunc(fields, func(_, v string) bool { return v == "" })
	fields["owner"] = l.UIDOwner
	fields["initiator"] = l.UIDInitiator
	if fallback {
		fields["fallback"] = "true"
	}
	u, _ := appctx.ContextGetUser(ctx)
	admin.Audit(ctx, admin.AuditEvent{
		Action:  "publicshareprovider." + action,
		Actor:   u.GetUsername(),
		Target:  "link:" + strconv.FormatUint(uint64(l.Id), 10),
		Granted: err == nil,
		Err:     err,
		Fields:  fields,
	})
}

// linkRole returns the role of a public link with the name given. A link
// cannot have the other roles.
func linkRole(name string) (*permissions.Role, error) {
	switch role := permissions.RoleFromName(name); role.Name {
	case permissions.RoleViewer, permissions.RoleUploader, permissions.RoleEditor:
		return role, nil
	}
	return nil, errtypes.BadRequest(fmt.Sprintf("the role of a public link is viewer, uploader or editor, not %q", name))
}

func stat(uctx context.Context, ref *provider.Reference) (*provider.StatResponse, error) {
	gw, err := revaservice.Gateway(uctx)
	if err != nil {
		return nil, err
	}
	return gw.Stat(uctx, &provider.StatRequest{Ref: ref})
}

func isNotFound(err error) bool {
	var nf errtypes.IsNotFound
	return errors.As(err, &nf)
}

func isExpired(l *model.PublicLink) bool {
	return l.Expiration.Valid && !l.Expiration.V.After(time.Now())
}

// linkUsers returns the users a link is acted on as, in order: the initiator
// manages the link, and the owner of a project can be a service account.
func linkUsers(l *model.PublicLink) []string {
	return slices.Compact(slices.DeleteFunc([]string{l.UIDInitiator, l.UIDOwner}, func(v string) bool { return v == "" }))
}

func linkRef(id, token string) (*link.PublicShareReference, error) {
	switch {
	case id != "":
		return &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}}}, nil
	case token != "":
		return &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: token}}, nil
	}
	return nil, errtypes.BadRequest("give the id or the token of the link")
}

// parseResourceID reads storage_id!opaque_id, or the storage_id$space_id!opaque_id
// of the spaces. The link table matches the storage id and the opaque id only.
func parseResourceID(s string) (*provider.ResourceId, error) {
	storage, opaque, ok := strings.Cut(s, "!")
	if !ok || storage == "" || opaque == "" {
		return nil, errtypes.BadRequest(fmt.Sprintf("invalid resource id %q: use storage_id!opaque_id", s))
	}
	storage, space, _ := strings.Cut(storage, "$")
	return &provider.ResourceId{StorageId: storage, SpaceId: space, OpaqueId: opaque}, nil
}

func linkRefID(id uint) *link.PublicShareReference {
	ref, _ := linkRef(strconv.FormatUint(uint64(id), 10), "")
	return ref
}

func resourceOf(l *model.PublicLink) *provider.ResourceId {
	return &provider.ResourceId{StorageId: l.Instance, OpaqueId: l.Inode, SpaceId: l.SpaceID}
}
