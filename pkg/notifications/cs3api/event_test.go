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

package cs3api

import (
	"reflect"
	"testing"
)

func TestEventRoundTripKeepsNotificationPayload(t *testing.T) {
	recipients := []string{"bob@example.org", "carol@example.org"}
	templateData := map[string]any{"share_id": "share-1", "resource_name": "beach.png"}

	event, err := NewEvent("share-creation", recipients, templateData)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	req, err := SendRequestFromEvent(event, "alice", "alice@example.org")
	if err != nil {
		t.Fatalf("SendRequestFromEvent failed: %v", err)
	}

	if req.EventType != "share-creation" {
		t.Fatalf("event type = %q, want share-creation", req.EventType)
	}
	if !reflect.DeepEqual(req.Recipients, recipients) {
		t.Fatalf("recipients = %v, want %v", req.Recipients, recipients)
	}
	if !reflect.DeepEqual(req.TemplateData, templateData) {
		t.Fatalf("template data = %v, want %v", req.TemplateData, templateData)
	}
	if req.SubmittingUser != "alice" || req.Sender != "alice@example.org" {
		t.Fatalf("identity = (%q, %q), want (alice, alice@example.org)", req.SubmittingUser, req.Sender)
	}
}

func TestEventCarriesNoIdentity(t *testing.T) {
	event, err := NewEvent("upload", []string{"bob@example.org"}, nil)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	for key := range event.GetData().GetMap() {
		if key != recipientsKey && key != templateDataKey {
			t.Fatalf("event data contains unexpected key %q", key)
		}
	}

	req, err := SendRequestFromEvent(event, "", "")
	if err != nil {
		t.Fatalf("SendRequestFromEvent failed: %v", err)
	}
	if req.SubmittingUser != "" || req.Sender != "" {
		t.Fatalf("identity = (%q, %q), want empty: the gateway resolves it", req.SubmittingUser, req.Sender)
	}
}

func TestSendRequestFromEventRejectsMissingEvent(t *testing.T) {
	if _, err := SendRequestFromEvent(nil, "alice", "alice@example.org"); err == nil {
		t.Fatal("SendRequestFromEvent(nil) succeeded, want error")
	}
}
