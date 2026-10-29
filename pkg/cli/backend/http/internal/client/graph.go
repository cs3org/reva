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

package client

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// User is the authenticated identity.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Mail        string `json:"mail,omitempty"`
}

// Space is one CERNBox space (a "drive" in Graph terms).
type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Alias is the drive alias. At CERN it is the space root path without the
	// leading slash.
	Alias     string `json:"alias,omitempty"`
	Path      string `json:"path"`
	WebDavURL string `json:"webdav_url,omitempty"`

	QuotaTotal     int64 `json:"quota_total,omitempty"`
	QuotaUsed      int64 `json:"quota_used,omitempty"`
	QuotaRemaining int64 `json:"quota_remaining,omitempty"`

	// Owner is the account the space was created for. For a project, this is not
	// everybody who can write to it.
	Owner string `json:"owner,omitempty"`
	// YourRole is the caller's own role on the space root. The server does not
	// say which groups give it.
	YourRole string `json:"your_role,omitempty"`
	// RootPermissions is what the server sent, kept whole to debug a wrong role.
	RootPermissions []Permission `json:"root_permissions,omitempty"`
}

// Unified role ids, as defined by libregraph and accepted by ocgraph.
const (
	RoleViewer      = "b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"
	RoleSpaceViewer = "a8d5fe5e-96e3-418d-825b-534dbdf22b99"
	RoleEditor      = "fb6c3e19-e378-47e5-b277-9732f9de6e21"
	RoleSpaceEditor = "58c63c02-1d89-4572-916a-870abc5a1b7d"
	RoleFileEditor  = "2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"
	RoleManager     = "312c0871-5ef7-4b3a-85b6-0e4074c64049"
	RoleDenied      = "5d3754fa-a6a6-4985-b0af-dd1359e5d616"
)

var roleNames = map[string]string{
	"viewer":  RoleViewer,
	"reader":  RoleViewer,
	"editor":  RoleEditor,
	"writer":  RoleEditor,
	"manager": RoleManager,
	// Name that scripts may still pass for manager.
	"collab": RoleManager,
	"denied": RoleDenied,
}

// RoleID translates a short role name to a unified role id.
func RoleID(name string) (string, error) {
	if id, ok := roleNames[strings.ToLower(name)]; ok {
		return id, nil
	}
	return "", cberr.Usagef("unknown role %q: want viewer, editor, manager, or denied", name)
}

// RoleName translates a unified role id to the CLI's short name. An unknown id
// is returned as it is.
func RoleName(id string) string {
	switch id {
	case RoleViewer, RoleSpaceViewer:
		return "viewer"
	case RoleEditor, RoleSpaceEditor, RoleFileEditor:
		return "editor"
	case RoleManager:
		return "manager"
	case RoleDenied:
		return "denied"
	default:
		return id
	}
}

// Identity is a share recipient or link creator.
type Identity struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Type        string `json:"type,omitempty"` // "user" or "group"
}

// Link describes a public link permission.
type Link struct {
	Type        string `json:"type,omitempty"` // "view" or "edit"
	URL         string `json:"url,omitempty"`
	HasPassword bool   `json:"has_password,omitempty"`
	Name        string `json:"name,omitempty"`
}

