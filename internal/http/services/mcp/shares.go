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

package mcp

import (
	"fmt"
	"slices"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

// linkIDPrefix tells public links from user and group shares, whose ids may
// overlap.
const linkIDPrefix = "link:"

func (s *svc) registerShareTools() {
	// Creating a share pings the share provider with a share lookup, which
	// needs shares:list.
	manage := []string{scope.PermSharesManage, scope.PermSharesList}

	addTool(s, toolSpec{
		name: toolListShares,
		description: "List shares. direction incoming (the default) lists what other people shared with the user; " +
			"outgoing lists the shares and public links the user created. When next_cursor is set, call again with it as cursor; a page can hold fewer entries than others.",
		permissions: []string{scope.PermSharesList},
		readOnly:    true,
		enums:       map[string][]any{"direction": {"incoming", "outgoing"}},
	}, s.listShares)

	addTool(s, toolSpec{
		name: toolCreateShare,
		description: "Share a file or folder with a person or a group of CERNBox users. path is absolute. grantee is the exact username, such as jdoe, " +
			"or group:<name> for a group. role is viewer (the default) or editor. Ask the user before sharing.",
		permissions: manage,
		enums:       map[string][]any{"role": {"viewer", "editor"}},
	}, s.createShare)

	addTool(s, toolSpec{
		name: toolCreatePublicLink,
		description: "Create a public link to a file or folder: anyone with the link can open it, without a CERNBox account. path is absolute. " +
			"role is viewer (the default), editor, or uploader (a folder people can only upload into). Prefer setting an expiry, and a password for sensitive content. Ask the user before creating one.",
		permissions: manage,
		enums:       map[string][]any{"role": {"viewer", "editor", "uploader"}},
	}, s.createPublicLink)

	addTool(s, toolSpec{
		name:        toolRemoveShare,
		description: "Remove a share or public link the user created, by the id list_shares, create_share or create_public_link returned.",
		permissions: manage,
		destructive: true,
	}, s.removeShare)
}

type shareEntry struct {
	ID                string `json:"id" jsonschema:"pass it to remove_share"`
	Kind              string `json:"kind" jsonschema:"user, group or link"`
	Path              string `json:"path"`
	Role              string `json:"role"`
	Grantee           string `json:"grantee,omitempty" jsonschema:"who it is shared with"`
	Owner             string `json:"owner,omitempty" jsonschema:"who shared it, for an incoming share"`
	State             string `json:"state,omitempty" jsonschema:"for an incoming share: pending, accepted or rejected"`
	URL               string `json:"url,omitempty" jsonschema:"the public link"`
	Expiry            string `json:"expiry,omitempty" jsonschema:"when the public link stops working, RFC 3339"`
	PasswordProtected bool   `json:"password_protected,omitempty"`
	WebURL            string `json:"web_url"`
}

type listSharesIn struct {
	Direction string `json:"direction,omitempty" jsonschema:"incoming (the default) or outgoing"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page"`
}

type listSharesOut struct {
	Shares     []shareEntry `json:"shares"`
	NextCursor string       `json:"next_cursor,omitempty" jsonschema:"set when there are more shares: pass it as cursor"`
}

// shareItem is a share or a public link, before its resource is resolved.
type shareItem struct {
	id       string
	share    *collaboration.Share
	received *collaboration.ReceivedShare
	link     *link.PublicShare
}

func (it shareItem) key() string { return it.id }

func (it shareItem) resourceID() *provider.ResourceId {
	if it.link != nil {
		return it.link.GetResourceId()
	}
	return it.share.GetResourceId()
}

func (s *svc) listShares(c *call, in listSharesIn) (listSharesOut, error) {
	var items []shareItem
	switch in.Direction {
	case "", "incoming":
		res, err := c.gw.ListReceivedShares(c.ctx, &collaboration.ListReceivedSharesRequest{})
		if err != nil {
			return listSharesOut{}, asToolError(err, "shares")
		}
		if err := checkStatus(res.GetStatus(), "shares"); err != nil {
			return listSharesOut{}, err
		}
		for _, rs := range res.GetShares() {
			items = append(items, shareItem{id: rs.GetShare().GetId().GetOpaqueId(), share: rs.GetShare(), received: rs})
		}
	case "outgoing":
		res, err := c.gw.ListShares(c.ctx, &collaboration.ListSharesRequest{})
		if err != nil {
			return listSharesOut{}, asToolError(err, "shares")
		}
		if err := checkStatus(res.GetStatus(), "shares"); err != nil {
			return listSharesOut{}, err
		}
		for _, sh := range res.GetShares() {
			items = append(items, shareItem{id: sh.GetId().GetOpaqueId(), share: sh})
		}
		links, err := c.gw.ListPublicShares(c.ctx, &link.ListPublicSharesRequest{})
		if err != nil {
			return listSharesOut{}, asToolError(err, "public links")
		}
		if err := checkStatus(links.GetStatus(), "public links"); err != nil {
			return listSharesOut{}, err
		}
		for _, l := range links.GetShare() {
			items = append(items, shareItem{id: linkIDPrefix + l.GetId().GetOpaqueId(), link: l})
		}
	default:
		return listSharesOut{}, newError(codeInvalidArgument, "direction is incoming or outgoing")
	}

	// The resources are resolved for the page only. A share whose resource is
	// gone or outside the roots of the app password is left out.
	items, next, err := page(items, shareItem.key, in.Cursor, s.c.ListMaxEntries)
	if err != nil {
		return listSharesOut{}, err
	}
	out := listSharesOut{Shares: []shareEntry{}, NextCursor: next}
	names := map[string]string{}
	for _, it := range items {
		info, err := c.statID(it.resourceID())
		if code := errorCode(err); code == codeNotFound || code == codePermissionDenied {
			continue
		}
		if err != nil {
			return listSharesOut{}, err
		}
		if !c.ap.UnderRoots(info.GetPath()) {
			continue
		}
		out.Shares = append(out.Shares, s.shareEntry(c, it, info, names))
	}
	return out, nil
}

func (c *call) statID(id *provider.ResourceId) (*provider.ResourceInfo, error) {
	res, err := c.gw.Stat(c.ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: id}})
	if err != nil {
		return nil, asToolError(err, "the shared resource")
	}
	if err := checkStatus(res.GetStatus(), "the shared resource"); err != nil {
		return nil, err
	}
	return res.GetInfo(), nil
}

