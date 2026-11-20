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

package ocmd

import (
	"context"
	"errors"
	"testing"
)

type recordingHandler struct{ seen []string }

func (h *recordingHandler) HandleNotification(_ context.Context, n *Notification) error {
	h.seen = append(h.seen, n.Type)
	return nil
}

func mustParse(t *testing.T, r *NotificationRegistry, body string) *Notification {
	t.Helper()
	n, err := r.Parse([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return n
}

func TestRegistryDispatchOutcomes(t *testing.T) {
	ctx := context.Background()
	r := NewNotificationRegistry()
	h := &recordingHandler{}
	if err := r.SetHandler(NotificationUserRemoved, h); err != nil {
		t.Fatal(err)
	}

	n := mustParse(t, r, `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`)
	if out, err := r.Dispatch(ctx, n); out != DispatchHandled || err != nil || len(h.seen) != 1 {
		t.Fatalf("handled %s %v %v", out, err, h.seen)
	}

	n = mustParse(t, r, `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":"g"}}}`)
	if out, err := r.Dispatch(ctx, n); out != DispatchIgnored || err != nil {
		t.Fatalf("no handler %s %v", out, err)
	}

	for _, body := range []string{
		`{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"user"}`,
		`{"notificationType":"RESHARE_UNDO","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
		`{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","shareType":"federation","notification":{"future":{"groupId":"g"}}}`,
	} {
		n = mustParse(t, r, body)
		if out, err := r.Dispatch(ctx, n); n.Class != NotificationClassUnknown || out != DispatchIgnored || err != nil {
			t.Fatalf("unknown %s: %s %s %v", body, n.Class, out, err)
		}
	}

	req, err := r.Parse([]byte(`{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"owner":"a@a.example.org","shareWith":"b@b.example.org","shareId":"x","permissions":["read"]}}}`))
	if code, ok := NotificationErrorCodeOf(err); !ok || code != NotificationErrNotImplemented || req.Class != NotificationClassRequest {
		t.Fatalf("request status %v", err)
	}
	if out, err := r.Dispatch(ctx, req); out != DispatchNotImplemented || err != nil {
		t.Fatalf("request %s %v", out, err)
	}

	malformed, err := r.Parse([]byte(`{`))
	if err == nil {
		t.Fatal("expected error")
	}
	if out, err := r.Dispatch(ctx, malformed); out != DispatchIgnored || err != nil {
		t.Fatalf("malformed %s %v", out, err)
	}
}

func TestRegistryHandlerErrorIsReturnedAsOutcome(t *testing.T) {
	r := NewNotificationRegistry()
	boom := errors.New("repository unavailable")
	_ = r.SetHandler(NotificationShareUnshared, TypeHandlerFunc(func(context.Context, *Notification) error { return boom }))
	n := mustParse(t, r, `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	out, err := r.Dispatch(context.Background(), n)
	if out != DispatchHandled || !errors.Is(err, boom) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestRegistryRegistrationRules(t *testing.T) {
	r := NewNotificationRegistry()
	h := &recordingHandler{}
	if err := r.SetHandler("ACME", h); err == nil {
		t.Fatal("handler for a type without contract")
	}
	if err := r.SetHandler("RESHARE_UNDO", h); err == nil {
		t.Fatal("RESHARE_UNDO has a handler slot")
	}
	if err := r.SetHandler(NotificationShareAccepted, nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	for _, typ := range []string{NotificationRequestShare, NotificationRequestReshare} {
		if err := r.SetHandler(typ, h); err == nil {
			t.Fatalf("request %q has a handler slot", typ)
		}
	}
	if err := r.SetHandler(NotificationShareUnshared, h); err != nil {
		t.Fatal(err)
	}
	replacement := &recordingHandler{}
	if err := r.SetHandler(NotificationShareUnshared, replacement); err != nil {
		t.Fatal(err)
	}
	n := mustParse(t, r, `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	if out, err := r.Dispatch(context.Background(), n); out != DispatchHandled || err != nil || len(h.seen) != 0 || len(replacement.seen) != 1 {
		t.Fatalf("initialization replacement %s %v: %v %v", out, err, h.seen, replacement.seen)
	}
}
