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
	"cmp"
	"context"
	"path"
	"strings"
	"time"

	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/backend/http/internal/client"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

type spaceArea struct{ b *Backend }

func (s spaceArea) List(ctx context.Context) ([]*provider.StorageSpace, error) {
	list, err := s.b.c.Spaces(ctx)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*provider.StorageSpace, len(list))
	for i, sp := range list {
		out[i] = storageSpace(sp)
	}
	return out, nil
}

// storageSpace converts a Graph drive. The used bytes go in the size of the
// root. Graph gives the owner only by id, and no root resource id.
func storageSpace(sp client.Space) *provider.StorageSpace {
	s := &provider.StorageSpace{
		Id:        &provider.StorageSpaceId{OpaqueId: sp.ID},
		Name:      sp.Name,
		SpaceType: sp.Type,
		RootInfo:  &provider.ResourceInfo{Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER, Path: sp.Path, Size: uint64(max(sp.QuotaUsed, 0))},
	}
	if sp.Owner != "" {
		s.Owner = &userpb.User{Id: userID(sp.Owner)}
	}
	if sp.QuotaTotal > 0 {
		s.Quota = &provider.Quota{QuotaMaxBytes: uint64(sp.QuotaTotal)}
	}
	return s
}

func (s spaceArea) Update(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.StorageSpace, error) {
	return nil, notAvailable("space update")
}

func (s spaceArea) Quota(ctx context.Context, ref *provider.Reference) (*backend.Quota, error) {
	list, err := s.b.c.Spaces(ctx)
	if err != nil {
		return nil, conv(err)
	}
	all := make([]*provider.StorageSpace, len(list))
	for i, sp := range list {
		all[i] = storageSpace(sp)
	}
	key := ref.GetPath()
	if key == "" {
		key = ref.GetResourceId().GetSpaceId()
	}
	found, err := backend.FindSpace(all, key)
	if err != nil {
		return nil, err
	}
	for _, sp := range list {
		if sp.ID == found.Id.OpaqueId {
			return &backend.Quota{Total: uint64(max(sp.QuotaTotal, 0)), Used: uint64(max(sp.QuotaUsed, 0)), Remaining: uint64(max(sp.QuotaRemaining, 0))}, nil
		}
	}
	return nil, notFound("space %s not found", key)
}

type trash struct{ b *Backend }

func (t trash) List(ctx context.Context, ref *provider.Reference, from, to time.Time) ([]*provider.RecycleItem, error) {
	base := ref.GetPath()
	l, err := t.b.c.ListTrash(ctx, base, client.TrashWindow{From: from, To: to})
	if err != nil {
		return nil, conv(err)
	}
	root := base
	out := make([]*provider.RecycleItem, 0, len(l.Items))
	for _, it := range l.Items {
		orig := it.OriginalPath
		if orig != "" && !path.IsAbs(orig) {
			// The server gives the location relative to the root of the space.
			if root == "" {
				if root, err = t.b.c.ResolveSpace(ctx, "home"); err != nil {
					return nil, conv(err)
				}
			}
			orig = path.Join(root, orig)
		}
		ri := &provider.RecycleItem{
			Type:         provider.ResourceType_RESOURCE_TYPE_FILE,
			Key:          it.Key,
			Ref:          &provider.Reference{Path: orig},
			Size:         uint64(max(it.Size, 0)),
			DeletionTime: timestamp(it.DeletedAt),
		}
		if it.IsDir {
			ri.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
		}
		out = append(out, ri)
	}
	return out, nil
}

func (t trash) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	return conv(t.b.c.RestoreTrash(ctx, key, ref.GetPath()))
}

func (t trash) Purge(ctx context.Context, ref *provider.Reference, key string) error {
	return conv(t.b.c.PurgeTrash(ctx, key, ref.GetPath()))
}

type versions struct{ b *Backend }

// fileID returns the resource id of ref as Graph and the meta URLs want it.
func (b *Backend) fileID(ctx context.Context, ref *provider.Reference) (string, error) {
	if id := ref.GetResourceId(); id != nil && ref.GetPath() == "" {
		return spaces.EncodeToStringifiedResourceID(id), nil
	}
	info, err := files{b}.Stat(ctx, ref)
	if err != nil {
		return "", err
	}
	if info.Id == nil {
		return "", invalid("the server gives no id for %s", info.Path)
	}
	return spaces.EncodeToStringifiedResourceID(info.Id), nil
}

func (v versions) List(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	id, err := v.b.fileID(ctx, ref)
	if err != nil {
		return nil, err
	}
	list, err := v.b.c.ListVersions(ctx, id)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*provider.FileVersion, len(list))
	for i, fv := range list {
		out[i] = &provider.FileVersion{Key: fv.Key, Size: uint64(max(fv.Size, 0)), Mtime: uint64(max(fv.Modified.Unix(), 0)), Etag: fv.ETag}
	}
	return out, nil
}

