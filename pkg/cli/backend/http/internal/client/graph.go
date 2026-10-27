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
	// Type is "personal", "project", or another server-defined type.
	Type string `json:"type"`
	// Alias is the server's drive alias, which at CERN is the space root path
	// without its leading slash, for example "eos/user/g/gdelmont".
	Alias string `json:"alias,omitempty"`
	// Path is the absolute CERNBox path of the space root.
	Path string `json:"path"`
	// WebDavURL addresses the space root directly.
	WebDavURL string `json:"webdav_url,omitempty"`

	QuotaTotal     int64 `json:"quota_total,omitempty"`
	QuotaUsed      int64 `json:"quota_used,omitempty"`
	QuotaRemaining int64 `json:"quota_remaining,omitempty"`

	// Owner is the account the space belongs to, which for a project is the
	// account it was created for rather than anybody who can write to it.
	Owner string `json:"owner,omitempty"`
	// YourRole is the caller's own role on the space root, as a short name.
	//
	// The server computes it from the caller's group membership and does not
	// disclose the groups themselves, so this says what you may do and never who
	// else may do it.
	YourRole string `json:"your_role,omitempty"`
	// RootPermissions is every permission the server reported on the space root.
	//
	// Kept whole rather than reduced to YourRole, because when a role is not the
	// one somebody expected, the only way to tell a mis-read from a server that
	// really said that is to look at what arrived.
	RootPermissions []Permission `json:"root_permissions,omitempty"`
}

// Unified role identifiers, as defined by libregraph and accepted by ocgraph.
// The CLI exposes short names and translates here, so that users type
// --role editor rather than a UUID.
const (
	RoleViewer      = "b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"
	RoleSpaceViewer = "a8d5fe5e-96e3-418d-825b-534dbdf22b99"
	RoleEditor      = "fb6c3e19-e378-47e5-b277-9732f9de6e21"
	RoleSpaceEditor = "58c63c02-1d89-4572-916a-870abc5a1b7d"
	RoleFileEditor  = "2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"
	RoleManager     = "312c0871-5ef7-4b3a-85b6-0e4074c64049"
	RoleDenied      = "5d3754fa-a6a6-4985-b0af-dd1359e5d616"
)

// roleNames maps the CLI's short role names to unified role ids.
var roleNames = map[string]string{
	"viewer":  RoleViewer,
	"reader":  RoleViewer,
	"editor":  RoleEditor,
	"writer":  RoleEditor,
	"manager": RoleManager,
	// Kept so that anything already passing --role collab keeps working; it is
	// no longer what the role is called back.
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

// RoleName translates a unified role id back to the CLI's short name, falling
// back to the raw id so an unknown role is still displayed rather than hidden.
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
	// Type is "user" or "group".
	Type string `json:"type,omitempty"`
}

// Link describes a public link permission.
type Link struct {
	// Type is the libregraph link type, "view" or "edit".
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
	// HasPassword reports whether the link is password protected.
	HasPassword bool `json:"has_password,omitempty"`
	// Name is the label the link was given, for telling several apart.
	Name string `json:"name,omitempty"`
}