// Permission is a share or a public link on a resource.
type Permission struct {
	ID        string     `json:"id"`
	Role      string     `json:"role,omitempty"`
	GrantedTo *Identity  `json:"granted_to,omitempty"`
	Link      *Link      `json:"link,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Path      string     `json:"path,omitempty"`
	Created   *time.Time `json:"created,omitempty"`
	InvitedBy *Identity  `json:"invited_by,omitempty"`
}

type graphUser struct {
	ID                       *string `json:"id"`
	DisplayName              string  `json:"displayName"`
	Mail                     *string `json:"mail"`
	OnPremisesSamAccountName string  `json:"onPremisesSamAccountName"`
}

// Me returns the authenticated user. It fetches the user once, because every
// WebDAV URL needs the username.
func (c *Client) Me(ctx context.Context) (*User, error) {
	c.meOnce.Do(func() {
		c.me, c.meErr = c.fetchMe(ctx)
	})
	return c.me, c.meErr
}

func (c *Client) fetchMe(ctx context.Context) (*User, error) {
	var gu graphUser
	if err := c.getJSON(ctx, c.URL(graphV1+"/me"), "identify the current user", "", &gu); err != nil {
		// A 404 here is a missing API, not a missing file.
		if cberr.KindOf(err) == cberr.KindNotFound {
			return nil, cberr.New(cberr.KindOther, "identify the current user", "",
				"this server does not expose the graph API at "+graphV1+"/me")
		}
		return nil, err
	}
	u := &User{
		DisplayName: gu.DisplayName,
		Username:    gu.OnPremisesSamAccountName,
	}
	if gu.ID != nil {
		u.ID = *gu.ID
	}
	if gu.Mail != nil {
		u.Mail = *gu.Mail
	}
	if u.Username == "" {
		return nil, cberr.New(cberr.KindOther, "identify the current user", "",
			"the server did not return a username")
	}
	return u, nil
}

type graphDrive struct {
	ID         *string `json:"id"`
	Name       string  `json:"name"`
	DriveType  *string `json:"driveType"`
	DriveAlias *string `json:"driveAlias"`
	Root       *struct {
		WebDavURL   *string           `json:"webDavUrl"`
		Permissions []graphPermission `json:"permissions"`
	} `json:"root"`
	Owner *struct {
		User *struct {
			ID *string `json:"id"`
		} `json:"user"`
	} `json:"owner"`
	Quota *struct {
		Total     *int64 `json:"total"`
		Used      *int64 `json:"used"`
		Remaining *int64 `json:"remaining"`
	} `json:"quota"`
}

type graphCollection[T any] struct {
	Value []T `json:"value"`
}

// roleOfCaller picks the role of the asking user. A single unattributed grant
// counts as the caller's: a project root has that shape when the server names
// nobody.
func roleOfCaller(perms []Permission, me *User) string {
	if me != nil {
		for _, p := range perms {
			if p.GrantedTo == nil || p.Role == "" {
				continue
			}
			if p.GrantedTo.ID == me.Username || p.GrantedTo.ID == me.ID {
				return p.Role
			}
		}
	}

	var only string
	for _, p := range perms {
		if p.Role == "" {
			continue
		}
		if p.GrantedTo != nil && me != nil {
			continue
		}
		if only != "" {
			return "" // cannot tell which one is ours
		}
		only = p.Role
	}
	return only
}

// Spaces returns the caller's spaces, cached for the configured TTL.
func (c *Client) Spaces(ctx context.Context) ([]Space, error) {
	c.spacesMu.Lock()
	defer c.spacesMu.Unlock()

	if c.spacesOnce && time.Since(c.spacesAt) < c.spacesTTL {
		return c.spaces, c.spacesErr
	}

	// A failure here only costs the role, so do not fail the listing.
	var me *User
	if u, err := c.Me(ctx); err == nil {
		me = u
	}

	var coll graphCollection[graphDrive]
	err := c.getJSON(ctx, c.URL(graphBeta+"/me/drives"), "list spaces", "", &coll)
	if err != nil {
		c.spaces, c.spacesErr, c.spacesAt, c.spacesOnce = nil, err, time.Now(), true
		return nil, err
	}

	spaces := make([]Space, 0, len(coll.Value))
	for _, d := range coll.Value {
		s := Space{Name: d.Name}
		if d.ID != nil {
			s.ID = *d.ID
		}
		if d.DriveType != nil {
			s.Type = *d.DriveType
		}
		if d.DriveAlias != nil {
			s.Alias = *d.DriveAlias
			s.Path = cleanPath("/" + *d.DriveAlias)
		}
		if d.Root != nil {
			if d.Root.WebDavURL != nil {
				s.WebDavURL = *d.Root.WebDavURL
			}
			s.RootPermissions = toPermissions(d.Root.Permissions)
			s.YourRole = roleOfCaller(s.RootPermissions, me)
		}
		if d.Owner != nil && d.Owner.User != nil && d.Owner.User.ID != nil {
			s.Owner = *d.Owner.User.ID
		}
		if d.Quota != nil {
			if d.Quota.Total != nil {
				s.QuotaTotal = *d.Quota.Total
			}
			if d.Quota.Used != nil {
				s.QuotaUsed = *d.Quota.Used
			}
			if d.Quota.Remaining != nil {
				s.QuotaRemaining = *d.Quota.Remaining
			}
		}
		spaces = append(spaces, s)
	}

	c.spaces, c.spacesErr, c.spacesAt, c.spacesOnce = spaces, nil, time.Now(), true
	return spaces, nil
}

// ResolveSpace maps a space alias to the absolute path of the space root. It
// accepts "home", "project/<name>", "<name>", the drive alias and the space id.
func (c *Client) ResolveSpace(ctx context.Context, alias string) (string, error) {
	spaces, err := c.Spaces(ctx)
	if err != nil {
		return "", err
	}

	want := strings.ToLower(strings.Trim(alias, "/"))
	projectName := strings.TrimPrefix(want, "project/")

	var personal *Space
	for i := range spaces {
		s := &spaces[i]
		if s.Type == "personal" && personal == nil {
			personal = s
		}
		switch {
		case strings.EqualFold(s.ID, alias),
			strings.EqualFold(s.Alias, want),
			strings.EqualFold(s.Name, want):
			return spaceRoot(s)
		case s.Type == "project" && strings.EqualFold(s.Name, projectName):
			return spaceRoot(s)
		}
	}

	if want == "home" || want == "personal" {
		if personal == nil {
			return "", cberr.New(cberr.KindNotFound, "resolve space", alias,
				"you have no personal space on this instance")
		}
		return spaceRoot(personal)
	}

	return "", cberr.New(cberr.KindNotFound, "resolve space", alias,
		fmt.Sprintf("no space named %q: 'space list' shows what you have", alias))
}

func spaceRoot(s *Space) (string, error) {
	if s.Path == "" {
		return "", cberr.New(cberr.KindOther, "resolve space", s.Name,
			"the server did not report a path for this space")
	}
	return s.Path, nil
}

// SpacePathOfID returns the path of the space that a resource belongs to.
//
// A resource id is "storage$space!item", and the space part is the base32 of
// the space path. So "localhome$F5SW64ZPOVZWK4RPMUXWK2LOON2GK2LO!3857" is in
// the space /eos/user/e/einstein. A received share has no path of its own, so
// this is the only way to find it. An id that does not decode gives false,
// because a guessed path could point at somebody else's directory.
func SpacePathOfID(resourceID string) (string, bool) {
	spaceID, _, ok := splitResourceID(resourceID)
	if !ok {
		return "", false
	}
	_, encoded, ok := strings.Cut(spaceID, "$")
	if !ok || encoded == "" {
		return "", false
	}
	// reva writes the padding in some ids and not in others.
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.TrimRight(encoded, "="))
	if err != nil {
		return "", false
	}
	p := string(raw)
	if !strings.HasPrefix(p, "/") {
		return "", false
	}
	return path.Clean(p), true
}

// splitResourceID splits "storage$space!item" into the space id and the full id.
func splitResourceID(id string) (spaceID, itemID string, ok bool) {
	spaceID, itemID, ok = strings.Cut(id, "!")
	if !ok || spaceID == "" || itemID == "" {
		return "", "", false
	}
	return spaceID, id, true
}

func (c *Client) itemURL(resourceID, suffix string) (string, error) {
	spaceID, fullID, ok := splitResourceID(resourceID)
	if !ok {
		return "", cberr.New(cberr.KindOther, "address resource", resourceID,
			"the server returned a resource id in a form this client does not understand")
	}
	return c.URL(fmt.Sprintf("%s/drives/%s/items/%s%s",
		graphBeta, url.PathEscape(spaceID), url.PathEscape(fullID), suffix)), nil
}

// Recipient names a share target.
type Recipient struct {
	ID   string // username or group name
	Type string // "user" or "group"
}

type inviteRequest struct {
	Recipients []inviteRecipient `json:"recipients"`
	Roles      []string          `json:"roles"`
	// Omitted when unset: ocgraph rejects a null.
	Expiration *time.Time `json:"expirationDateTime,omitempty"`
}

type inviteRecipient struct {
	ObjectID string `json:"objectId"`
	Type     string `json:"@libre.graph.recipient.type"`
}

type graphPermission struct {
	ID          *string  `json:"id"`
	Roles       []string `json:"roles"`
	GrantedToV2 *struct {
		User  *graphIdentity `json:"user"`
		Group *graphIdentity `json:"group"`
	} `json:"grantedToV2"`
	GrantedTo *struct {
		User  *graphIdentity `json:"user"`
		Group *graphIdentity `json:"group"`
	} `json:"grantedTo"`
	Link *struct {
		Type             *string `json:"type"`
		WebURL           *string `json:"webUrl"`
		PreventsDownload *bool   `json:"preventsDownload"`
		HasPassword      *bool   `json:"@libre.graph.permissions.link.hasPassword"`
		DisplayName      *string `json:"@libre.graph.displayName"`
	} `json:"link"`
	ExpirationDateTime *time.Time `json:"expirationDateTime"`
	CreatedDateTime    *time.Time `json:"createdDateTime"`
	// reva puts the password flag of a link here, outside the link object.
	HasPassword *bool `json:"hasPassword"`
	Invitation  *struct {
		InvitedBy *struct {
			User *graphIdentity `json:"user"`
		} `json:"invitedBy"`
	} `json:"invitation"`
}

type graphIdentity struct {
	ID          *string `json:"id"`
	DisplayName *string `json:"displayName"`
}

func (p graphPermission) toPermission() Permission {
	out := Permission{}
	if p.ID != nil {
		out.ID = *p.ID
	}
	if len(p.Roles) > 0 {
		out.Role = RoleName(p.Roles[0])
	}
	granted := p.GrantedToV2
	if granted == nil {
		granted = p.GrantedTo
	}
	if granted != nil {
		switch {
		case granted.User != nil:
			out.GrantedTo = identityOf(granted.User, "user")
		case granted.Group != nil:
			out.GrantedTo = identityOf(granted.Group, "group")
		}
	}
	if p.Link != nil {
		l := &Link{}
		if p.Link.Type != nil {
			l.Type = *p.Link.Type
		}
		if p.Link.WebURL != nil {
			l.URL = *p.Link.WebURL
		}
		if p.Link.HasPassword != nil {
			l.HasPassword = *p.Link.HasPassword
		} else if p.HasPassword != nil {
			l.HasPassword = *p.HasPassword
		}
		if p.Link.DisplayName != nil {
			l.Name = *p.Link.DisplayName
		}
		out.Link = l
	}
	out.ExpiresAt = p.ExpirationDateTime
	out.Created = p.CreatedDateTime
	if p.Invitation != nil && p.Invitation.InvitedBy != nil && p.Invitation.InvitedBy.User != nil {
		out.InvitedBy = identityOf(p.Invitation.InvitedBy.User, "user")
	}
	return out
}

func identityOf(id *graphIdentity, kind string) *Identity {
	out := &Identity{Type: kind}
	if id.ID != nil {
		out.ID = *id.ID
	}
	if id.DisplayName != nil {
		out.DisplayName = *id.DisplayName
	}
	return out
}

// Share grants recipients a role on a resource.
func (c *Client) Share(ctx context.Context, resourceID string, recipients []Recipient, role string, expiry *time.Time) ([]Permission, error) {
	u, err := c.itemURL(resourceID, "/invite")
	if err != nil {
		return nil, err
	}
	req := inviteRequest{Roles: []string{role}, Expiration: expiry}
	for _, r := range recipients {
		req.Recipients = append(req.Recipients, inviteRecipient{ObjectID: r.ID, Type: r.Type})
	}

	var coll graphCollection[graphPermission]
	if err := c.postJSON(ctx, u, "share", "", req, &coll); err != nil {
		return nil, err
	}
	return toPermissions(coll.Value), nil
}

// ListPermissions returns the shares and links on a resource.
func (c *Client) ListPermissions(ctx context.Context, resourceID string) ([]Permission, error) {
	u, err := c.itemURL(resourceID, "/permissions")
	if err != nil {
		return nil, err
	}
	var coll graphCollection[graphPermission]
	if err := c.getJSON(ctx, u, "list shares", "", &coll); err != nil {
		return nil, err
	}
	return toPermissions(coll.Value), nil
}

// UpdatePermission changes the role, the expiry, or both on an existing share.
func (c *Client) UpdatePermission(ctx context.Context, resourceID, permissionID, role string, expiry *time.Time) (*Permission, error) {
	u, err := c.itemURL(resourceID, "/permissions/"+url.PathEscape(permissionID))
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if role != "" {
		body["roles"] = []string{role}
	}
	if expiry != nil {
		body["expirationDateTime"] = expiry
	}
	if len(body) == 0 {
		return nil, cberr.Usagef("nothing to update: pass --role or --expiry")
	}

	var out graphPermission
	if err := c.sendJSON(ctx, http.MethodPatch, u, "update share", permissionID, body, &out); err != nil {
		return nil, err
	}
	p := out.toPermission()
	return &p, nil
}

// LinkOptions configures a public link.
type LinkOptions struct {
	Type        string // "view" or "edit"
	DisplayName string
	Password    string
	Expiry      *time.Time
}

// CreateLink creates a public link on a resource.
func (c *Client) CreateLink(ctx context.Context, resourceID string, opts LinkOptions) (*Permission, error) {
	u, err := c.itemURL(resourceID, "/createLink")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"type": opts.Type}
	if opts.DisplayName != "" {
		body["displayName"] = opts.DisplayName
	}
	if opts.Password != "" {
		body["password"] = opts.Password
	}
	if opts.Expiry != nil {
		body["expirationDateTime"] = opts.Expiry
	}

	var out graphPermission
	if err := c.postJSON(ctx, u, "create link", "", body, &out); err != nil {
		return nil, err
	}
	p := out.toPermission()
	return &p, nil
}

// DriveItem is an entry in a shared-with-me or shared-by-me listing.
type DriveItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size,omitempty"`
	Path string `json:"path,omitempty"`
	// SharedBy is who granted a received share.
	SharedBy *Identity `json:"shared_by,omitempty"`
	// SharedWith is who a share you made went to. It is empty for a public link,
	// which has no grantee.
	SharedWith []Identity `json:"shared_with,omitempty"`
	// Links counts the public links on the resource.
	Links    int    `json:"links,omitempty"`
	Role     string `json:"role,omitempty"`
	Accepted bool   `json:"accepted"`
	// ShareID identifies the caller's copy of a received share (a share jail id).
	// ID is the shared resource. Only ShareID works for an update.
	ShareID string `json:"share_id,omitempty"`
	// Hidden reports whether the caller hid this received share. The grant stays.
	Hidden      bool         `json:"hidden,omitempty"`
	Federated   bool         `json:"federated,omitempty"`
	Permissions []Permission `json:"-"`
}