func (v versions) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	id, err := v.b.fileID(ctx, ref)
	if err != nil {
		return err
	}
	return conv(v.b.c.RestoreVersion(ctx, id, key))
}

// locks: WebDAV has LOCK and UNLOCK, but they do not carry the CS3 lock.
type locks struct{}

func (locks) Get(context.Context, *provider.Reference) (*provider.Lock, error) {
	return nil, notAvailable("locks")
}
func (locks) Set(context.Context, *provider.Reference, *provider.Lock) error {
	return notAvailable("locks")
}
func (locks) Refresh(context.Context, *provider.Reference, *provider.Lock) error {
	return notAvailable("locks")
}
func (locks) Unlock(context.Context, *provider.Reference, *provider.Lock) error {
	return notAvailable("locks")
}

type ocmArea struct{ b *Backend }

func invite(i client.Invite) *invitepb.InviteToken {
	t := &invitepb.InviteToken{Token: i.Token, Description: i.Description}
	if i.Expiration != nil {
		t.Expiration = timestamp(*i.Expiration)
	}
	return t
}

func (o ocmArea) CreateInvite(ctx context.Context, description string) (*invitepb.InviteToken, error) {
	i, err := o.b.c.GenerateInvite(ctx, description)
	if err != nil {
		return nil, conv(err)
	}
	return invite(*i), nil
}

func (o ocmArea) Invites(ctx context.Context) ([]*invitepb.InviteToken, error) {
	list, err := o.b.c.ListInvites(ctx)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*invitepb.InviteToken, len(list))
	for i := range list {
		out[i] = invite(list[i])
	}
	return out, nil
}

func (o ocmArea) AcceptInvite(ctx context.Context, token, domain string) error {
	return conv(o.b.c.AcceptInvite(ctx, token, domain))
}

func (o ocmArea) Contacts(ctx context.Context, filter string) ([]*userpb.User, error) {
	list, err := o.b.c.AcceptedUsers(ctx)
	if err != nil {
		return nil, conv(err)
	}
	filter = strings.ToLower(filter)
	var out []*userpb.User
	for _, u := range list {
		if filter != "" && !strings.Contains(strings.ToLower(u.DisplayName+" "+u.Address()+" "+u.Mail), filter) {
			continue
		}
		out = append(out, &userpb.User{
			Id:          &userpb.UserId{Idp: u.IDP, OpaqueId: u.UserID, Type: userpb.UserType_USER_TYPE_FEDERATED},
			DisplayName: u.DisplayName,
			Mail:        u.Mail,
		})
	}
	return out, nil
}

func (o ocmArea) RemoveContact(ctx context.Context, id *userpb.UserId) error {
	return conv(o.b.c.RemoveAcceptedUser(ctx, id.GetIdp(), id.GetOpaqueId()))
}

func (o ocmArea) Providers(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	list, err := o.b.c.ListProviders(ctx)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*ocmprovider.ProviderInfo, len(list))
	for i, p := range list {
		out[i] = &ocmprovider.ProviderInfo{Name: p.Name, FullName: p.FullName, Domain: p.Domain, Homepage: p.Homepage}
	}
	return out, nil
}

// CreateShare uses the invite of Graph with a remote recipient. The access
// methods have no Graph form: the share gets the viewer role.
func (o ocmArea) CreateShare(ctx context.Context, ref *provider.Reference, grantee *userpb.UserId, methods []*ocm.AccessMethod) (*ocm.Share, error) {
	if len(methods) > 0 {
		return nil, notAvailable("OCM access methods")
	}
	id, err := o.b.fileID(ctx, ref)
	if err != nil {
		return nil, err
	}
	r := client.Recipient{ID: grantee.GetOpaqueId() + "@" + grantee.GetIdp(), Type: client.RecipientRemote}
	perms, err := o.b.c.Share(ctx, id, []client.Recipient{r}, client.RoleViewer, nil)
	if err != nil {
		return nil, conv(err)
	}
	if len(perms) == 0 {
		return nil, invalid("the server created no share")
	}
	return &ocm.Share{Id: &ocm.ShareId{OpaqueId: perms[0].ID}, ResourceId: parseID(id), Grantee: &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: grantee}}}, nil
}

func (o ocmArea) Shares(ctx context.Context, resource *provider.ResourceId) ([]*ocm.Share, error) {
	return nil, notAvailable("the list of OCM shares")
}

func (o ocmArea) GetShare(ctx context.Context, id string) (*ocm.Share, error) {
	return nil, notAvailable("an OCM share by id")
}