// Permission is a share or a public link on a resource.
type Permission struct {
	ID        string     `json:"id"`
	Role      string     `json:"role,omitempty"`
	GrantedTo *Identity  `json:"granted_to,omitempty"`
	Link      *Link      `json:"link,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Path is the resource the permission applies to, when the server reports
	// it. Listings fill it in so the user can tell shares apart.
	Path string `json:"path,omitempty"`
	// Created and InvitedBy are when and by whom the share was made, when
	// the server says it.
	Created   *time.Time `json:"created,omitempty"`
	InvitedBy *Identity  `json:"invited_by,omitempty"`
}

// -- identity -----------------------------------------------------------------

type graphUser struct {
	ID                       *string `json:"id"`
	DisplayName              string  `json:"displayName"`
	Mail                     *string `json:"mail"`
	OnPremisesSamAccountName string  `json:"onPremisesSamAccountName"`
}

// Me returns the authenticated user, fetching it at most once per process. The
// username is needed to build every path-addressed WebDAV URL, so this runs
// before almost any other call.
func (c *Client) Me(ctx context.Context) (*User, error) {
	c.meOnce.Do(func() {
		c.me, c.meErr = c.fetchMe(ctx)
	})
	return c.me, c.meErr
}

func (c *Client) fetchMe(ctx context.Context) (*User, error) {
	var gu graphUser
	if err := c.getJSON(ctx, c.URL(graphV1+"/me"), "identify the current user", "", &gu); err != nil {
		// A 404 here is not a missing file, it is a missing API. The default
		// "no such file or directory" sends whoever hits it looking for a path
		// that was never involved.
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

// -- spaces -------------------------------------------------------------------

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

// roleOfCaller picks the permission that belongs to the asking user.
//
// Taking the first one with a role was wrong the moment a space reported more
// than one: the answer would depend on the order the server happened to send
// them, and would silently be somebody else's role. A single unattributed grant
// is still read as the caller's, because that is the shape a project root has
// when the server names nobody.
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
			// Attributed to somebody, and not to us.
			continue
		}
		if only != "" {
			// More than one unattributed role: no way to tell which is ours.
			return ""
		}
		only = p.Role
	}
	return only
}

// Spaces returns the caller's spaces, cached for the configured TTL. The
// listing backs both "cernbox space list" and every space-alias resolution, so
// caching it keeps a shell loop from re-fetching it on each invocation.
func (c *Client) Spaces(ctx context.Context) ([]Space, error) {
	c.spacesMu.Lock()
	defer c.spacesMu.Unlock()

	if c.spacesOnce && time.Since(c.spacesAt) < c.spacesTTL {
		return c.spaces, c.spacesErr
	}

	// Who is asking, so that a space reporting several grants can be read for the
	// caller's own rather than for whichever came first. Cached, and a failure
	// here only costs the role, so it is not worth failing the listing over.
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

// ResolveSpace maps a space alias to the absolute path of that space's root,
// implementing pathspec.SpaceResolver.
//
// Several spellings are accepted, because users arrive from different places:
// "home" for the personal space, "project/<name>" or just "<name>" for a
// project, the server's own drive alias, and the raw space id that a script
// might have stored.
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

// SpaceAlias returns the alias the CLI displays for a space.
func SpaceAlias(s Space) string {
	switch s.Type {
	case "personal":
		return "home"
	case "project":
		return "project/" + s.Name
	default:
		if s.Alias != "" {
			return s.Alias
		}
		return s.Name
	}
}

// -- shares and links ---------------------------------------------------------

// SpacePathOfID recovers the path of the space a resource belongs to, from the
// resource id the graph reports.
//
// The id is "storage$space!item", and the space part is the base32 of the
// space's path: "localhome$F5SW64ZPOVZWK4RPMUXWK2LOON2GK2LO!3857" is an item in
// the space rooted at /eos/user/e/einstein. That encoding is reva's, not
// something this client invented, and the alternative is worse: a received share
// carries no path of its own, so without decoding this there is no way to reach
// what somebody has shared with you except by guessing at the layout of their
// home directory.
//
// A path that does not decode is reported rather than guessed at, since acting
// on a wrong one would mean reading somebody else's directory.
func SpacePathOfID(resourceID string) (string, bool) {
	spaceID, _, ok := splitResourceID(resourceID)
	if !ok {
		return "", false
	}
	_, encoded, ok := strings.Cut(spaceID, "$")
	if !ok || encoded == "" {
		return "", false
	}
	// reva writes the padding sometimes and omits it elsewhere, so decode
	// without it and trim whatever is there.
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

// splitResourceID splits a stringified resource id, "storage$space!item", into
// the space id the Graph route needs and the item id.
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
	// ID is the username or group name.
	ID string
	// Type is "user" or "group".
	Type string
}

type inviteRequest struct {
	Recipients []inviteRecipient `json:"recipients"`
	Roles      []string          `json:"roles"`
	// Expiration is omitted entirely when unset, because ocgraph decodes the
	// body with DisallowUnknownFields and a null would be rejected.
	Expiration *time.Time `json:"expirationDateTime,omitempty"`
}

type inviteRecipient struct {
	ObjectID string `json:"objectId"`
	Type     string `json:"@libre.graph.recipient.type"`
}

type graphPermission struct {
	ID          *string `json:"id"`
	Roles       []string
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
	// HasPassword is where reva puts the password flag of a link, outside
	// the link object.
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

// UnmarshalJSON decodes the roles array, which libregraph names "roles".
func (p *graphPermission) UnmarshalJSON(b []byte) error {
	type alias graphPermission
	var raw struct {
		alias
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*p = graphPermission(raw.alias)
	p.Roles = raw.Roles
	return nil
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

// RemovePermission deletes a share or link.
func (c *Client) RemovePermission(ctx context.Context, resourceID, permissionID string) error {
	u, err := c.itemURL(resourceID, "/permissions/"+url.PathEscape(permissionID))
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, request{
		method:  http.MethodDelete,
		url:     u,
		op:      "remove share",
		path:    permissionID,
		expects: []int{http.StatusNoContent, http.StatusOK},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// LinkOptions configures a public link.
type LinkOptions struct {
	// Type is "view" or "edit".
	Type        string
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

// LinkUpdate is what can be changed on a public link that already exists. An
// empty field leaves that part of the link alone.
type LinkUpdate struct {
	// Type is "view" or "edit".
	Type        string
	DisplayName string
	Expiry      *time.Time
	// ClearExpiry removes the expiry, which is a different request from not
	// mentioning it: the server distinguishes an absent field from an explicit
	// null, and only the null takes an expiry away.
	ClearExpiry bool
}

// Empty reports whether the update would change nothing.
func (u LinkUpdate) Empty() bool {
	return u.Type == "" && u.DisplayName == "" && u.Expiry == nil && !u.ClearExpiry
}

// UpdateLink changes the type, name or expiry of an existing public link.
//
// A link is a permission, but it is not updated like one: a share carries its
// role in "roles", whereas a link carries it in the link type, and the server
// rejects a body that mixes the two. It also refuses unknown fields outright,
// so each key here has to be one libregraph names.
func (c *Client) UpdateLink(ctx context.Context, resourceID, permissionID string, u LinkUpdate) (*Permission, error) {
	if u.Empty() {
		return nil, cberr.Usagef("nothing to update: pass --role, --name, --expiry or --no-expiry")
	}
	if u.Expiry != nil && u.ClearExpiry {
		return nil, cberr.Usagef("--expiry and --no-expiry ask for opposite things")
	}

	endpoint, err := c.itemURL(resourceID, "/permissions/"+url.PathEscape(permissionID))
	if err != nil {
		return nil, err
	}

	body := map[string]any{}
	link := map[string]any{}
	if u.Type != "" {
		link["type"] = u.Type
	}
	if u.DisplayName != "" {
		link["@libre.graph.displayName"] = u.DisplayName
	}
	if len(link) > 0 {
		body["link"] = link
	}
	switch {
	case u.Expiry != nil:
		body["expirationDateTime"] = u.Expiry
	case u.ClearExpiry:
		body["expirationDateTime"] = nil
	}

	var out graphPermission
	if err := c.sendJSON(ctx, http.MethodPatch, endpoint, "update link", permissionID, body, &out); err != nil {
		return nil, err
	}
	p := out.toPermission()
	return &p, nil
}

// SetLinkPassword sets or clears the password on an existing link.
func (c *Client) SetLinkPassword(ctx context.Context, resourceID, permissionID, password string) error {
	u, err := c.itemURL(resourceID, "/permissions/"+url.PathEscape(permissionID)+"/setPassword")
	if err != nil {
		return err
	}
	return c.postJSON(ctx, u, "set link password", permissionID,
		map[string]any{"password": password}, nil)
}

// DriveItem is an entry in a shared-with-me or shared-by-me listing.
type DriveItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size,omitempty"`
	// Path is the CERNBox path when the server reports one.
	Path string `json:"path,omitempty"`
	// SharedBy is who granted the share, for received shares.
	SharedBy *Identity `json:"shared_by,omitempty"`
	// SharedWith is who it was granted to, for shares you made. A resource is
	// listed once however many shares it carries, so there can be several: and
	// a public link has no grantee at all, which is why this can be empty on a
	// row that is nonetheless shared.
	SharedWith []Identity `json:"shared_with,omitempty"`
	// Links counts the public links on the resource, for the same listing: a row
	// with no grantee and a link is shared with whoever has the address.
	Links int `json:"links,omitempty"`
	// Role is the role the caller holds on the item.
	Role string `json:"role,omitempty"`
	// Accepted reports whether a received share has been accepted.
	Accepted bool `json:"accepted"`
	// ShareID identifies the caller's own copy of a received share, in the form
	// the graph calls a share jail id. It is not the same as ID, which names the
	// resource that was shared, and only this one can be used to update the
	// share: reva's handler refuses anything else.
	ShareID string `json:"share_id,omitempty"`
	// Hidden reports whether the caller has taken this received share out of
	// their own listings. It hides nothing else: the grant is untouched and the
	// resource stays readable.
	Hidden bool `json:"hidden,omitempty"`
	// Federated reports whether the share came from another OCM provider.
	Federated bool `json:"federated,omitempty"`
	// Permissions are the shares and the links of the item, with their ids.
	Permissions []Permission `json:"-"`
}

