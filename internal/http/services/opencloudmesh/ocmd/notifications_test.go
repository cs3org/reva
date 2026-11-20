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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

func newNotificationTestService(t *testing.T, mode, endpoint string, gw gateway.GatewayAPIClient) *svc {
	t.Helper()
	stampGateway(gw)
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc":     "localhost:0",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
		"http_sig_mode":  mode,
		"endpoint":       endpoint,
	})
	if err != nil {
		t.Fatalf("ocmd service: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sv := s.(*svc)
	sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})
	return sv
}

func newNotificationsService(t *testing.T) (*svc, string) {
	t.Helper()
	disco := ocmDiscoveryServer(t, "webdav", "file")
	t.Cleanup(disco.Close)
	return newNotificationTestService(t, "permissive", "", &notificationsMockGW{}), disco.Listener.Addr().String()
}

type notificationsMockGW struct {
	gateway.GatewayAPIClient
}

func (m *notificationsMockGW) IsProviderAllowed(context.Context, *ocmprovider.IsProviderAllowedRequest, ...grpc.CallOption) (*ocmprovider.IsProviderAllowedResponse, error) {
	return &ocmprovider.IsProviderAllowedResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil
}

func handlerFor(s *svc) http.Handler {
	r := router.New()
	s.Routes(r.Service("ocmd"))
	return r
}

var clientSeq atomic.Uint32

func postNotification(t *testing.T, h http.Handler, contentType, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	req := httptest.NewRequest(http.MethodPost, "/ocm/notifications", strings.NewReader(body))
	// Use separate clients so these requests do not exhaust one rate limit bucket.
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

func TestNotificationsReceiverMatrix(t *testing.T) {
	s, host := newNotificationsService(t)
	h := handlerFor(s)
	calls := 0
	for _, typ := range []string{NotificationShareUnshared, NotificationUserRemoved} {
		if err := s.notifications.registry.SetHandler(typ, TypeHandlerFunc(func(context.Context, *Notification) error { calls++; return nil })); err != nil {
			t.Fatal(err)
		}
	}
	envelope := `"senderDomain":"` + host + `","resourceType":"file"`
	valid := `{"notificationType":"SHARE_UNSHARED",` + envelope + `,"notification":{"file":{"providerId":"42"}}}`
	for _, tc := range []struct {
		name, contentType, body string
		status, calls           int
		apiCode                 reqres.APIErrorCode
		ignoredType             string
	}{
		{"known", "application/json", valid, 201, 1, "", ""},
		{"charset", "application/json; charset=utf-8", valid, 201, 1, "", ""},
		{"unknown", "application/json", `{"notificationType":"FUTURE",` + envelope + `}`, 201, 0, "", "FUTURE"},
		{"padded", "application/json", `{"notificationType":" SHARE_UNSHARED ",` + envelope + `}`, 201, 0, "", " SHARE_UNSHARED "},
		{"whitespace", "application/json", `{"notificationType":" \t",` + envelope + `}`, 201, 0, "", " \t"},
		{"known unhandled", "application/json", `{"notificationType":"CONTACT_REMOVED","senderDomain":"` + host + `","shareType":"user","notification":{"user":{"userId":"a@peer.example","targetUserId":"b@local.example"}}}`, 201, 0, "", "CONTACT_REMOVED"},
		{"malformed JSON", "application/json", `{`, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"invalid envelope", "application/json", `{"notificationType":"FUTURE","resourceType":"file"}`, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"invalid known payload", "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"` + host + `","shareType":"user","notification":{"user":{"userId":"invalid"}}}`, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"invalid request", "application/json", `{"notificationType":"REQUEST_SHARE",` + envelope + `}`, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"non JSON", "text/plain", valid, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"missing content type", "", valid, 400, 0, reqres.APIErrorInvalidParameter, ""},
		{"oversized", "application/json", valid + strings.Repeat(" ", maxNotificationBody), 400, 0, reqres.APIErrorInvalidParameter, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			rr, logs := postNotification(t, h, tc.contentType, tc.body)
			checkResponse(t, rr, tc.status, tc.apiCode)
			if calls-before != tc.calls {
				t.Fatalf("handler calls %d, want %d", calls-before, tc.calls)
			}
			if tc.ignoredType != "" && (!loggedType(logs, tc.ignoredType) || !strings.Contains(logs, "OCM notification ignored") || strings.Contains(logs, "unknown type")) {
				t.Fatalf("incorrect ignored diagnostic: %s", logs)
			}
		})
	}
}

func TestNotificationsSuccessResponseBytes(t *testing.T) {
	s, host := newNotificationsService(t)
	rr, _ := postNotification(t, handlerFor(s), "application/json", `{"notificationType":"SHARE_UNSHARED","senderDomain":"`+host+`","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
	if rr.Code != http.StatusCreated || rr.Body.Len() != 0 || rr.Header().Get("Content-Type") != "" {
		t.Fatalf("status %d body %q content-type %q", rr.Code, rr.Body.String(), rr.Header().Get("Content-Type"))
	}
}

func TestNotificationsLogsWithoutPayload(t *testing.T) {
	s, host := newNotificationsService(t)
	for _, tc := range []struct {
		name, typ, payload string
		status             int
	}{
		{"success", "SHARE_ACCEPTED", `,"file":{"providerId":"42"}`, 201},
		{"schema", "SHARE_ACCEPTED", "", 400},
		{"unknown", "FUTURE", "", 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"notificationType":"` + tc.typ + `","senderDomain":"` + host + `","resourceType":"file","sharedSecret":"secret-marker","notification":{"message":"payload-marker"` + tc.payload + `}}`
			rr, logs := postNotification(t, handlerFor(s), "application/json", body)
			if rr.Code != tc.status {
				t.Fatalf("status %d, want %d", rr.Code, tc.status)
			}
			for _, marker := range []string{"secret-marker", "payload-marker", "has_shared_secret"} {
				if strings.Contains(logs+rr.Body.String(), marker) {
					t.Fatalf("confidential or retired field in diagnostics: %s %s", rr.Body.String(), logs)
				}
			}
		})
	}
}

