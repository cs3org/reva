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

package ocdav

import (
	"context"
	"encoding/json"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/notifications"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/rs/zerolog"
)

func (s *svc) sendUploadNotification(ctx context.Context, client gateway.GatewayAPIClient, info *provider.ResourceInfo, log zerolog.Logger) {
	if info == nil {
		return
	}

	publicShare := publicShareFromResourceInfo(info)
	if publicShare == nil {
		return
	}

	var recipients []*userpb.User
	recipients = append(recipients, s.notificationRecipientsFromPublicShare(ctx, client, publicShare, log)...)
	recipients = uniqueRecipients(recipients)
	if len(recipients) == 0 {
		return
	}

	templateData := map[string]any{
		"resource_id":   spaces.EncodeToStringifiedResourceID(info.GetId()),
		"resource_name": info.GetName(),
		"resource_path": info.GetPath(),
		"resource_type": info.GetType().String(),
		"mime_type":     info.GetMimeType(),
		"size":          info.GetSize(),
		"share_id":      publicShareIDString(publicShare),
		"share_token":   publicShare.GetToken(),
	}

	// PublishEvent is restricted to reva daemons. Re-sign this request's token with
	// the machine scope, keeping the acting user (for public link uploads the
	// public user publiclink:<id>) unchanged.
	publishCtx, err := scope.ContextWithMachineScope(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to elevate context for upload notification")
		return
	}

	// Name the shared folder the upload landed in. The machine scope lets us
	// stat the link's target directly, so we read the folder's real name and
	// path rather than the public-link path or the uploaded file.
	if res, err := client.Stat(publishCtx, &provider.StatRequest{
		Ref: &provider.Reference{ResourceId: publicShare.GetResourceId()},
	}); err == nil && res.GetStatus().GetCode() == rpc.Code_CODE_OK {
		templateData["share_path"] = res.GetInfo().GetPath()
		templateData["share_name"] = res.GetInfo().GetName()
	} else {
		log.Error().Err(err).Msg("failed to resolve shared folder for upload notification")
	}

	event := notifications.EncodeEvent(model.EventUpload, recipients, templateData)
	res, err := client.PublishEvent(publishCtx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		log.Error().Err(err).Msg("failed to send upload notification event")
		return
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		log.Error().Str("code", code.String()).Str("message", res.GetStatus().GetMessage()).Msg("gateway rejected upload notification event")
	}
}

func (s *svc) notificationRecipientsFromPublicShare(ctx context.Context, client gateway.GatewayAPIClient, publicShare *link.PublicShare, log zerolog.Logger) []*userpb.User {
	var recipients []*userpb.User
	for _, address := range splitRecipients(publicShare.GetNotifyUploadsExtraRecipients()) {
		// The extra recipients of a link are addresses typed by its creator,
		// they need not belong to an account here.
		recipients = append(recipients, &userpb.User{Mail: address})
	}
	if publicShare.GetNotifyUploads() {
		if owner := s.publicShareOwner(ctx, client, publicShare, log); owner != nil {
			recipients = append(recipients, owner)
		}
	}
	return recipients
}

func (s *svc) publicShareOwner(ctx context.Context, client gateway.GatewayAPIClient, publicShare *link.PublicShare, log zerolog.Logger) *userpb.User {
	owner := publicShare.GetOwner()
	if owner == nil {
		owner = publicShare.GetCreator()
	}
	if owner == nil || client == nil {
		return nil
	}

	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 owner,
		SkipFetchingUserGroups: true,
	})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetUser() == nil {
		log.Debug().Err(err).Msg("failed to resolve public share owner for upload notification")
		return nil
	}
	return res.GetUser()
}

func publicShareIDString(publicShare *link.PublicShare) string {
	if publicShare == nil || publicShare.GetId() == nil {
		return ""
	}
	return publicShare.GetId().GetOpaqueId()
}

func publicShareFromResourceInfo(info *provider.ResourceInfo) *link.PublicShare {
	entry := info.GetOpaque().GetMap()["link-share"]
	if entry == nil || entry.Decoder != "json" {
		return nil
	}

	var publicShare link.PublicShare
	if err := json.Unmarshal(entry.Value, &publicShare); err != nil {
		return nil
	}
	return &publicShare
}

func splitRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})

	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			recipients = append(recipients, part)
		}
	}
	return recipients
}

// uniqueRecipients drops the recipients without an address and collapses the
// ones sharing one, as the link owner may also be listed as an extra recipient.
func uniqueRecipients(users []*userpb.User) []*userpb.User {
	out := make([]*userpb.User, 0, len(users))
	seen := make(map[string]struct{}, len(users))
	for _, user := range users {
		mail := strings.TrimSpace(user.GetMail())
		if mail == "" {
			continue
		}
		key := strings.ToLower(mail)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, user)
	}
	return out
}
