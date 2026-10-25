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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

const receiverSecret = "receiver-secret-0f8a7c"

// newNotificationsService builds the real ocmd service, inherited ingress
// limiter included, and closes it when the test ends.
func newNotificationsService(t *testing.T) *svc {
	t.Helper()
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc":     "localhost:0",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
	})
	if err != nil {
		t.Fatalf("ocmd service: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s.(*svc)
}

var clientSeq atomic.Uint32

func postNotification(t *testing.T, h http.Handler, contentType, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	req := httptest.NewRequest(http.MethodPost, notificationsPath, strings.NewReader(body))
	// Each request comes from its own client, so that the contract tests
	// never drain one bucket of the shared ingress limiter.
	req.RemoteAddr = fmt.Sprintf("[2001:db8:%x::1]:1000", clientSeq.Add(1))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr, logs.String()
}

func checkResponse(t *testing.T, rr *httptest.ResponseRecorder, status int, apiCode reqres.APIErrorCode) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status %d, want %d; body %s", rr.Code, status, rr.Body.String())
	}
	if status == http.StatusCreated {
		if rr.Body.Len() != 0 {
			t.Fatalf("201 has a body: %q", rr.Body.String())
		}
		return
	}
	var apiErr reqres.APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil || apiErr.Code != apiCode {
		t.Fatalf("error body %q", rr.Body.String())
	}
}

// The shared envelope corpus through the real router: rejected and request
// notifications reach no registered handler.
func TestNotificationsEnvelopeCorpusThroughRouter(t *testing.T) {
	raw, err := os.ReadFile("testdata/notification-envelopes.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []envelopeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	codes := map[int]reqres.APIErrorCode{400: reqres.APIErrorInvalidParameter, 501: reqres.APIErrorUnimplemented}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			want := tc
			if d, ok := deferredEnvelopes[tc.ID]; ok {
				want.HTTP, want.HandlerCalls = d.HTTP, d.HandlerCalls
			}
			s := newNotificationsService(t)
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if !IsRequestNotification(typ) {
					if err := s.notif.registry.SetHandler(typ, h); err != nil {
						t.Fatal(err)
					}
				}
			}
			rr, _ := postNotification(t, s.Handler(), "application/json", string(tc.Body))
			checkResponse(t, rr, want.HTTP, codes[want.HTTP])
			if len(h.seen) != want.HandlerCalls {
				t.Fatalf("%s: handler calls %d, want %d", tc.Purpose, len(h.seen), want.HandlerCalls)
			}
		})
	}
}

func TestNotificationsReceiverMatrix(t *testing.T) {
	h := newNotificationsService(t).Handler()
	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
		apiCode     reqres.APIErrorCode
	}{
		{"SHARE_ACCEPTED", "application/json", `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"SHARE_DECLINED with secret", "application/json", `{"notificationType":"SHARE_DECLINED","senderDomain":"b.example.org","resourceType":"file","shareType":"user","notification":{"sharedSecret":"` + receiverSecret + `","file":{"providerId":"42"}}}`, 201, ""},
		{"SHARE_CHANGE_PERMISSION", "application/json", `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","notification":{"file":{"providerId":"42","permissions":["read"]}}}`, 201, ""},
		{"json content type with charset", "application/json; charset=utf-8", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"unknown future type", "application/json", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","shareType":"user","notification":{"sharedSecret":"` + receiverSecret + `","future":{"x":1}}}`, 201, ""},
		{"RESHARE_UNDO is unknown", "application/json", `{"notificationType":"RESHARE_UNDO","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"flat providerId only", "application/json", `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","providerId":"42","notification":{"sharedSecret":"` + receiverSecret + `"}}`, 400, reqres.APIErrorInvalidParameter},
		{"missing senderDomain", "application/json", `{"notificationType":"USER_REMOVED","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`, 400, reqres.APIErrorInvalidParameter},
		// senderDomain may include a port.
		{"senderDomain with port", "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org:443","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"senderDomain with scheme", "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"https://a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"unknown type with null selector", "application/json", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","resourceType":null}`, 400, reqres.APIErrorInvalidParameter},
		{"malformed JSON", "application/json", `{"notificationType":`, 400, reqres.APIErrorInvalidParameter},
		{"non-JSON content type", "text/plain", `{"notificationType":"SHARE_UNSHARED"}`, 400, reqres.APIErrorInvalidParameter},
		{"missing content type", "", `{"notificationType":"SHARE_UNSHARED"}`, 400, reqres.APIErrorInvalidParameter},
		{"oversized body", "application/json", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","shareType":"user","x":"` + strings.Repeat("a", maxNotificationBody) + `"}`, 400, reqres.APIErrorInvalidParameter},
		{"non-string sharedSecret", "application/json", `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"sharedSecret":42,"file":{"providerId":"42"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"userId not an OCM address", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"userId host with path", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org/path"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"userId with zoned IPv6", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@[fe80::1%25eth0]"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"REQUEST_SHARE", "application/json", `{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"owner":"a@a.example.org","shareWith":"b@b.example.org","shareId":"x","permissions":["read"]}}}`, 501, reqres.APIErrorUnimplemented},
		{"malformed REQUEST_SHARE", "application/json", `{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file"}`, 400, reqres.APIErrorInvalidParameter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr, logs := postNotification(t, h, tc.contentType, tc.body)
			if strings.Contains(logs, receiverSecret) || strings.Contains(rr.Body.String(), receiverSecret) {
				t.Fatalf("shared secret leaked: %s", logs)
			}
			checkResponse(t, rr, tc.status, tc.apiCode)
		})
	}
}