type graphDriveItem struct {
	ID              *string `json:"id"`
	Name            *string `json:"name"`
	Size            *int64  `json:"size"`
	ParentReference *struct {
		// Relative to the space root.
		Path *string `json:"path"`
	} `json:"parentReference"`
	RemoteItem *struct {
		ID   *string `json:"id"`
		Name *string `json:"name"`
		Size *int64  `json:"size"`
		// Relative to the root of the space of the owner. It is the only path a
		// received share reports.
		Path *string `json:"path"`
		// A received share carries its grant here.
		Permissions []graphPermission `json:"permissions"`
	} `json:"remoteItem"`
	UIHidden          *bool             `json:"@UI.Hidden"`
	ClientSynchronize *bool             `json:"@client.synchronize"`
	Permissions       []graphPermission `json:"permissions"`
	CreatedBy         *struct {
		User *graphIdentity `json:"user"`
	} `json:"createdBy"`
}

func (d graphDriveItem) toDriveItem() DriveItem {
	out := DriveItem{}
	if d.ID != nil {
		out.ID = *d.ID
		// A received share has two ids. This one is the caller's copy, which an
		// update needs. The remote item overwrites ID below.
		out.ShareID = *d.ID
	}
	if d.Name != nil {
		out.Name = *d.Name
	}
	if d.Size != nil {
		out.Size = *d.Size
	}
	if d.RemoteItem != nil {
		if out.Name == "" && d.RemoteItem.Name != nil {
			out.Name = *d.RemoteItem.Name
		}
		if out.Size == 0 && d.RemoteItem.Size != nil {
			out.Size = *d.RemoteItem.Size
		}
		if d.RemoteItem.ID != nil {
			out.ID = *d.RemoteItem.ID
			// The id of a received OCM share has this prefix.
			out.Federated = strings.HasPrefix(*d.RemoteItem.ID, OCMReceivedPrefix)
		}
	}

	// Neither listing reports an absolute path. A received share has a path
	// relative to the space of its owner. A share you made has only the path of
	// the parent. The space root comes from the resource id.
	switch {
	case d.RemoteItem != nil && d.RemoteItem.Path != nil && *d.RemoteItem.Path != "":
		out.Path = *d.RemoteItem.Path
		if d.RemoteItem.ID != nil {
			if root, ok := SpacePathOfID(*d.RemoteItem.ID); ok {
				out.Path = path.Join(root, *d.RemoteItem.Path)
			}
		}
	case d.ID != nil && out.Name != "":
		parent := ""
		if d.ParentReference != nil && d.ParentReference.Path != nil {
			parent = *d.ParentReference.Path
		}
		if root, ok := SpacePathOfID(*d.ID); ok {
			out.Path = path.Join(root, parent, out.Name)
		} else if parent != "" {
			out.Path = path.Join(parent, out.Name)
		}
	}

	if d.CreatedBy != nil && d.CreatedBy.User != nil {
		out.SharedBy = identityOf(d.CreatedBy.User, "user")
	}
	// A received share carries its grant on the remote item.
	perms := d.Permissions
	if len(perms) == 0 && d.RemoteItem != nil {
		perms = d.RemoteItem.Permissions
	}
	if len(perms) > 0 && len(perms[0].Roles) > 0 {
		out.Role = RoleName(perms[0].Roles[0])
	}
	// A share you made has no createdBy, so the recipients come from the
	// permissions.
	out.Permissions = toPermissions(perms)
	for _, p := range perms {
		switch converted := p.toPermission(); {
		case converted.GrantedTo != nil:
			out.SharedWith = append(out.SharedWith, *converted.GrantedTo)
		case converted.Link != nil:
			out.Links++
		}
	}
	out.Accepted = d.ClientSynchronize != nil && *d.ClientSynchronize
	out.Hidden = d.UIHidden != nil && *d.UIHidden
	return out
}

