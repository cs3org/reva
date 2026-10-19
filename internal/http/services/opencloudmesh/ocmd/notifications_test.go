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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

const receiverSecret = "receiver-secret-0f8a7c"

// newNotificationsRouter builds the real ocmd service router.
func newNotificationsRouter(t *testing.T) http.Handler {
	t.Helper()
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc":     "localhost:0",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
	})
	if err != nil {
		t.Fatalf("ocmd service: %v", err)
	}
	return s.Handler()
}

func postNotification(t *testing.T, h http.Handler, contentType, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	req := httptest.NewRequest(http.MethodPost, notificationsPath, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr, logs.String()
}

func TestNotificationsReceiverMatrix(t *testing.T) {
	h := newNotificationsRouter(t)
	cases := []struct {
		name        string
		class       string
		contentType string
		body        string
		status      int
		apiCode     reqres.APIErrorCode
	}{
		{"A SHARE_ACCEPTED", "A", "application/json", `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"A SHARE_DECLINED", "A", "application/json", `{"notificationType":"SHARE_DECLINED","senderDomain":"b.example.org","resourceType":"file","shareType":"user","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"A SHARE_UNSHARED", "A", "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"A SHARE_CHANGE_PERMISSION", "A", "application/json", `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","notification":{"file":{"providerId":"42","permissions":["read"]}}}`, 201, ""},
		{"A USER_REMOVED", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`, 201, ""},
		{"A GROUP_REMOVED", "A", "application/json", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":"g1"}}}`, 201, ""},
		{"A json content type with charset", "A", "application/json; charset=utf-8", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`, 201, ""},
		{"B Nextcloud flat SHARE_ACCEPTED", "B", "application/json", `{"notificationType":"SHARE_ACCEPTED","resourceType":"file","providerId":"42","notification":{"sharedSecret":"` + receiverSecret + `","message":"m","shareWith":"bob","senderId":"alice","shareType":"user"}}`, 201, ""},
		{"B RESHARE_UNDO alias", "B", "application/json", `{"notificationType":"RESHARE_UNDO","resourceType":"file","providerId":"42","notification":{"sharedSecret":"` + receiverSecret + `"}}`, 201, ""},
		{"B USER_REMOVED without senderDomain", "B", "application/json", `{"notificationType":"USER_REMOVED","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`, 201, ""},
		{"C unknown type", "C", "application/json", `{"notificationType":"ACME_EVENT","senderDomain":"a.example.org","notification":{"sharedSecret":"` + receiverSecret + `"}}`, 201, ""},
		{"C CONTACT_REMOVED before its handler exists", "C", "application/json", `{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`, 201, ""},
		{"C MLS type without MLS support", "C", "application/json", `{"notificationType":"MLS_COMMIT","senderDomain":"a.example.org","shareType":"federation","notification":{"federation":{"mlsGroupId":"g"}}}`, 201, ""},
		{"D malformed JSON", "D", "application/json", `{"notificationType":`, 400, reqres.APIErrorInvalidParameter},
		{"D non-JSON content type", "D", "text/plain", `{"notificationType":"SHARE_UNSHARED"}`, 400, reqres.APIErrorInvalidParameter},
		{"D missing content type", "D", "", `{"notificationType":"SHARE_UNSHARED"}`, 400, reqres.APIErrorInvalidParameter},
		{"D missing notificationType", "D", "application/json", `{"senderDomain":"a.example.org","resourceType":"file"}`, 400, reqres.APIErrorInvalidParameter},
		{"D oversized body", "D", "application/json", `{"notificationType":"ACME","x":"` + strings.Repeat("a", maxNotificationBody) + `"}`, 400, reqres.APIErrorInvalidParameter},
		{"400 providerId missing", "A", "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":"` + receiverSecret + `"}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 userId missing", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 userId not an OCM address", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 groupId missing", "A", "application/json", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":""}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 non-string sharedSecret", "B", "application/json", `{"notificationType":"SHARE_ACCEPTED","resourceType":"file","providerId":"42","notification":{"sharedSecret":42}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 null resourceType on USER_REMOVED", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":null,"notification":{"user":{"userId":"alice@a.example.org"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 empty resourceType on USER_REMOVED", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":"","notification":{"user":{"userId":"alice@a.example.org"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 numeric message", "A", "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"message":17,"file":{"providerId":"42"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 undefined permission", "A", "application/json", `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":["admin"]}}}`, 400, reqres.APIErrorInvalidParameter},
		{"400 userId host with path", "A", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org/path"}}}`, 400, reqres.APIErrorInvalidParameter},
		{"201 unknown type with null and odd fields", "C", "application/json", `{"notificationType":"ACME_EVENT","senderDomain":"a.example.org","resourceType":null,"notification":{"message":17,"user":null}}`, 201, ""},
		{"R REQUEST_SHARE", "R", "application/json", `{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"owner":"a@a.org","shareWith":"b@b.org","shareId":"x","permissions":["read"]}}}`, 501, reqres.APIErrorUnimplemented},
		{"R REQUEST_RESHARE", "R", "application/json", `{"notificationType":"REQUEST_RESHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42","shareWith":"c@c.org"}}}`, 501, reqres.APIErrorUnimplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr, logs := postNotification(t, h, tc.contentType, tc.body)
			if rr.Code != tc.status {
				t.Fatalf("status %d, want %d; body %s", rr.Code, tc.status, rr.Body.String())
			}
			if strings.Contains(logs, receiverSecret) || strings.Contains(rr.Body.String(), receiverSecret) {
				t.Fatalf("shared secret leaked: %s", logs)
			}
			if tc.status == http.StatusCreated {
				if rr.Body.Len() != 0 {
					t.Fatalf("201 has a body: %q", rr.Body.String())
				}
				return
			}
			var apiErr reqres.APIError
			if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil || apiErr.Code != tc.apiCode {
				t.Fatalf("error body %q", rr.Body.String())
			}
		})
	}
}

func TestNotificationsSuccessResponseBytes(t *testing.T) {
	h := newNotificationsRouter(t)
	rr, _ := postNotification(t, h, "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	if rr.Code != http.StatusCreated || rr.Body.Len() != 0 || rr.Header().Get("Content-Type") != "" {
		t.Fatalf("status %d body %q content-type %q", rr.Code, rr.Body.String(), rr.Header().Get("Content-Type"))
	}
}

func TestNotificationsLogsStructuredFieldsWithoutPayload(t *testing.T) {
	h := newNotificationsRouter(t)
	paths := map[string]string{
		"success": `{"notificationType":"SHARE_ACCEPTED","resourceType":"file","providerId":"42","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
		"schema":  `{"notificationType":"SHARE_ACCEPTED","resourceType":"file","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
		"unknown": `{"notificationType":"ACME","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
		"request": `{"notificationType":"REQUEST_SHARE","notification":{"sharedSecret":"` + receiverSecret + `","message":"payload-text"}}`,
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
	for _, want := range []string{`"type":"SHARE_ACCEPTED"`, `"class":"B"`, `"envelope_provider_id":true`, `"missing_sender_domain":true`, `"has_shared_secret":true`, `"provider_id":"42"`} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log lacks %s: %s", want, logs)
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
	h := newNotificationsRouter(t)
	host := strings.TrimPrefix(peer.URL, "http://")
	for _, typ := range []string{"SHARE_ACCEPTED", "SHARE_DECLINED", "SHARE_UNSHARED", "SHARE_CHANGE_PERMISSION", "RESHARE_UNDO"} {
		rr, _ := postNotification(t, h, "application/json", `{"notificationType":"`+typ+`","senderDomain":"`+host+`","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("%s: %d", typ, rr.Code)
		}
	}
	rr, _ := postNotification(t, h, "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"`+host+`","shareType":"user","notification":{"user":{"userId":"alice@`+host+`"}}}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("USER_REMOVED: %d", rr.Code)
	}
}

func TestNotificationsRejectedKnownTypeReachesNoHandler(t *testing.T) {
	h := &notifHandler{}
	if err := h.init(nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	count := TypeHandlerFunc(func(context.Context, *Notification) error { calls++; return nil })
	for _, typ := range []string{NotificationUserRemoved, NotificationShareUnshared, NotificationShareChangePermission} {
		if err := h.registry.SetHandler(typ, count); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":null,"notification":{"user":{"userId":"leaky-value@a.example.org"}}}`,
		`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"message":["leaky-value"],"file":{"providerId":"42"}}}`,
		`{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":["leaky-value"]}}}`,
	} {
		rr, logs := postNotification(t, http.HandlerFunc(h.Notifications), "application/json", body)
		if rr.Code != http.StatusBadRequest || strings.Contains(rr.Body.String(), "leaky-value") || strings.Contains(logs, "leaky-value") {
			t.Fatalf("status %d body %s logs %s", rr.Code, rr.Body.String(), logs)
		}
	}
	if calls != 0 {
		t.Fatalf("handler ran %d times for invalid notifications", calls)
	}
	rr, _ := postNotification(t, http.HandlerFunc(h.Notifications), "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`)
	if rr.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("valid notification: status %d calls %d", rr.Code, calls)
	}
}
