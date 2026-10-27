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

package http

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend/http/internal/client"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// shareJail is the space of the received shares in Graph. A received share
// has the id <shareJail>$<shareJail>!<share id> there
// (internal/http/services/owncloud/ocgraph/drives.go).
const shareJail = "a0ca6a90-a365-4782-871e-d44447bbc668"

func jailID(id string) string { return shareJail + "$" + shareJail + "!" + id }

type shares struct{ b *Backend }

func resourceIDString(id *provider.ResourceId) string {
	return spaces.EncodeToStringifiedResourceID(id)
}

// parseID reads a resource id with its space (storage$space!item), or
// without one (storage!item), which OCS gives without spaces.
func parseID(s string) *provider.ResourceId {
	if id, ok := spaces.ParseResourceID(s); ok {
		return id
	}
	id, err := spaces.ResourceIdFromString(s)
	if err != nil {
		return nil
	}
	return id
}

// sharedByMe returns the items that the caller shared, each with its
// permissions. The listing of sharedByMe has no permission ids in reva, so
// they come from the permissions of each item.
func (b *Backend) sharedByMe(ctx context.Context) ([]client.DriveItem, error) {
	items, err := b.c.SharedByMe(ctx)
	if err != nil {
		return nil, conv(err)
	}
	for i := range items {
		if perms, err := b.c.ListPermissions(ctx, items[i].ID); err == nil {
			items[i].Permissions = perms
		}
	}
	return items, nil
}

// itemOf returns the Graph id of the item that the OCS share o is on: OCS
// gives it without its space when spaces are off.
func (b *Backend) itemOf(ctx context.Context, o *client.OCSShare) (string, error) {
	if _, ok := spaces.ParseResourceID(o.ItemSource); ok {
		return o.ItemSource, nil
	}
	want := parseID(o.ItemSource)
	items, err := b.c.SharedByMe(ctx)
	if err != nil {
		return "", conv(err)
	}
	for _, it := range items {
		if id := parseID(it.ID); id != nil && want != nil && id.StorageId == want.StorageId && id.OpaqueId == want.OpaqueId {
			return it.ID, nil
		}
	}
	return "", notFound("the item of share %s not found", o.ID)
}

func (s shares) List(ctx context.Context, resource *provider.ResourceId) ([]*collaboration.Share, error) {
	var out []*collaboration.Share
	add := func(rid *provider.ResourceId, perms []client.Permission) {
		for _, p := range perms {
			if p.GrantedTo != nil {
				out = append(out, graphShare(p, rid))
			}
		}
	}
	if resource != nil {
		perms, err := s.b.c.ListPermissions(ctx, resourceIDString(resource))
		if err != nil {
			return nil, conv(err)
		}
		add(resource, perms)
		return out, nil
	}
	items, err := s.b.sharedByMe(ctx)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		add(parseID(it.ID), it.Permissions)
	}
	return out, nil
}

// graphShare converts a Graph permission. Graph gives no mtime and no
// owner, and of the grantee only its id and its type: the idp and the user
// type stay empty. The permissions are those of the role.
func graphShare(p client.Permission, rid *provider.ResourceId) *collaboration.Share {
	s := &collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: p.ID},
		ResourceId:  rid,
		Permissions: &collaboration.SharePermissions{Permissions: rolePermissions(p.Role)},
		Grantee:     grantee(p.GrantedTo.Type, p.GrantedTo.ID),
	}
	if p.ExpiresAt != nil {
		s.Expiration = timestamp(*p.ExpiresAt)
	}
	if p.Created != nil {
		s.Ctime = timestamp(*p.Created)
	}
	if p.InvitedBy != nil {
		s.Creator = userID(p.InvitedBy.ID)
	}
	return s
}

func grantee(kind, id string) *provider.Grantee {
	if kind == "group" {
		return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id: &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: id}}}
	}
	return &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: id}}}
}

func rolePermissions(name string) *provider.ResourcePermissions {
	switch name {
	case "denied":
		return &provider.ResourcePermissions{}
	case "uploader":
		return permissions.NewUploaderRole().CS3ResourcePermissions()
	}
	return permissions.RoleFromName(name).CS3ResourcePermissions()
}

// unifiedRole returns the Graph role of the CS3 permissions p.
func unifiedRole(p *provider.ResourcePermissions) (string, error) {
	name := permissions.RoleFromResourcePermissions(p).Name
	switch name {
	case permissions.RoleUploader:
		return permissions.UnifiedRoleUploaderID, nil
	case permissions.RoleFileEditor:
		return permissions.UnifiedRoleFileEditorID, nil
	}
	id, err := client.RoleID(name)
	if err != nil {
		return "", invalid("these permissions match no role of the HTTP API: %v", err)
	}
	return id, nil
}