// SharedWithMe lists shares other people have granted to the caller.
func (c *Client) SharedWithMe(ctx context.Context) ([]DriveItem, error) {
	return c.driveItems(ctx, c.URL(graphBeta+"/me/drive/sharedWithMe"), "list received shares")
}

// SharedByMe lists shares the caller has granted to other people.
func (c *Client) SharedByMe(ctx context.Context) ([]DriveItem, error) {
	return c.driveItems(ctx, c.URL(graphBeta+"/me/drive/sharedByMe"), "list shares")
}

func (c *Client) driveItems(ctx context.Context, u, op string) ([]DriveItem, error) {
	var coll graphCollection[graphDriveItem]
	if err := c.getJSON(ctx, u, op, "", &coll); err != nil {
		return nil, err
	}
	out := make([]DriveItem, 0, len(coll.Value))
	for _, item := range coll.Value {
		out = append(out, item.toDriveItem())
	}
	return out, nil
}

// HideReceivedShare hides a received share from the caller's listings. The
// grant stays, and the share stays in sharedWithMe with the flag set.
//
// shareID must be DriveItem.ShareID. reva answers 404 for a resource id.
func (c *Client) HideReceivedShare(ctx context.Context, shareID string) error {
	u, err := c.itemURL(shareID, "")
	if err != nil {
		return err
	}
	return c.sendJSON(ctx, http.MethodPatch, u, "hide received share", shareID,
		map[string]any{"@UI.Hidden": true}, nil)
}