type graphDriveItem struct {
	ID              *string `json:"id"`
	Name            *string `json:"name"`
	Size            *int64  `json:"size"`
	ParentReference *struct {
		// Path is the parent's path relative to the space root, so the resource's
		// own path is this plus its name.
		Path *string `json:"path"`
	} `json:"parentReference"`
	RemoteItem *struct {
		ID   *string `json:"id"`
		Name *string `json:"name"`
		Size *int64  `json:"size"`
		// Path is where the resource sits in the space it belongs to, relative to
		// that space's root. It is the only path a received share reports.
		Path *string `json:"path"`
		// The grant lives here for a received share: the top-level item is the
		// entry in the caller's own virtual drive, and the permission belongs
		// to the resource it points at.
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
		// Kept before the remote item overwrites ID below. A received share has
		// two ids and they are not interchangeable: this one addresses the
		// caller's copy of the share, which is the only thing an update accepts.
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
			// reva encodes the id of a received OCM share with this prefix, so
			// it is what distinguishes a federated share from a local one in a
			// listing that holds both.
			out.Federated = strings.HasPrefix(*d.RemoteItem.ID, OCMReceivedPrefix)
		}
	}

	// The absolute path, which is the only kind worth showing: it is what can be
	// typed into another command. Neither listing reports one.
	//
	// A received share reports a path relative to the space it lives in, which is
	// somebody else's. A share you made reports no path for the resource at all ,
	// only its parent's, again relative to a space root: so the two are put
	// together from different pieces. The space root itself is recoverable from
	// the resource id in both cases.
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
	// Prefer the top-level permissions, and fall back to the remote item's,
	// which is where a received share actually carries its grant.
	perms := d.Permissions
	if len(perms) == 0 && d.RemoteItem != nil {
		perms = d.RemoteItem.Permissions
	}
	if len(perms) > 0 && len(perms[0].Roles) > 0 {
		out.Role = RoleName(perms[0].Roles[0])
	}
	// Who it went to. A share you made carries this and nothing else: there is
	// no createdBy on it, since you are the one who created it: so without
	// reading the permissions the recipient column has nothing in it at all.
	out.Permissions = toPermissions(perms)
	for _, p := range perms {
		switch converted := p.toPermission(); {
		case converted.GrantedTo != nil:
			out.SharedWith = append(out.SharedWith, *converted.GrantedTo)
		case converted.Link != nil:
			out.Links++
		}
	}
	// A received share that has been accepted is synchronised and not hidden.
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

