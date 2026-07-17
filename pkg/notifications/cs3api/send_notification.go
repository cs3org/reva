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

// Package cs3api contains local stand-ins for notification API messages that
// will move to go-cs3apis once the upstream SendNotification RPC is available.
package cs3api

import "github.com/cs3org/reva/v3/pkg/notifications/model"

// SendNotificationRequest is the local shape of the future
// cs3.gateway.v1beta1.SendNotificationRequest.
//
// Sender and submitting user are intentionally absent. The gateway must derive
// them from the authenticated context before publishing the event.
type SendNotificationRequest struct {
	EventType    string         `json:"event_type"`
	Recipients   []string       `json:"recipients"`
	TemplateData map[string]any `json:"template_data,omitempty"`
}

// SendNotificationResponse is the local shape of the future
// cs3.gateway.v1beta1.SendNotificationResponse.
type SendNotificationResponse struct {
	NotificationID string `json:"notification_id"`
}

// ToModel converts the local API request into the internal sender request once
// the gateway has resolved the authenticated submitter and sender.
func (r SendNotificationRequest) ToModel(submittingUser, sender string) model.SendRequest {
	return model.SendRequest{
		EventType:      r.EventType,
		SubmittingUser: submittingUser,
		Sender:         sender,
		Recipients:     append([]string(nil), r.Recipients...),
		TemplateData:   cloneTemplateData(r.TemplateData),
	}
}

func cloneTemplateData(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
