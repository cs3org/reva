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

type recordingMLS struct{ got *Notification }

func (m *recordingMLS) HandleMLSNotification(_ context.Context, n *Notification) error {
	m.got = n
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

func TestRegistryAliasNeverReachesUnsharedHandler(t *testing.T) {
	r := NewNotificationRegistry()
	unshared, undo := &recordingHandler{}, &recordingHandler{}
	if err := r.SetHandler(NotificationShareUnshared, unshared); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHandler(NotificationReshareUndo, undo); err != nil {
		t.Fatal(err)
	}
	n := mustParse(t, r, `{"notificationType":"RESHARE_UNDO","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	if out, err := r.Dispatch(context.Background(), n); out != DispatchHandled || err != nil {
		t.Fatalf("dispatch %s %v", out, err)
	}
	if len(unshared.seen) != 0 || len(undo.seen) != 1 {
		t.Fatalf("unshared %v undo %v", unshared.seen, undo.seen)
	}
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

	n = mustParse(t, r, `{"notificationType":"ACME","shareType":"user"}`)
	if out, err := r.Dispatch(ctx, n); out != DispatchIgnored || err != nil {
		t.Fatalf("unknown %s %v", out, err)
	}

	req, err := r.Parse([]byte(`{"notificationType":"REQUEST_SHARE","resourceType":"file"}`))
	if ReceiverStatus(err) != NotificationStatusNotImplemented {
		t.Fatalf("request status %v", err)
	}
	if out, err := r.Dispatch(ctx, req); out != DispatchNotImplemented || err != nil {
		t.Fatalf("request %s %v", out, err)
	}

	mls := mustParse(t, r, `{"notificationType":"MLS_WELCOME","senderDomain":"a.example.org","shareType":"federation","notification":{"federation":{"mlsGroupId":"g"}}}`)
	if out, err := r.Dispatch(ctx, mls); out != DispatchMLSUnavailable || !errors.Is(err, ErrMLSIngressUnavailable) {
		t.Fatalf("mls nil seam %s %v", out, err)
	}
	ingress := &recordingMLS{}
	r.SetMLSIngress(ingress)
	if out, err := r.Dispatch(ctx, mls); out != DispatchHandled || err != nil || ingress.got != mls {
		t.Fatalf("mls installed %s %v", out, err)
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
	if err := r.SetHandler(NotificationShareAccepted, nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	valid := func(*Notification) error { return nil }
	for _, typ := range []string{NotificationUserRemoved, NotificationReshareUndo, NotificationRequestShare, NotificationRequestReshare, NotificationMLSCommit, ""} {
		if err := r.RegisterExtension(NotificationExtension{Type: typ, Validate: valid, Handler: h}); err == nil {
			t.Fatalf("extension %q accepted", typ)
		}
	}
	if err := r.RegisterExtension(NotificationExtension{Type: "ACME", Handler: h}); err == nil {
		t.Fatal("extension without validator accepted")
	}
}

func TestRegistryExtensionBecomesKnownAndValidated(t *testing.T) {
	r := NewNotificationRegistry()
	h := &recordingHandler{}
	err := r.RegisterExtension(NotificationExtension{
		Type: "CONTACT_REMOVED",
		Validate: func(n *Notification) error {
			if err := n.normalizeCommon(); err != nil {
				return err
			}
			var u struct {
				UserID      string `json:"userId"`
				Counterpart string `json:"counterpartUserId"`
			}
			if _, err := n.payloadObject("user", &u); err != nil {
				return err
			}
			if u.Counterpart == "" {
				return missingField("notification.user.counterpartUserId")
			}
			return nil
		},
		Handler: h,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Parse([]byte(`{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`)); ReceiverStatus(err) != NotificationStatusBadRequest {
		t.Fatalf("malformed extension escaped: %v", err)
	}
	n := mustParse(t, r, `{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org","counterpartUserId":"bob@b.example.org"}}}`)
	if n.Class != NotificationClassSpec {
		t.Fatalf("class %s", n.Class)
	}
	if out, err := r.Dispatch(context.Background(), n); out != DispatchHandled || err != nil || len(h.seen) != 1 {
		t.Fatalf("%s %v", out, err)
	}
	legacy := mustParse(t, r, `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`)
	if out, _ := r.Dispatch(context.Background(), legacy); out != DispatchIgnored || len(h.seen) != 1 {
		t.Fatalf("legacy reached the extension handler: %s", out)
	}
}

func TestRegistryConcurrentSetHandlerAndDispatch(t *testing.T) {
	r := NewNotificationRegistry()
	n := mustParse(t, r, `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = r.SetHandler(NotificationShareAccepted, TypeHandlerFunc(func(context.Context, *Notification) error { return nil }))
		}
	}()
	for i := 0; i < 200; i++ {
		_, _ = r.Dispatch(context.Background(), n)
		_, _ = r.Parse([]byte(`{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`))
	}
	<-done
}
