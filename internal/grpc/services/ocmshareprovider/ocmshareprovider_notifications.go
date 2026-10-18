// Copyright 2018-2024 CERN
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

package ocmshareprovider

import (
	"context"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
)

// notifier sends one notification to a peer. Emission happens only after a
// committed effective change; its result is logged and never undoes it.
type notifier interface {
	NewNotification(ctx context.Context, domain string, n *ocmd.OutgoingNotification) ocmd.SendResult
}

// asyncNotifier sends in the background, so a slow or unreachable peer does
// not delay the RPC. Each notification is still attempted exactly once.
type asyncNotifier struct {
	c *ocmd.OCMClient
}

func (a asyncNotifier) NewNotification(ctx context.Context, domain string, n *ocmd.OutgoingNotification) ocmd.SendResult {
	detached := context.WithoutCancel(ctx)
	go a.c.NewNotification(detached, domain, n)
	return ocmd.SendResult{}
}

// emit sends a built share-state notification to domain, if any.
func (s *service) emit(ctx context.Context, domain string, b ocmd.ShareStateNotification) {
	log := appctx.GetLogger(ctx)
	if s.notifier == nil {
		return
	}
	if domain == "" {
		log.Warn().Str("type", b.Type).Msg("not sending OCM notification: unknown peer domain")
		return
	}
	n, err := b.Build()
	if err != nil {
		log.Warn().Str("type", b.Type).Str("error", err.Error()).Msg("not sending OCM notification")
		return
	}
	s.notifier.NewNotification(ctx, ocmd.TrimOCMScheme(domain), n)
}

// outgoingResourceType resolves the resource type of an outgoing share. It is
// read-only payload preparation after the committed change; it never decides
// whether a notification is due.
func outgoingResourceType(ctx context.Context, id *providerpb.ResourceId) string {
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return ""
	}
	res, err := gw.Stat(ctx, &providerpb.StatRequest{Ref: &providerpb.Reference{ResourceId: id}})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetInfo() == nil {
		return ""
	}
	switch rt := getResourceType(res.GetInfo()); rt {
	case "file", "folder":
		return rt
	}
	return ""
}

func receivedResourceType(t ocm.SharedResourceType) string {
	switch t {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE:
		return "file"
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER:
		return "folder"
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
		return "ro-crate"
	}
	return ""
}

// receivedSecret is the protocol secret this server received for the share.
func receivedSecret(rs *ocm.ReceivedShare) string {
	for _, p := range rs.GetProtocols() {
		if w, ok := p.Term.(*ocm.Protocol_WebdavOptions); ok && w.WebdavOptions.GetSharedSecret() != "" {
			return w.WebdavOptions.GetSharedSecret()
		}
	}
	return ""
}

// webdavPermissionNames maps the stored WebDAV permissions to the OCM
// permission names used on the wire.
func webdavPermissionNames(sh *ocm.Share) []string {
	for _, m := range sh.GetAccessMethods() {
		if w, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
			var perms []string
			if w.WebdavOptions.GetPermissions().GetInitiateFileDownload() {
				perms = append(perms, "read")
			}
			if w.WebdavOptions.GetPermissions().GetInitiateFileUpload() {
				perms = append(perms, "write")
			}
			return perms
		}
	}
	return nil
}

// notifyUnshared sends SHARE_UNSHARED to the grantee's server after a
// committed deletion. The identifier is this server's share ID and the
// optional secret is the outgoing token.
func (s *service) notifyUnshared(ctx context.Context, removal *share.ShareRemoval) {
	if removal == nil || removal.Share == nil {
		return
	}
	sh := removal.Share
	rt := outgoingResourceType(ctx, sh.GetResourceId())
	if rt == "" {
		appctx.GetLogger(ctx).Warn().Str("share_id", sh.GetId().GetOpaqueId()).Msg("not sending SHARE_UNSHARED: resource type unknown")
		return
	}
	s.emit(ctx, sh.GetGrantee().GetUserId().GetIdp(), ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareUnshared,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: rt,
		ShareType:    "user",
		ProviderID:   sh.GetId().GetOpaqueId(),
		SharedSecret: sh.GetToken(),
	})
}

// notifyPermissionChange sends SHARE_CHANGE_PERMISSION after a committed
// change of a stored permission value.
func (s *service) notifyPermissionChange(ctx context.Context, update *share.ShareUpdate) {
	if !update.PermissionsChanged() || update.After == nil {
		return
	}
	sh := update.After
	rt := outgoingResourceType(ctx, sh.GetResourceId())
	if rt == "" {
		appctx.GetLogger(ctx).Warn().Str("share_id", sh.GetId().GetOpaqueId()).Msg("not sending SHARE_CHANGE_PERMISSION: resource type unknown")
		return
	}
	var perms []string
	if update.WebDAVPermissionsChanged {
		perms = webdavPermissionNames(sh)
	}
	s.emit(ctx, sh.GetGrantee().GetUserId().GetIdp(), ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareChangePermission,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: rt,
		ShareType:    "user",
		ProviderID:   sh.GetId().GetOpaqueId(),
		Permissions:  perms,
	})
}

// notifyReceivedState sends SHARE_ACCEPTED or SHARE_DECLINED to the
// provider after a committed state transition. The identifier is the one
// the provider assigned, and the provider is the original sender of the
// share, never the owner fallback. explicitDecline is false for a transfer
// failure, which is not the recipient declining.
func (s *service) notifyReceivedState(ctx context.Context, update *share.ReceivedShareUpdate, explicitDecline bool) {
	if !update.StateChanged() {
		return
	}
	rs := update.After
	var typ string
	switch rs.GetState() {
	case ocm.ShareState_SHARE_STATE_ACCEPTED:
		typ = ocmd.NotificationShareAccepted
	case ocm.ShareState_SHARE_STATE_REJECTED:
		if !explicitDecline {
			return
		}
		typ = ocmd.NotificationShareDeclined
	default:
		return
	}
	s.emit(ctx, rs.GetCreator().GetIdp(), ocmd.ShareStateNotification{
		Type:         typ,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: receivedResourceType(rs.GetSharedResourceType()),
		ShareType:    "user",
		ProviderID:   rs.GetRemoteShareId(),
		SharedSecret: receivedSecret(rs),
	})
}

// shareCreated is the success seam of CreateOCMShare. It deliberately sends
// nothing: the share creation notification itself is the POST /shares call
// made before the share is stored.
func (s *service) shareCreated(context.Context, *ocm.Share) {}