// SetReceivedShareState accepts or declines a received share.
//
// In reva this is the same field as hidden, so a decline hides the share. With
// storage ACLs it does not necessarily remove the access.
//
// id may be the share id or the resource id.
func (c *Client) SetReceivedShareState(ctx context.Context, id string, accept bool) error {
	shareID, err := c.receivedShareID(ctx, id)
	if err != nil {
		return err
	}
	u, err := c.itemURL(shareID, "")
	if err != nil {
		return err
	}
	body := map[string]any{
		"@UI.Hidden":          !accept,
		"@client.synchronize": accept,
	}
	return c.sendJSON(ctx, http.MethodPatch, u, "update received share", id, body, nil)
}

// receivedShareID maps either id of a received share to the one an update takes.
func (c *Client) receivedShareID(ctx context.Context, id string) (string, error) {
	items, err := c.SharedWithMe(ctx)
	if err != nil {
		return "", err
	}

	resolved := ""
	for _, it := range items {
		if it.ShareID == id {
			return id, nil
		}
		if it.ID == id && it.ShareID != "" {
			resolved = it.ShareID
		}
	}
	if resolved != "" {
		return resolved, nil
	}
	return "", cberr.New(cberr.KindNotFound, "update received share", id,
		"nothing with this id has been shared with you. 'share received' lists what has")
}

