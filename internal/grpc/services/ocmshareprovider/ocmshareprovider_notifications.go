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

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
)

// notifier sends one notification to a peer after a committed change. Its
// result never undoes the change.
type notifier interface {
	NewNotification(ctx context.Context, domain string, n *ocmd.OutgoingNotification) ocmd.SendResult
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

func resourceTypeName(t ocm.SharedResourceType) string {
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

// notifyUnshared sends SHARE_UNSHARED to the grantee's server. The resource
// type comes from the receipt, because the resource may be gone already.
func (s *service) notifyUnshared(ctx context.Context, removal *share.ShareRemoval) {
	if removal == nil || removal.Share == nil {
		return
	}
	sh := removal.Share
	rt := resourceTypeName(removal.ResourceType)
	s.emit(ctx, sh.GetGrantee().GetUserId().GetIdp(), ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareUnshared,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: rt,
		ShareType:    "user",
		ProviderID:   sh.GetId().GetOpaqueId(),
		SharedSecret: sh.GetToken(),
	})
}

// notifyPermissionChange sends SHARE_CHANGE_PERMISSION when a stored
// permission value changed.
func (s *service) notifyPermissionChange(ctx context.Context, update *share.ShareUpdate) {
	if !update.PermissionsChanged() || update.After == nil {
		return
	}
	sh := update.After
	rt := resourceTypeName(update.ResourceType)
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

// notifyReceivedState sends SHARE_ACCEPTED or SHARE_DECLINED to the original
// sender of the share, never to the owner fallback.
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
		ResourceType: resourceTypeName(rs.GetSharedResourceType()),
		ShareType:    "user",
		ProviderID:   rs.GetRemoteShareId(),
		SharedSecret: receivedSecret(rs),
	})
}