func (s *svc) shareEntry(c *call, it shareItem, info *provider.ResourceInfo, names map[string]string) shareEntry {
	e := shareEntry{ID: it.id, Path: info.GetPath(), WebURL: s.webURL(info.GetPath(), info.GetId())}
	if l := it.link; l != nil {
		e.Kind = "link"
		e.Role = permissions.RoleFromResourcePermissions(l.GetPermissions().GetPermissions()).Name
		e.URL = s.c.WebBaseURL + "/s/" + l.GetToken()
		e.PasswordProtected = l.GetPasswordProtected()
		if l.GetExpiration() != nil {
			e.Expiry = formatTime(l.GetExpiration())
		}
		return e
	}
	sh := it.share
	e.Role = permissions.RoleFromResourcePermissions(sh.GetPermissions().GetPermissions()).Name
	if g := sh.GetGrantee(); g.GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP {
		e.Kind = "group"
		e.Grantee = "group:" + g.GetGroupId().GetOpaqueId()
	} else {
		e.Kind = "user"
		e.Grantee = c.username(g.GetUserId(), names)
	}
	if rs := it.received; rs != nil {
		e.Owner = c.username(sh.GetOwner(), names)
		e.State = strings.ToLower(strings.TrimPrefix(rs.GetState().String(), "SHARE_STATE_"))
	}
	return e
}

// username resolves a user id for display, falling back to the id.
func (c *call) username(id *userpb.UserId, names map[string]string) string {
	if id == nil {
		return ""
	}
	if n, ok := names[id.GetOpaqueId()]; ok {
		return n
	}
	n := id.GetOpaqueId()
	if res, err := c.gw.GetUser(c.ctx, &userpb.GetUserRequest{UserId: id, SkipFetchingUserGroups: true}); err == nil && res.GetUser().GetUsername() != "" {
		n = res.GetUser().GetUsername()
	}
	names[id.GetOpaqueId()] = n
	return n
}