func TestNotificationsDefaultHandlersMakeNoEffectCalls(t *testing.T) {
	unrelatedPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected outbound request %s %s", r.Method, r.URL)
	}))
	t.Cleanup(unrelatedPeer.Close)
	s, host := newNotificationsService(t)
	h := handlerFor(s)
	for _, typ := range []string{"SHARE_ACCEPTED", "SHARE_DECLINED", "SHARE_UNSHARED", "SHARE_CHANGE_PERMISSION"} {
		rr, _ := postNotification(t, h, "application/json", `{"notificationType":"`+typ+`","senderDomain":"`+host+`","resourceType":"file","notification":{"file":{"providerId":"42"}}}`)
		checkResponse(t, rr, http.StatusCreated, "")
	}
	userHost := strings.TrimPrefix(unrelatedPeer.URL, "http://")
	rr, _ := postNotification(t, h, "application/json", `{"notificationType":"USER_REMOVED","senderDomain":"`+host+`","shareType":"user","notification":{"user":{"userId":"alice@`+userHost+`"}}}`)
	checkResponse(t, rr, http.StatusCreated, "")
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

func TestNotificationsDiagnosticsCarryNoCallerText(t *testing.T) {
	s, host := newNotificationsService(t)
	key := "key-marker-" + strings.Repeat("k", 108)
	const value = "secret-value-marker-0b5e"
	for _, tc := range []struct {
		name, extra, payload string
		status               int
	}{
		{"malformed", `"` + key + `":"` + value + `",`, `"` + key + `":"` + value + `"`, 400},
		{"opaque envelope string", `"sharedSecret":"` + value + `","` + key + `":42,`, `"file":{"providerId":"42"}`, 201},
		{"opaque envelope number", `"sharedSecret":42,`, `"file":{"providerId":"42"},"` + key + `":{"value":"` + value + `"}`, 201},
		{"opaque envelope object", `"sharedSecret":{"value":"` + value + `"},`, `"file":{"providerId":"42"}`, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + host + `","resourceType":"file",` + tc.extra + `"notification":{` + tc.payload + `}}`
			rr, logs := postNotification(t, handlerFor(s), "application/json", body)
			checkResponse(t, rr, tc.status, reqres.APIErrorInvalidParameter)
			if strings.Contains(rr.Body.String()+logs, key) || strings.Contains(rr.Body.String()+logs, value) {
				t.Fatalf("caller text in diagnostics: %s %s", rr.Body.String(), logs)
			}
		})
	}
}

func TestNotificationsHandlerFailure(t *testing.T) {
	s, host := newNotificationsService(t)
	body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + host + `","resourceType":"file","notification":{"message":"payload-marker","file":{"providerId":"42"}}}`
	for _, fail := range []bool{true, false} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			calls := 0
			if err := s.notifications.registry.SetHandler(NotificationShareUnshared, TypeHandlerFunc(func(context.Context, *Notification) error {
				calls++
				if fail {
					return errors.New("handler-error-marker payload-marker")
				}
				return nil
			})); err != nil {
				t.Fatal(err)
			}
			rr, logs := postNotification(t, handlerFor(s), "application/json", body)
			if calls != 1 {
				t.Fatalf("handler calls %d, want 1", calls)
			}
			if strings.Contains(logs+rr.Body.String(), "handler-error-marker") || strings.Contains(logs+rr.Body.String(), "payload-marker") {
				t.Fatal("handler error or payload appeared in diagnostics")
			}
			if fail {
				checkResponse(t, rr, 500, reqres.APIErrorServerError)
				var apiErr reqres.APIError
				if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil || apiErr.Message != "error handling OCM notification" {
					t.Fatalf("unexpected failure response: %s", rr.Body.String())
				}
			} else {
				checkResponse(t, rr, 201, "")
			}
		})
	}
}

type notificationsDenyGW struct {
	gateway.GatewayAPIClient
}