// ocsShare converts an OCS share. OCS has the creator, the owner and the
// ctime, but no mtime, and the idp of no user.
func ocsShare(o *client.OCSShare) *collaboration.Share {
	kind := "user"
	if o.ShareType == client.OCSShareGroup {
		kind = "group"
	}
	return &collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: o.ID},
		ResourceId:  parseID(o.ItemSource),
		Permissions: &collaboration.SharePermissions{Permissions: permissions.OcsPermissions(o.Permissions).AsCS3Permissions()},
		Grantee:     grantee(kind, o.ShareWith),
		Owner:       userID(o.UIDFileOwner),
		Creator:     userID(o.UIDOwner),
		Ctime:       seconds(o.STime),
		Expiration:  ocsTime(o.Expiration),
	}
}

func userID(id string) *userpb.UserId {
	if id == "" {
		return nil
	}
	return &userpb.UserId{OpaqueId: id}
}

func seconds(s uint64) *types.Timestamp {
	if s == 0 {
		return nil
	}
	return &types.Timestamp{Seconds: s}
}

// ocsTime parses the expiration of OCS. Its time of day is not reliable
// (conversions.timestampToExpiration writes the seconds for the minutes),
// so only the date counts.
func ocsTime(s string) *types.Timestamp {
	if len(s) < 10 {
		return nil
	}
	t, err := time.Parse(time.DateOnly, s[:10])
	if err != nil {
		return nil
	}
	return timestamp(t)
}

func (s shares) Get(ctx context.Context, id string) (*collaboration.Share, error) {
	o, err := s.b.c.GetShare(ctx, id)
	if err != nil {
		return nil, conv(err)
	}
	if o.ShareType != client.OCSShareUser && o.ShareType != client.OCSShareGroup {
		return nil, notFound("share %s not found", id)
	}
	return ocsShare(o), nil
}

func (s shares) Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error) {
	info := req.GetResourceInfo()
	if info.GetId() == nil {
		return nil, invalid("the resource to share has no id")
	}
	role, err := unifiedRole(req.GetGrant().GetPermissions().GetPermissions())
	if err != nil {
		return nil, err
	}
	g := req.GetGrant().GetGrantee()
	r := client.Recipient{ID: g.GetUserId().GetOpaqueId(), Type: "user"}
	if g.GetType() == provider.GranteeType_GRANTEE_TYPE_GROUP {
		r = client.Recipient{ID: g.GetGroupId().GetOpaqueId(), Type: "group"}
	}
	var exp *time.Time
	if e := req.GetGrant().GetExpiration(); e != nil {
		t := time.Unix(int64(e.Seconds), 0)
		exp = &t
	}
	perms, err := s.b.c.Share(ctx, resourceIDString(info.Id), []client.Recipient{r}, role, exp)
	if err != nil {
		return nil, conv(err)
	}
	for _, p := range perms {
		if p.GrantedTo != nil {
			return graphShare(p, info.Id), nil
		}
	}
	return nil, invalid("the server created no share")
}

// Update changes the permissions with OCS, by the id alone. OCS cannot set
// an expiration, so that goes to the Graph route of the item, which OCS
// tells.
func (s shares) Update(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.Share, error) {
	id := req.GetRef().GetId().GetOpaqueId()
	if id == "" {
		return nil, notAvailable("a share reference by key")
	}
	switch f := req.GetField().GetField().(type) {
	case *collaboration.UpdateShareRequest_UpdateField_Permissions:
		p := f.Permissions.GetPermissions()
		form := url.Values{"permissions": {strconv.Itoa(int(permissions.RoleFromResourcePermissions(p).OCSPermissions()))}}
		if _, err := s.b.c.UpdateShare(ctx, id, form); err != nil {
			return nil, conv(err)
		}
	case *collaboration.UpdateShareRequest_UpdateField_Expiration:
		if f.Expiration == nil {
			return nil, notAvailable("removing the expiration of a share")
		}
		o, err := s.b.c.GetShare(ctx, id)
		if err != nil {
			return nil, conv(err)
		}
		item, err := s.b.itemOf(ctx, o)
		if err != nil {
			return nil, err
		}
		t := time.Unix(int64(f.Expiration.Seconds), 0)
		if _, err := s.b.c.UpdatePermission(ctx, item, id, "", &t); err != nil {
			return nil, conv(err)
		}
	default:
		return nil, notAvailable("this share update")
	}
	return s.Get(ctx, id)
}

func (s shares) Remove(ctx context.Context, id string) error {
	return conv(s.b.c.RemoveShare(ctx, id))
}