type createShareIn struct {
	Path    string `json:"path" jsonschema:"absolute path of the file or folder"`
	Grantee string `json:"grantee" jsonschema:"username, such as jdoe, or group:<name>"`
	Role    string `json:"role,omitempty" jsonschema:"viewer (the default) or editor"`
}

func shareRole(name string, allowed ...string) (*permissions.Role, error) {
	if name == "" {
		name = permissions.RoleViewer
	}
	if slices.Contains(allowed, name) {
		return permissions.RoleFromName(name), nil
	}
	return nil, newError(codeInvalidArgument, fmt.Sprintf("role is one of %s", strings.Join(allowed, ", ")))
}

func (s *svc) createShare(c *call, in createShareIn) (shareEntry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return shareEntry{}, err
	}
	c.path = p
	role, err := shareRole(in.Role, permissions.RoleViewer, permissions.RoleEditor)
	if err != nil {
		return shareEntry{}, err
	}
	grantee, err := c.grantee(in.Grantee)
	if err != nil {
		return shareEntry{}, err
	}
	info, err := c.stat(p)
	if err != nil {
		return shareEntry{}, err
	}
	res, err := c.gw.CreateShare(c.ctx, &collaboration.CreateShareRequest{
		ResourceInfo: info,
		Grant: &collaboration.ShareGrant{
			Grantee:     grantee,
			Permissions: &collaboration.SharePermissions{Permissions: role.CS3ResourcePermissions()},
		},
	})
	if err != nil {
		return shareEntry{}, asToolError(err, "share of "+p)
	}
	if err := checkStatus(res.GetStatus(), "share of "+p); err != nil {
		return shareEntry{}, err
	}
	return s.shareEntry(c, shareItem{id: res.GetShare().GetId().GetOpaqueId(), share: res.GetShare()}, info, map[string]string{}), nil
}

// grantee resolves a username, or group:<name>, by its exact name.
func (c *call) grantee(name string) (*provider.Grantee, error) {
	if group, ok := strings.CutPrefix(name, "group:"); ok {
		res, err := c.gw.GetGroupByClaim(c.ctx, &grouppb.GetGroupByClaimRequest{Claim: "group_name", Value: group, SkipFetchingMembers: true})
		if err != nil {
			return nil, asToolError(err, "group "+group)
		}
		if err := checkStatus(res.GetStatus(), "group "+group); err != nil {
			return nil, err
		}
		return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_GROUP, Id: &provider.Grantee_GroupId{GroupId: res.GetGroup().GetId()}}, nil
	}
	if name == "" {
		return nil, newError(codeInvalidArgument, "grantee is a username or group:<name>")
	}
	res, err := c.gw.GetUserByClaim(c.ctx, &userpb.GetUserByClaimRequest{Claim: "username", Value: name, SkipFetchingUserGroups: true})
	if err != nil {
		return nil, asToolError(err, "user "+name)
	}
	if err := checkStatus(res.GetStatus(), "user "+name); err != nil {
		return nil, err
	}
	return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: res.GetUser().GetId()}}, nil
}

type createPublicLinkIn struct {
	Path     string `json:"path" jsonschema:"absolute path of the file or folder"`
	Role     string `json:"role,omitempty" jsonschema:"viewer (the default), editor or uploader"`
	Expiry   string `json:"expiry,omitempty" jsonschema:"when the link stops working: a date, YYYY-MM-DD, or an RFC 3339 time"`
	Password string `json:"password,omitempty" jsonschema:"password needed to open the link"`
}

func parseExpiry(s string) (*types.Timestamp, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		d, derr := time.Parse(time.DateOnly, s)
		if derr != nil {
			return nil, newError(codeInvalidArgument, "expiry is a date, YYYY-MM-DD, or an RFC 3339 time")
		}
		t = d.Add(24*time.Hour - time.Second)
	}
	if !t.After(time.Now()) {
		return nil, newError(codeInvalidArgument, "expiry is in the past")
	}
	return &types.Timestamp{Seconds: uint64(t.Unix())}, nil
}