func (m *notificationsDenyGW) IsProviderAllowed(context.Context, *ocmprovider.IsProviderAllowedRequest, ...grpc.CallOption) (*ocmprovider.IsProviderAllowedResponse, error) {
	return &ocmprovider.IsProviderAllowedResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "policy denial"},
	}, nil
}

func TestNotificationsSignatureModeMatrix(t *testing.T) {
	body := func(domain string) string {
		return `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + domain + `","resourceType":"file","notification":{"file":{"providerId":"42"}}}`
	}

	t.Run("permissive unsigned advertiser rejected", func(t *testing.T) {
		key := newSigningKey(t)
		peer := signingServer(t, key)
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "permissive", "", &notificationsMockGW{}))
		rr, _ := postNotification(t, h, "application/json", body(peer.Listener.Addr().String()))
		checkResponse(t, rr, http.StatusUnauthorized, reqres.APIErrorUnauthenticated)
	})

	t.Run("strict unsigned nonadvertiser rejected", func(t *testing.T) {
		peer := ocmDiscoveryServer(t, "webdav", "file")
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "strict", "", &notificationsMockGW{}))
		rr, _ := postNotification(t, h, "application/json", body(peer.Listener.Addr().String()))
		checkResponse(t, rr, http.StatusUnauthorized, reqres.APIErrorUnauthenticated)
	})

	t.Run("permissive unsigned nonadvertiser accepted", func(t *testing.T) {
		peer := ocmDiscoveryServer(t, "webdav", "file")
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "permissive", "", &notificationsMockGW{}))
		rr, _ := postNotification(t, h, "application/json", body(peer.Listener.Addr().String()))
		checkResponse(t, rr, http.StatusCreated, "")
	})

	t.Run("valid signature accepted in strict mode", func(t *testing.T) {
		key := newSigningKey(t)
		peer := signingServer(t, key)
		t.Cleanup(peer.Close)
		// An empty endpoint makes verification reconstruct the URI from the request.
		h := handlerFor(newNotificationTestService(t, "strict", "", &notificationsMockGW{}))
		payload := body(peer.Listener.Addr().String())
		req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if err := key.Sign(req, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("signed request: status %d, want 201", rr.Code)
		}
	})

	t.Run("tampered body rejected in both modes", func(t *testing.T) {
		for _, mode := range []string{"permissive", "strict"} {
			t.Run(mode, func(t *testing.T) {
				key := newSigningKey(t)
				peer := signingServer(t, key)
				t.Cleanup(peer.Close)
				h := handlerFor(newNotificationTestService(t, mode, "", &notificationsMockGW{}))
				payload := body(peer.Listener.Addr().String())
				req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				// Sign altered bytes but send the original body.
				other := strings.Replace(payload, `"42"`, `"43"`, 1)
				if err := key.Sign(req, []byte(other)); err != nil {
					t.Fatal(err)
				}
				tampered := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
				tampered.Header = req.Header
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, tampered)
				if rr.Code != http.StatusUnauthorized {
					t.Fatalf("tampered body: status %d, want 401", rr.Code)
				}
			})
		}
	})

	t.Run("policy denial rejects before any effect", func(t *testing.T) {
		key := newSigningKey(t)
		peer := signingServer(t, key)
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "permissive", "", &notificationsDenyGW{}))
		payload := body(peer.Listener.Addr().String())
		req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if err := key.Sign(req, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		checkResponse(t, rr, http.StatusUnauthorized, reqres.APIErrorUnauthenticated)
	})

	t.Run("unknown valid type authenticates first", func(t *testing.T) {
		key := newSigningKey(t)
		peer := signingServer(t, key)
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "strict", "", &notificationsMockGW{}))
		payload := `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"` + peer.Listener.Addr().String() + `","resourceType":"file"}`
		req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if err := key.Sign(req, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("valid unknown type: status %d, want bodyless 201", rr.Code)
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("unknown type: body %q", rr.Body.String())
		}
	})

	t.Run("malformed known type stays 400 with a valid signature", func(t *testing.T) {
		key := newSigningKey(t)
		peer := signingServer(t, key)
		t.Cleanup(peer.Close)
		h := handlerFor(newNotificationTestService(t, "strict", "", &notificationsMockGW{}))
		payload := `{"notificationType":"USER_REMOVED","senderDomain":"` + peer.Listener.Addr().String() + `","shareType":"group","notification":{"user":{"userId":"alice@` + peer.Listener.Addr().String() + `"}}}`
		req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if err := key.Sign(req, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		checkResponse(t, rr, http.StatusBadRequest, reqres.APIErrorInvalidParameter)
	})

	t.Run("undiscoverable sender is a fixed dependency error", func(t *testing.T) {
		h := handlerFor(newNotificationTestService(t, "permissive", "", &notificationsMockGW{}))
		rr, _ := postNotification(t, h, "application/json", body("192.0.2.1"))
		checkResponse(t, rr, http.StatusInternalServerError, reqres.APIErrorServerError)
		if strings.Contains(rr.Body.String(), "192.0.2.1") {
			t.Fatalf("untrusted sender value leaked into the response")
		}
	})
}