func toPermissions(in []graphPermission) []Permission {
	out := make([]Permission, 0, len(in))
	for _, p := range in {
		out = append(out, p.toPermission())
	}
	return out
}

func (c *Client) getJSON(ctx context.Context, u, op, path string, out any) error {
	resp, err := c.do(ctx, request{
		method: http.MethodGet,
		url:    u,
		header: http.Header{"Accept": []string{"application/json"}},
		op:     op,
		path:   path,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp.Body, out, op, path)
}

func (c *Client) postJSON(ctx context.Context, u, op, path string, in, out any) error {
	return c.sendJSON(ctx, http.MethodPost, u, op, path, in, out)
}

func (c *Client) sendJSON(ctx context.Context, method, u, op, path string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return cberr.Wrap(cberr.KindOther, op, path, err)
	}
	resp, err := c.do(ctx, request{
		method: method,
		url:    u,
		header: http.Header{
			"Content-Type": []string{"application/json"},
			"Accept":       []string{"application/json"},
		},
		body: stringBody(string(payload)),
		op:   op,
		path: path,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil
	}
	return decodeJSON(resp.Body, out, op, path)
}

func decodeJSON(r io.Reader, out any, op, path string) error {
	if err := json.NewDecoder(r).Decode(out); err != nil {
		return cberr.Wrap(cberr.KindOther, op, path, fmt.Errorf("malformed server response: %w", err))
	}
	return nil
}
