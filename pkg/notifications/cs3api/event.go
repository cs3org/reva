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

// Package cs3api translates between notification events as defined by the CS3
// gateway API and the internal notification model.
package cs3api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

const (
	recipientsKey   = "recipients"
	templateDataKey = "template_data"
	jsonDecoder     = "json"
)

// PublishEvent submits a notification event to the gateway, which resolves the
// sender and the submitting user from the authenticated context and publishes
// the event to the notification backend. It returns the event id assigned by
// the gateway.
func PublishEvent(ctx context.Context, client gateway.GatewayAPIClient, eventType string, recipients []string, templateData map[string]any) (string, error) {
	event, err := NewEvent(eventType, recipients, templateData)
	if err != nil {
		return "", err
	}

	res, err := client.PublishEvent(ctx, &gateway.PublishEventRequest{Event: event})
	if err != nil {
		return "", err
	}
	if code := res.GetStatus().GetCode(); code != rpc.Code_CODE_OK {
		return "", fmt.Errorf("gateway rejected event %s: %s (%s)", eventType, code.String(), res.GetStatus().GetMessage())
	}
	return res.GetEventId(), nil
}

// NewEvent builds a CS3 event carrying a notification. The sender and the
// submitting user are intentionally absent: the gateway derives them from the
// authenticated request context.
func NewEvent(eventType string, recipients []string, templateData map[string]any) (*gateway.Event, error) {
	data := &types.Opaque{Map: map[string]*types.OpaqueEntry{}}

	if err := setJSON(data, recipientsKey, recipients); err != nil {
		return nil, err
	}
	if len(templateData) > 0 {
		if err := setJSON(data, templateDataKey, templateData); err != nil {
			return nil, err
		}
	}

	return &gateway.Event{
		Type: eventType,
		Data: data,
	}, nil
}

// SendRequestFromEvent decodes a CS3 event into an internal send request. The
// submitting user and the sender are the identities the gateway resolved from
// the request context.
func SendRequestFromEvent(event *gateway.Event, submittingUser, sender string) (model.SendRequest, error) {
	if event == nil {
		return model.SendRequest{}, errors.New("event is required")
	}

	var recipients []string
	if err := getJSON(event.GetData(), recipientsKey, &recipients); err != nil {
		return model.SendRequest{}, err
	}

	var templateData map[string]any
	if err := getJSON(event.GetData(), templateDataKey, &templateData); err != nil {
		return model.SendRequest{}, err
	}

	return model.SendRequest{
		EventType:      event.GetType(),
		SubmittingUser: submittingUser,
		Sender:         sender,
		Recipients:     recipients,
		TemplateData:   templateData,
	}, nil
}

// UserIDString renders a user id as the stable identifier used to attribute
// notifications to their submitter.
func UserIDString(id *userpb.UserId) string {
	if id == nil {
		return ""
	}
	return strings.Join([]string{id.GetIdp(), id.GetOpaqueId(), id.GetType().String(), id.GetTenantId()}, ":")
}

func setJSON(opaque *types.Opaque, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to encode notification %s: %w", key, err)
	}

	opaque.Map[key] = &types.OpaqueEntry{
		Decoder: jsonDecoder,
		Value:   encoded,
	}
	return nil
}

func getJSON(opaque *types.Opaque, key string, target any) error {
	entry := opaque.GetMap()[key]
	if entry == nil {
		return nil
	}
	if entry.GetDecoder() != jsonDecoder {
		return fmt.Errorf("notification %s has unsupported decoder %s", key, entry.GetDecoder())
	}
	if err := json.Unmarshal(entry.GetValue(), target); err != nil {
		return fmt.Errorf("failed to decode notification %s: %w", key, err)
	}
	return nil
}