func (o ocmArea) UpdateShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) error {
	return notAvailable("the update of an OCM share")
}

func (o ocmArea) RemoveShare(ctx context.Context, id string) error {
	return notAvailable("the removal of an OCM share")
}

func (o ocmArea) Received(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	items, err := o.b.c.SharedWithMe(ctx)
	if err != nil {
		return nil, conv(err)
	}
	var out []*ocm.ReceivedShare
	for _, it := range items {
		if it.Federated {
			out = append(out, receivedOCM(it))
		}
	}
	return out, nil
}

// receivedOCM converts a federated item of sharedWithMe. Graph gives no
// protocols, no ctime and no remote share id.
func receivedOCM(it client.DriveItem) *ocm.ReceivedShare {
	rs := &ocm.ReceivedShare{Id: &ocm.ShareId{OpaqueId: it.ShareID}, Name: it.Name, State: ocm.ShareState_SHARE_STATE_PENDING}
	if it.SharedBy != nil {
		rs.Creator = &userpb.UserId{OpaqueId: it.SharedBy.ID}
	}
	switch {
	case it.Accepted:
		rs.State = ocm.ShareState_SHARE_STATE_ACCEPTED
	case it.Hidden:
		rs.State = ocm.ShareState_SHARE_STATE_REJECTED
	}
	return rs
}

func (o ocmArea) GetReceived(ctx context.Context, id string) (*ocm.ReceivedShare, error) {
	all, err := o.Received(ctx)
	if err != nil {
		return nil, err
	}
	for _, rs := range all {
		if rs.Id.OpaqueId == id {
			return rs, nil
		}
	}
	return nil, notFound("received OCM share %s not found", id)
}

func (o ocmArea) UpdateReceived(ctx context.Context, id string, state ocm.ShareState) error {
	if state != ocm.ShareState_SHARE_STATE_ACCEPTED && state != ocm.ShareState_SHARE_STATE_REJECTED {
		return invalid("a received share can only be accepted or rejected over HTTPS")
	}
	return conv(o.b.c.SetReceivedShareState(ctx, id, state == ocm.ShareState_SHARE_STATE_ACCEPTED))
}

type tokens struct{ b *Backend }

// List gives the client id of each app token in Password, because over
// HTTPS the client id is the id of the token. The scope has no source.
func (t tokens) List(ctx context.Context) ([]*authapp.AppPassword, error) {
	list, err := t.b.c.ListConnectedClients(ctx)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*authapp.AppPassword, len(list))
	for i, c := range list {
		out[i] = &authapp.AppPassword{Password: c.ID, Label: cmp.Or(c.Name, c.Description), Ctime: rfc3339(c.CreatedAt), Utime: rfc3339(c.LastSeenAt)}
	}
	return out, nil
}

func rfc3339(s string) *types.Timestamp {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return timestamp(t)
}

func (t tokens) Create(ctx context.Context, req *authapp.GenerateAppPasswordRequest) (*authapp.AppPassword, error) {
	return nil, notAvailable("token create")
}

func (t tokens) Revoke(ctx context.Context, id string) error {
	return conv(t.b.c.RevokeConnectedClient(ctx, id))
}

type users struct{ b *Backend }

// Me converts the user of Graph, which has no idp and no user type.
func (u users) Me(ctx context.Context) (*userpb.User, error) {
	me, err := u.b.c.Me(ctx)
	if err != nil {
		return nil, conv(err)
	}
	return &userpb.User{Id: &userpb.UserId{OpaqueId: me.ID}, Username: me.Username, DisplayName: me.DisplayName, Mail: me.Mail}, nil
}

func (u users) ByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	if claim != "username" && claim != "mail" {
		return nil, notAvailable("a user lookup by " + claim)
	}
	found, err := u.Find(ctx, value)
	if err != nil {
		return nil, err
	}
	for _, f := range found {
		if claim == "username" && f.Username == value || claim == "mail" && strings.EqualFold(f.Mail, value) {
			return f, nil
		}
	}
	return nil, notFound("no user with %s %s", claim, value)
}

// Find searches the primary accounts only: the user search of Graph has a
// filter for the other types that the backend does not send.
func (u users) Find(ctx context.Context, query string) ([]*userpb.User, error) {
	list, err := u.b.c.FindUsers(ctx, query)
	if err != nil {
		return nil, conv(err)
	}
	out := make([]*userpb.User, len(list))
	for i, f := range list {
		out[i] = &userpb.User{Id: &userpb.UserId{OpaqueId: f.ID}, Username: f.Username, DisplayName: f.DisplayName, Mail: f.Mail}
	}
	return out, nil
}

func (u users) Groups(ctx context.Context, id *userpb.UserId) ([]string, error) {
	return nil, notAvailable("the groups of a user")
}
