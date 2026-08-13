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

package notifications

import (
	"encoding/json"
	"fmt"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// A notification event's payload rides in the CS3 Event's Data opaque: the
// event type is a plain field, everything event-specific (recipients, template
// data) is packed here under these keys. EncodeEvent and DecodeEvent are the two
// halves of that contract and must stay in sync.
const (
	recipientsKey   = "recipients"
	templateDataKey = "template_data"
	jsonDecoder     = "json"
)

// EncodeEvent packs a notification into a CS3 gateway event. The sender and the
// submitting user are intentionally absent: the gateway derives them from the
// authenticated request context, and the CS3 API forbids callers from supplying
// them.
func EncodeEvent(eventType string, recipients []*userpb.User, templateData map[string]any) *gateway.Event {
	data := &types.Opaque{Map: map[string]*types.OpaqueEntry{}}

	setJSON(data, recipientsKey, recipients)
	if len(templateData) > 0 {
		setJSON(data, templateDataKey, templateData)
	}

	return &gateway.Event{
		Type: eventType,
		Data: data,
	}
}

// DecodeEvent extracts the recipients and template data a producer packed into
// the event with EncodeEvent. It is the inverse of EncodeEvent; the event type
// is read directly from the event and the identities are resolved by the
// gateway from the request context.
func DecodeEvent(event *gateway.Event) (recipients []*userpb.User, templateData map[string]any, err error) {
	if err = getJSON(event.GetData(), recipientsKey, &recipients); err != nil {
		return nil, nil, err
	}
	if err = getJSON(event.GetData(), templateDataKey, &templateData); err != nil {
		return nil, nil, err
	}
	return recipients, templateData, nil
}

// UserIDString renders a user id as the stable identifier used to attribute
// notifications to their submitter.
func UserIDString(id *userpb.UserId) string {
	if id == nil {
		return ""
	}
	return strings.Join([]string{id.GetIdp(), id.GetOpaqueId(), id.GetType().String(), id.GetTenantId()}, ":")
}

// setJSON encodes value under key. recipients and template data are always
// JSON-safe, so a marshal failure is not reachable; if one ever occurred the
// key is left unset and the gateway rejects the event when it validates the
// decoded request.
func setJSON(opaque *types.Opaque, key string, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}

	opaque.Map[key] = &types.OpaqueEntry{
		Decoder: jsonDecoder,
		Value:   encoded,
	}
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