// HideReceivedShare takes a received share out of the caller's own listings,
// and out of the web interface's.
//
// It hides and nothing more: the grant is untouched, so the resource stays
// readable and stays in the sharedWithMe listing with the flag set: which is
// what lets the clipboard keep finding a handover it has hidden.
//
// shareID must be DriveItem.ShareID rather than DriveItem.ID. reva's handler
// insists on the share jail form and answers 404 for a resource id, which is the
// same thing the listing reports as the item's id.
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
// In reva's model these are the same field as hidden: accepting sets the share's
// state to accepted and declining sets it to rejected, which is what the graph
// reports as @UI.Hidden. Declining therefore takes the share out of the caller's
// listings; on a deployment where sharing is done with storage ACLs it does not
// necessarily take away their access.
//
// id may be either of the two ids a received share has. The listing reports the
// resource that was shared, reva's handler accepts only the caller's own copy of
// the share, and resolving between them is not something to make the user's
// problem: before this, every accept and decline answered 404.
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

// receivedShareID maps whichever id the caller has onto the one an update takes.
//
// An id that is already a share's own id is returned untouched, so a caller that
// has the right one costs nothing but the listing it takes to confirm it.
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

// -- JSON helpers -------------------------------------------------------------

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
		body: bytesBody(payload),
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
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(r).Decode(out); err != nil {
		return cberr.Wrap(cberr.KindOther, op, path, fmt.Errorf("malformed server response: %w", err))
	}
	return nil
}