func (s *svc) createPublicLink(c *call, in createPublicLinkIn) (shareEntry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return shareEntry{}, err
	}
	c.path = p
	role, err := shareRole(in.Role, permissions.RoleViewer, permissions.RoleEditor, permissions.RoleUploader)
	if err != nil {
		return shareEntry{}, err
	}
	exp, err := parseExpiry(in.Expiry)
	if err != nil {
		return shareEntry{}, err
	}
	info, err := c.stat(p)
	if err != nil {
		return shareEntry{}, err
	}
	if role.Name == permissions.RoleUploader && info.GetType() != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return shareEntry{}, newError(codeInvalidArgument, "an uploader link needs a folder")
	}
	// Public share managers read the link's display name from here.
	info.ArbitraryMetadata = &provider.ArbitraryMetadata{Metadata: map[string]string{"name": "", "quicklink": "false"}}
	res, err := c.gw.CreatePublicShare(c.ctx, &link.CreatePublicShareRequest{
		ResourceInfo: info,
		Grant: &link.Grant{
			Permissions: &link.PublicSharePermissions{Permissions: role.CS3ResourcePermissions()},
			Password:    in.Password,
			Expiration:  exp,
		},
	})
	if err != nil {
		return shareEntry{}, asToolError(err, "public link to "+p)
	}
	if err := checkStatus(res.GetStatus(), "public link to "+p); err != nil {
		return shareEntry{}, err
	}
	l := res.GetShare()
	return s.shareEntry(c, shareItem{id: linkIDPrefix + l.GetId().GetOpaqueId(), link: l}, info, map[string]string{}), nil
}

type removeShareIn struct {
	ShareID string `json:"share_id" jsonschema:"id of the share or public link"`
}

type removeShareOut struct {
	ShareID string `json:"share_id"`
	Removed bool   `json:"removed"`
}

func (s *svc) removeShare(c *call, in removeShareIn) (removeShareOut, error) {
	if in.ShareID == "" {
		return removeShareOut{}, newError(codeInvalidArgument, "share_id is required")
	}
	if id, ok := strings.CutPrefix(in.ShareID, linkIDPrefix); ok {
		ref := &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}}}
		if len(c.ap.Roots) > 0 {
			res, err := c.gw.GetPublicShare(c.ctx, &link.GetPublicShareRequest{Ref: ref})
			if err != nil {
				return removeShareOut{}, asToolError(err, "public link "+id)
			}
			if err := checkStatus(res.GetStatus(), "public link "+id); err != nil {
				return removeShareOut{}, err
			}
			if err := c.resourceUnderRoots(res.GetShare().GetResourceId()); err != nil {
				return removeShareOut{}, err
			}
		}
		res, err := c.gw.RemovePublicShare(c.ctx, &link.RemovePublicShareRequest{Ref: ref})
		if err != nil {
			return removeShareOut{}, asToolError(err, "public link "+id)
		}
		if err := checkStatus(res.GetStatus(), "public link "+id); err != nil {
			return removeShareOut{}, err
		}
		return removeShareOut{ShareID: in.ShareID, Removed: true}, nil
	}

	ref := &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: in.ShareID}}}
	if len(c.ap.Roots) > 0 {
		res, err := c.gw.GetShare(c.ctx, &collaboration.GetShareRequest{Ref: ref})
		if err != nil {
			return removeShareOut{}, asToolError(err, "share "+in.ShareID)
		}
		if err := checkStatus(res.GetStatus(), "share "+in.ShareID); err != nil {
			return removeShareOut{}, err
		}
		if err := c.resourceUnderRoots(res.GetShare().GetResourceId()); err != nil {
			return removeShareOut{}, err
		}
	}
	res, err := c.gw.RemoveShare(c.ctx, &collaboration.RemoveShareRequest{Ref: ref})
	if err != nil {
		return removeShareOut{}, asToolError(err, "share "+in.ShareID)
	}
	if err := checkStatus(res.GetStatus(), "share "+in.ShareID); err != nil {
		return removeShareOut{}, err
	}
	return removeShareOut{ShareID: in.ShareID, Removed: true}, nil
}

func (c *call) resourceUnderRoots(id *provider.ResourceId) error {
	info, err := c.statID(id)
	if err != nil {
		return err
	}
	c.path = info.GetPath()
	if !c.ap.UnderRoots(info.GetPath()) {
		return newError(codeOutsideRoots, "the shared resource is outside the folders this app password is restricted to")
	}
	return nil
}