func (s shares) Received(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	items, err := s.b.c.SharedWithMe(ctx)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*collaboration.ReceivedShare, 0, len(items))
	for _, it := range items {
		if !it.Federated {
			out = append(out, receivedShare(it))
		}
	}
	return out, nil
}

// receivedShare converts an item of sharedWithMe. The mount point has no
// source, and of the share the ctime, mtime and owner neither.
func receivedShare(it client.DriveItem) *collaboration.ReceivedShare {
	_, id, _ := strings.Cut(it.ShareID, "!")
	sh := &collaboration.Share{
		Id:          &collaboration.ShareId{OpaqueId: id},
		ResourceId:  parseID(it.ID),
		Permissions: &collaboration.SharePermissions{Permissions: rolePermissions(it.Role)},
	}
	for _, p := range it.Permissions {
		if p.GrantedTo != nil {
			sh.Grantee = grantee(p.GrantedTo.Type, p.GrantedTo.ID)
			break
		}
	}
	if it.SharedBy != nil {
		sh.Creator = userID(it.SharedBy.ID)
	}
	rs := &collaboration.ReceivedShare{Share: sh, Hidden: it.Hidden, State: collaboration.ShareState_SHARE_STATE_PENDING}
	switch {
	case it.Accepted:
		rs.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
	case it.Hidden:
		rs.State = collaboration.ShareState_SHARE_STATE_REJECTED
	}
	return rs
}

func (s shares) UpdateReceived(ctx context.Context, id string, state collaboration.ShareState) (*collaboration.ReceivedShare, error) {
	if state != collaboration.ShareState_SHARE_STATE_ACCEPTED && state != collaboration.ShareState_SHARE_STATE_REJECTED {
		return nil, invalid("a received share can only be accepted or rejected over HTTPS")
	}
	if err := s.b.c.SetReceivedShareState(ctx, jailID(id), state == collaboration.ShareState_SHARE_STATE_ACCEPTED); err != nil {
		return nil, conv(err)
	}
	all, err := s.Received(ctx)
	if err != nil {
		return nil, err
	}
	for _, rs := range all {
		if rs.Share.Id.OpaqueId == id {
			return rs, nil
		}
	}
	return nil, notFound("received share %s not found", id)
}

func (s shares) HideReceived(ctx context.Context, id string) error {
	return conv(s.b.c.HideReceivedShare(ctx, jailID(id)))
}

type links struct{ b *Backend }

func (l links) List(ctx context.Context, resource *provider.ResourceId) ([]*link.PublicShare, error) {
	var out []*link.PublicShare
	add := func(rid *provider.ResourceId, perms []client.Permission) {
		for _, p := range perms {
			if p.Link != nil {
				out = append(out, graphLink(p, rid))
			}
		}
	}
	if resource != nil {
		perms, err := l.b.c.ListPermissions(ctx, resourceIDString(resource))
		if err != nil {
			return nil, conv(err)
		}
		add(resource, perms)
		return out, nil
	}
	items, err := l.b.sharedByMe(ctx)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		add(parseID(it.ID), it.Permissions)
	}
	return out, nil
}

// graphLink converts a link permission of Graph. The token is the last part
// of the web URL. Graph gives no mtime, creator or owner, and the
// permissions are those of the link type.
func graphLink(p client.Permission, rid *provider.ResourceId) *link.PublicShare {
	s := &link.PublicShare{
		Id:                &link.PublicShareId{OpaqueId: p.ID},
		ResourceId:        rid,
		Token:             linkToken(p.Link.URL),
		Permissions:       &link.PublicSharePermissions{Permissions: linkPermissions(p.Link.Type)},
		PasswordProtected: p.Link.HasPassword,
		DisplayName:       p.Link.Name,
	}
	if p.ExpiresAt != nil {
		s.Expiration = timestamp(*p.ExpiresAt)
	}
	if p.Created != nil {
		s.Ctime = timestamp(*p.Created)
	}
	return s
}

func linkToken(webURL string) string {
	u, err := url.Parse(webURL)
	if err != nil {
		return ""
	}
	_, tok, ok := strings.Cut(u.Path, "/s/")
	if !ok {
		return ""
	}
	tok, _, _ = strings.Cut(tok, "/")
	return tok
}

func linkPermissions(linkType string) *provider.ResourcePermissions {
	switch linkType {
	case "edit":
		return permissions.NewEditorRole().CS3ResourcePermissions()
	case "createOnly", "upload":
		return permissions.NewUploaderRole().CS3ResourcePermissions()
	}
	return permissions.NewViewerRole().CS3ResourcePermissions()
}

