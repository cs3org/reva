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

package ocmshareprovider

import (
	"context"
	"slices"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"google.golang.org/protobuf/proto"
)

// notifier sends one notification to a peer after a committed change. Its
// result never undoes the change. Successful admission owns the state proof.
type notifier interface {
	NewNotification(ctx context.Context, domain string, n *ocmd.OutgoingNotification) ocmd.SendResult
}

// emit reports whether the notifier accepted the handoff.
func (s *service) emit(ctx context.Context, domain string, b ocmd.ShareStateNotification, stillCurrent func(context.Context) bool) bool {
	log := appctx.GetLogger(ctx)
	if s.notifier == nil {
		return false
	}
	// The stored peer host is validated as it is; nothing is stripped from it.
	peer, err := providerdomain.CanonicalOCMHost(domain)
	if err != nil {
		log.Warn().Str("type", b.Type).Msg("not sending OCM notification: invalid peer host")
		return false
	}
	n, err := b.Build()
	if err != nil {
		log.Warn().Str("type", b.Type).Str("error", err.Error()).Msg("not sending OCM notification")
		return false
	}
	if stillCurrent != nil {
		n.StillCurrent = func(ctx context.Context) bool {
			ctx, cancel := context.WithTimeout(ctx, time.Duration(s.conf.ClientTimeout)*time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			return ctx.Err() == nil && stillCurrent(ctx) && ctx.Err() == nil && time.Now().Before(deadline)
		}
	}
	return s.notifier.NewNotification(ctx, peer, n) == (ocmd.SendResult{})
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

// projectWebDAVPermissions returns nil when the snapshot cannot be sent.
func projectWebDAVPermissions(snapshot *provider.ResourcePermissions) []string {
	if snapshot == nil {
		return nil
	}
	names := share.WebDAVPermissionNames(snapshot, snapshot.GetAddGrant())
	decoded, err := share.ParseWebDAVPermissions(names)
	if err != nil || !share.SameWebDAVOperations(snapshot, decoded.GetPermissions()) ||
		snapshot.GetAddGrant() != decoded.GetReshare() {
		return nil
	}
	if len(names) == 0 && !proto.Equal(snapshot, &provider.ResourcePermissions{}) {
		return nil
	}
	// The canonical native roles use MOVE and a single delete bit. Wire
	// write represents both deletes and does not carry native MOVE.
	if snapshot.GetMove() || snapshot.GetDeleteContainer() != decoded.GetPermissions().GetDeleteContainer() {
		if !proto.Equal(snapshot, permissions.NewEditorRole().CS3ResourcePermissions()) &&
			!proto.Equal(snapshot, permissions.NewManagerRole().CS3ResourcePermissions()) {
			return nil
		}
	}
	return names
}

func changedWebDAVPermissionNames(sh *ocm.Share, fields []*ocm.UpdateOCMShareRequest_UpdateField) []string {
	var requested []string
	for _, field := range slices.Backward(fields) {
		if options := field.GetAccessMethods().GetWebdavOptions(); options != nil {
			requested = projectWebDAVPermissions(options.GetPermissions())
			if requested == nil {
				return nil
			}
			break
		}
	}
	for _, method := range sh.GetAccessMethods() {
		if snapshot := method.GetWebdavOptions().GetPermissions(); snapshot != nil {
			committed := projectWebDAVPermissions(snapshot)
			// Legacy storage can broaden a granular update to a full role.
			if requested != nil && !slices.Equal(requested, committed) {
				return nil
			}
			return committed
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
	if rt == "" {
		// The share was stored before resource types were kept. The local
		// change still applies, but the peer is not told.
		appctx.GetLogger(ctx).Warn().Str("type", ocmd.NotificationShareUnshared).Msg("not sending OCM notification: share has no stored resource type")
		return
	}
	s.emit(ctx, sh.GetGrantee().GetUserId().GetIdp(), ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareUnshared,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: rt,
		ShareType:    "user",
		ProviderID:   sh.GetId().GetOpaqueId(),
	}, nil)
}

// notifyPermissionChange sends SHARE_CHANGE_PERMISSION when a stored
// permission value changed.
func (s *service) notifyPermissionChange(ctx context.Context, update *share.ShareUpdate, fields []*ocm.UpdateOCMShareRequest_UpdateField) {
	if !update.PermissionsChanged() || update.After == nil {
		return
	}
	sh := update.After
	rt := resourceTypeName(update.ResourceType)
	if rt == "" {
		appctx.GetLogger(ctx).Warn().Str("type", ocmd.NotificationShareChangePermission).Msg("not sending OCM notification: share has no stored resource type")
		return
	}
	domain := sh.GetGrantee().GetUserId().GetIdp()
	var perms []string
	var stillCurrent func(context.Context) bool
	if update.WebDAVPermissionsChanged {
		perms = changedWebDAVPermissionNames(sh, fields)
	}
	if perms != nil {
		ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.GetId()}}
		stillCurrent = func(ctx context.Context) bool {
			current, err := s.repo.GetShare(ctx, appctx.ContextMustGetUser(ctx), ref)
			if err != nil || current.GetGrantee().GetUserId().GetIdp() != domain {
				return false
			}
			names := changedWebDAVPermissionNames(current, nil)
			return names != nil && slices.Equal(names, perms)
		}
	}
	s.emit(ctx, domain, ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareChangePermission,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: rt,
		ShareType:    "user",
		ProviderID:   sh.GetId().GetOpaqueId(),
		Permissions:  perms,
	}, stillCurrent)
}

// notifyReceivedState sends SHARE_ACCEPTED or SHARE_DECLINED to the original
// sender of the share, never to the owner fallback.
func (s *service) notifyReceivedState(ctx context.Context, update *share.ReceivedShareUpdate, explicitDecline bool) {
	var handedOff bool
	var proof *share.StateNotice
	if update != nil && update.ClaimStateNotice != nil {
		proof = update.ClaimStateNotice()
		if proof == nil {
			return
		}
		defer func() {
			if !handedOff {
				proof.Release()
			}
		}()
	}
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
	var stillCurrent func(context.Context) bool
	if proof == nil && s.conf.Driver == "sql" {
		ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.GetId()}}
		stillCurrent = func(ctx context.Context) bool {
			current, err := s.repo.GetReceivedShare(ctx, appctx.ContextMustGetUser(ctx), ref)
			return err == nil && current != nil &&
				proto.Equal(current.GetId(), rs.GetId()) &&
				proto.Equal(current.GetGrantee(), rs.GetGrantee()) &&
				proto.Equal(current.GetCreator(), rs.GetCreator()) &&
				current.GetRemoteShareId() == rs.GetRemoteShareId() &&
				current.GetState() == rs.GetState()
		}
	}
	handedOff = s.emit(ctx, rs.GetCreator().GetIdp(), ocmd.ShareStateNotification{
		Type:         typ,
		SenderDomain: s.conf.ProviderDomain,
		ResourceType: resourceTypeName(rs.GetSharedResourceType()),
		ShareType:    "user",
		ProviderID:   rs.GetRemoteShareId(),
		StateNotice:  proof,
	}, stillCurrent)
}