func TestNotificationsSuccessResponseBytes(t *testing.T) {
	h := newNotificationsService(t).Handler()
	rr, _ := postNotification(t, h, "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	if rr.Code != http.StatusCreated || rr.Body.Len() != 0 || rr.Header().Get("Content-Type") != "" {
		t.Fatalf("status %d body %q content-type %q", rr.Code, rr.Body.String(), rr.Header().Get("Content-Type"))
	}
}

func TestNotificationsLogsStructuredFieldsWithoutPayload(t *testing.T) {
	h := newNotificationsService(t).Handler()
	paths := map[string]string{
		"success": `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text","file":{"providerId":"42"}}}`,
		"schema":  `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
		"unknown": `{"notificationType":"ACME","senderDomain":"b.example.org","shareType":"user","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
		"request": `{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text","file":{"owner":"a@a.example.org","shareWith":"b@b.example.org","shareId":"x","permissions":["read"]}}}`,
		"decode":  `{"notificationType":"SHARE_ACCEPTED","notification":{"sharedSecret":"` + receiverSecret + `"`,
	}
	for name, body := range paths {
		t.Run(name, func(t *testing.T) {
			_, logs := postNotification(t, h, "application/json", body)
			if strings.Contains(logs, receiverSecret) || strings.Contains(logs, "payload-text") {
				t.Fatalf("raw payload logged: %s", logs)
			}
			if !strings.Contains(logs, `"class"`) || !strings.Contains(logs, `"outcome"`) {
				t.Fatalf("missing structured fields: %s", logs)
			}
		})
	}
	_, logs := postNotification(t, h, "application/json", paths["success"])
	for _, want := range []string{`"type":"SHARE_ACCEPTED"`, `"class":"A"`, `"has_shared_secret":true`, `"provider_id":"42"`, `"sender_domain":"b.example.org"`} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log lacks %s: %s", want, logs)
		}
	}
	for _, gone := range []string{"legacy", "alias", "envelope_provider_id", "missing_sender_domain", "contract"} {
		if strings.Contains(logs, `"`+gone+`"`) {
			t.Fatalf("compatibility field %s logged: %s", gone, logs)
		}
	}
}