func linkType(p *provider.ResourcePermissions) (string, error) {
	switch permissions.RoleFromResourcePermissions(p).Name {
	case permissions.RoleViewer:
		return "view", nil
	case permissions.RoleEditor, permissions.RoleFileEditor:
		return "edit", nil
	case permissions.RoleUploader:
		return "createOnly", nil
	}
	return "", invalid("these permissions match no link type: use viewer, uploader or editor")
}

// ocsLink converts an OCS link, which has the token, the creator, the owner
// and the ctime.
func ocsLink(o *client.OCSShare) *link.PublicShare {
	return &link.PublicShare{
		Id:                &link.PublicShareId{OpaqueId: o.ID},
		Token:             o.Token,
		ResourceId:        parseID(o.ItemSource),
		Permissions:       &link.PublicSharePermissions{Permissions: permissions.OcsPermissions(o.Permissions).AsCS3Permissions()},
		Owner:             userID(o.UIDFileOwner),
		Creator:           userID(o.UIDOwner),
		Ctime:             seconds(o.STime),
		Expiration:        ocsTime(o.Expiration),
		DisplayName:       o.Name,
		Description:       o.Description,
		PasswordProtected: o.ShareWith != "",
	}
}

// linkID returns the id of the link of ref. A token is looked up in the
// links of the caller, because OCS addresses a link by id only.
func (l links) linkID(ctx context.Context, ref *link.PublicShareReference) (string, error) {
	if id := ref.GetId().GetOpaqueId(); id != "" {
		return id, nil
	}
	tok := ref.GetToken()
	all, err := l.List(ctx, nil)
	if err != nil {
		return "", err
	}
	for _, s := range all {
		if tok != "" && s.Token == tok {
			return s.Id.OpaqueId, nil
		}
	}
	return "", notFound("link %s not found", tok)
}

func (l links) Get(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error) {
	id, err := l.linkID(ctx, ref)
	if err != nil {
		return nil, err
	}
	o, err := l.b.c.GetShare(ctx, id)
	if err != nil {
		return nil, conv(err)
	}
	if o.ShareType != client.OCSSharePublic {
		return nil, notFound("link %s not found", id)
	}
	return ocsLink(o), nil
}

func (l links) Create(ctx context.Context, req *link.CreatePublicShareRequest) (*link.PublicShare, error) {
	info := req.GetResourceInfo()
	if info.GetId() == nil {
		return nil, invalid("the resource of the link has no id")
	}
	lt, err := linkType(req.GetGrant().GetPermissions().GetPermissions())
	if err != nil {
		return nil, err
	}
	opts := client.LinkOptions{Type: lt, Password: req.GetGrant().GetPassword()}
	if e := req.GetGrant().GetExpiration(); e != nil {
		t := time.Unix(int64(e.Seconds), 0)
		opts.Expiry = &t
	}
	p, err := l.b.c.CreateLink(ctx, resourceIDString(info.Id), opts)
	if err != nil {
		return nil, conv(err)
	}
	s := graphLink(*p, info.Id)
	if req.Description != "" {
		// Graph has no description: OCS sets it.
		o, err := l.b.c.UpdateShare(ctx, p.ID, url.Values{"description": {req.Description}})
		if err != nil {
			return nil, conv(err)
		}
		s.Description = o.Description
	}
	return s, nil
}

// Update uses OCS, which changes a link by its id alone.
func (l links) Update(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.PublicShare, error) {
	id, err := l.linkID(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	u := req.GetUpdate()
	form := url.Values{}
	switch u.GetType() {
	case link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS:
		p := u.GetGrant().GetPermissions().GetPermissions()
		form.Set("permissions", strconv.Itoa(int(permissions.RoleFromResourcePermissions(p).OCSPermissions())))
	case link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION:
		form.Set("expireDate", "")
		if e := u.GetGrant().GetExpiration(); e != nil {
			form.Set("expireDate", time.Unix(int64(e.Seconds), 0).UTC().Format("2006-01-02T15:04:05Z0700"))
		}
	case link.UpdatePublicShareRequest_Update_TYPE_PASSWORD:
		form.Set("password", u.GetGrant().GetPassword())
	case link.UpdatePublicShareRequest_Update_TYPE_DESCRIPTION:
		form.Set("description", u.GetDescription())
	case link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME:
		form.Set("name", u.GetDisplayName())
	default:
		return nil, notAvailable("this link update")
	}
	if _, err := l.b.c.UpdateShare(ctx, id, form); err != nil {
		return nil, conv(err)
	}
	return l.Get(ctx, &link.PublicShareReference{Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}}})
}

func (l links) Remove(ctx context.Context, ref *link.PublicShareReference) error {
	id, err := l.linkID(ctx, ref)
	if err != nil {
		return err
	}
	return conv(l.b.c.RemoveShare(ctx, id))
}
