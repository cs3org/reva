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
	"github.com/cs3org/reva/v3/pkg/notifications/cs3api"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/rs/zerolog"
)

func (s *svc) sendUploadNotification(ctx context.Context, client gateway.GatewayAPIClient, info *provider.ResourceInfo, log zerolog.Logger) {
	if info == nil {
		return
	}

	var recipients []string
	publicShare := publicShareFromResourceInfo(info)
	if publicShare != nil {
		recipients = append(recipients, s.notificationRecipientsFromPublicShare(ctx, client, publicShare, log)...)
	}
	recipients = uniqueNonEmptyStrings(recipients)
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
	}
	if publicShare != nil {
		templateData["share_id"] = publicShareIDString(publicShare)
		templateData["share_token"] = publicShare.GetToken()
	}

	// Public link uploads carry a publicshares token whose scope does not cover
	// PublishEvent. Authenticate as the share owner via machine auth so the gateway
	// can verify the caller is a reva daemon and use the owner as the rate-limit
	// identity. Fall back to the request context for regular (non-public) uploads.
	publishCtx := ctx
	if publicShare != nil {
		ownerID := publicShare.GetOwner()
		if ownerID == nil {
			ownerID = publicShare.GetCreator()
		}
		if ownerID != nil && s.c.MachineSecret != "" {
			machineCtx, err := cs3api.MachineCtx(ctx, client, ownerID.GetOpaqueId(), s.c.MachineSecret)
			if err != nil {
				log.Debug().Err(err).Msg("upload notification skipped: machine auth failed")
				return
			}
			publishCtx = machineCtx
		}
	}

	if _, err := cs3api.PublishEvent(publishCtx, client, model.EventUpload, recipients, templateData); err != nil {
		log.Error().Err(err).Msg("failed to send upload notification event")
	}
}

func (s *svc) notificationRecipientsFromPublicShare(ctx context.Context, client gateway.GatewayAPIClient, publicShare *link.PublicShare, log zerolog.Logger) []string {
	recipients := splitRecipients(publicShare.GetNotifyUploadsExtraRecipients())
	if publicShare.GetNotifyUploads() {
		ownerMail := s.publicShareOwnerMail(ctx, client, publicShare, log)
		recipients = append(recipients, ownerMail)
	}
	return recipients
}

func (s *svc) publicShareOwnerMail(ctx context.Context, client gateway.GatewayAPIClient, publicShare *link.PublicShare, log zerolog.Logger) string {
	owner := publicShare.GetOwner()
	if owner == nil {
		owner = publicShare.GetCreator()
	}
	if owner == nil || client == nil {
		return ""
	}

	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 owner,
		SkipFetchingUserGroups: true,
	})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetUser() == nil {
		log.Debug().Err(err).Msg("failed to resolve public share owner for upload notification")
		return ""
	}
	return res.GetUser().GetMail()
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

func uniqueNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