// The core log tier performs no mutation, outbound call or delivery: the
// receiver works without any reachable gateway or peer.
func TestNotificationsLogTierNeedsNoGatewayOrPeer(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected outbound request %s %s", r.Method, r.URL)
	}))
	defer peer.Close()
	h := newNotificationsService(t).Handler()
	host := strings.TrimPrefix(peer.URL, "http://")
	for _, typ := range []string{"SHARE_ACCEPTED", "SHARE_DECLINED", "SHARE_UNSHARED", "SHARE_CHANGE_PERMISSION"} {
		rr, _ := postNotification(t, h, "application/json", `{"notificationType":"`+typ+`","senderDomain":"peer.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("%s: %d", typ, rr.Code)
		}
	}
	rr, _ := postNotification(t, h, "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"peer.example.org","shareType":"user","notification":{"user":{"userId":"alice@`+host+`"}}}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("USER_REMOVED: %d", rr.Code)
	}
}

func TestNotificationsRejectedKnownTypeReachesNoHandler(t *testing.T) {
	s := newNotificationsService(t)
	calls := 0
	count := TypeHandlerFunc(func(context.Context, *Notification) error { calls++; return nil })
	for _, typ := range []string{NotificationUserRemoved, NotificationShareUnshared, NotificationShareChangePermission} {
		if err := s.notif.registry.SetHandler(typ, count); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":null,"notification":{"user":{"userId":"leaky-value@a.example.org"}}}`,
		`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"message":["leaky-value"],"file":{"providerId":"42"}}}`,
		`{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":["leaky-value"]}}}`,
		`{"notificationType":"SHARE_UNSHARED","senderDomain":"leaky-value.example.org:bad","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
	} {
		rr, logs := postNotification(t, s.Handler(), "application/json", body)
		if rr.Code != http.StatusBadRequest || strings.Contains(rr.Body.String(), "leaky-value") || strings.Contains(logs, "leaky-value") {
			t.Fatalf("status %d body %s logs %s", rr.Code, rr.Body.String(), logs)
		}
	}
	if calls != 0 {
		t.Fatalf("handler ran %d times for invalid notifications", calls)
	}
	rr, _ := postNotification(t, s.Handler(), "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`)
	if rr.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("valid notification: status %d calls %d", rr.Code, calls)
	}
}

// Representative router checks of the shared notificationType corpus: the
// type is kept exactly and an unknown type reaches no handler.
func TestNotificationsTypeBlanknessThroughRouter(t *testing.T) {
	s := newNotificationsService(t)
	h := &recordingHandler{}
	for typ := range builtinContracts {
		if !IsRequestNotification(typ) {
			if err := s.notif.registry.SetHandler(typ, h); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		typ    string
		status int
	}{
		{`\u0085`, http.StatusBadRequest},
		{`\ufeff`, http.StatusCreated},
		{`\u001c`, http.StatusCreated},
		{`\u0085EXT\u2003`, http.StatusCreated},
		{" SHARE_UNSHARED ", http.StatusCreated},
	} {
		rr, logs := postNotification(t, s.Handler(), "application/json",
			`{"notificationType":"`+tc.typ+`","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
		code := reqres.APIErrorCode("")
		if tc.status == http.StatusBadRequest {
			code = reqres.APIErrorInvalidParameter
		}
		checkResponse(t, rr, tc.status, code)
		if tc.status == http.StatusCreated && !loggedType(logs, mustUnquoteJSON(t, tc.typ)) {
			t.Fatalf("%q: type not kept exactly: %s", tc.typ, logs)
		}
	}
	if len(h.seen) != 0 {
		t.Fatalf("handlers ran: %v", h.seen)
	}
}

func loggedType(logs, typ string) bool {
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["type"] == typ {
			return true
		}
	}
	return false
}

func mustUnquoteJSON(t *testing.T, s string) string {
	t.Helper()
	var v string
	if err := json.Unmarshal([]byte(`"`+s+`"`), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A caller-chosen extension member name and an ordinary secret value never
// reach the response or the logs.
func TestNotificationsDiagnosticsCarryNoCallerText(t *testing.T) {
	key := "key-marker-" + strings.Repeat("k", 108)
	const value = "secret-value-marker-0b5e"
	s := newNotificationsService(t)
	h := &recordingHandler{}
	if err := s.notif.registry.SetHandler(NotificationShareUnshared, h); err != nil {
		t.Fatal(err)
	}
	if err := s.notif.registry.RegisterExtension(NotificationExtension{
		Type:     "EXAMPLE_WRAP",
		Validate: func(*Notification) error { return fmt.Errorf("%s: %w", key, invalidField("notification")) },
		Handler:  h,
	}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":"` + value + `","` + key + `":"x","file":{"providerId":"42"}}}`,
		`{"notificationType":"EXAMPLE_WRAP","senderDomain":"a.example.org","shareType":"user","notification":{"sharedSecret":"` + value + `"}}`,
	} {
		rr, logs := postNotification(t, s.Handler(), "application/json", body)
		checkResponse(t, rr, http.StatusBadRequest, reqres.APIErrorInvalidParameter)
		out := rr.Body.String() + logs
		if strings.Contains(out, key) || strings.Contains(out, value) || !strings.Contains(out, "invalid notification") {
			t.Fatalf("caller text in diagnostics: %s", out)
		}
	}
	if len(h.seen) != 0 {
		t.Fatalf("handlers ran: %v", h.seen)
	}
}
