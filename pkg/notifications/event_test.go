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
	"reflect"
	"testing"
)

func TestEventRoundTripKeepsNotificationPayload(t *testing.T) {
	recipients := []string{"bob@example.org", "carol@example.org"}
	templateData := map[string]any{"share_id": "share-1", "resource_name": "beach.png"}

	event := EncodeEvent("share-creation", recipients, templateData)

	if event.GetType() != "share-creation" {
		t.Fatalf("event type = %q, want share-creation", event.GetType())
	}

	gotRecipients, gotTemplateData, err := DecodeEvent(event)
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}
	if !reflect.DeepEqual(gotRecipients, recipients) {
		t.Fatalf("recipients = %v, want %v", gotRecipients, recipients)
	}
	if !reflect.DeepEqual(gotTemplateData, templateData) {
		t.Fatalf("template data = %v, want %v", gotTemplateData, templateData)
	}
}

func TestEventCarriesOnlyPayload(t *testing.T) {
	event := EncodeEvent("upload", []string{"bob@example.org"}, nil)

	for key := range event.GetData().GetMap() {
		if key != recipientsKey && key != templateDataKey {
			t.Fatalf("event data contains unexpected key %q", key)
		}
	}
}

func TestDecodeEventNilIsEmpty(t *testing.T) {
	recipients, templateData, err := DecodeEvent(nil)
	if err != nil {
		t.Fatalf("DecodeEvent(nil) failed: %v", err)
	}
	if len(recipients) != 0 || len(templateData) != 0 {
		t.Fatalf("DecodeEvent(nil) = (%v, %v), want empty", recipients, templateData)
	}
}
