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

package ocmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type notificationPolicyTrace struct {
	gateway.GatewayAPIClient
	record  func(string)
	deny    bool
	missing string
	sender  string
}

func (g *notificationPolicyTrace) IsProviderAllowed(_ context.Context, req *ocmprovider.IsProviderAllowedRequest, _ ...grpc.CallOption) (*ocmprovider.IsProviderAllowedResponse, error) {
	g.record("policy")
	g.sender = req.Provider.Domain
	if g.missing == "response" {
		return nil, nil
	}
	if g.missing == "status" {
		return &ocmprovider.IsProviderAllowedResponse{}, nil
	}
	code := rpc.Code_CODE_OK
	if g.deny {
		code = rpc.Code_CODE_PERMISSION_DENIED
	}
	return &ocmprovider.IsProviderAllowedResponse{Status: &rpc.Status{Code: code}}, nil
}

func TestNotificationsIngressOrder(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		want       int
		sequence   string
	}{
		{"malformed", "malformed", http.StatusBadRequest, ""},
		{"denied", "denied", http.StatusUnauthorized, "policy"},
		{"nil response", "response", http.StatusInternalServerError, "policy"},
		{"nil status", "status", http.StatusInternalServerError, "policy"},
		{"unsigned", "unsigned", http.StatusUnauthorized, "policy,discovery"},
		{"signed", "signed", http.StatusCreated, "policy,discovery,jwks,dispatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var events []string
			record := func(event string) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }
			key := newSigningKey(t)
			jwks, err := key.JWKS()
			if err != nil {
				t.Fatal(err)
			}
			var senderHost string
			mux := http.NewServeMux()
			mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
				if r.Host != senderHost {
					t.Errorf("discovery host %q, want %q", r.Host, senderHost)
				}
				record("discovery")
				_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{Endpoint: "http://" + r.Host, Capabilities: []string{"http-sig"}, JwksURI: "http://" + r.Host + "/jwks"})
			})
			mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) { record("jwks"); _, _ = w.Write(jwks) })
			peer := httptest.NewServer(mux)
			t.Cleanup(peer.Close)
			sender := peer.Listener.Addr().String()
			if tc.kind == "denied" {
				sender = "PEER.EXAMPLE:8443"
			}
			senderHost = strings.ToLower(sender)
			policy := &notificationPolicyTrace{record: record, deny: tc.kind == "denied", missing: tc.kind}
			stampGateway(policy)
			s, err := New(context.Background(), map[string]any{
				"gatewaysvc": "localhost:0", "http_sig_mode": "strict",
				"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			sv := s.(*svc)
			sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})
			if err := sv.notifications.registry.SetHandler(NotificationShareUnshared, TypeHandlerFunc(func(context.Context, *Notification) error { record("dispatch"); return nil })); err != nil {
				t.Fatal(err)
			}
			body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + sender + `","resourceType":"file","notification":{"file":{"providerId":"42"}}}`
			if tc.kind == "malformed" {
				body = "{"
			}
			req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = fmt.Sprintf("[2001:db8:%x::1]:1000", clientSeq.Add(1))
			if tc.kind == "signed" || tc.kind == "denied" {
				if err := key.Sign(req, []byte(body)); err != nil {
					t.Fatal(err)
				}
			}
			rr := httptest.NewRecorder()
			handlerFor(sv).ServeHTTP(rr, req)
			mu.Lock()
			got := strings.Join(events, ",")
			mu.Unlock()
			if rr.Code != tc.want || got != tc.sequence {
				t.Fatalf("status %d, sequence %q; want %d, %q", rr.Code, got, tc.want, tc.sequence)
			}
			if tc.kind != "malformed" && policy.sender != senderHost {
				t.Fatalf("policy sender %q, want %q", policy.sender, senderHost)
			}
			if tc.kind == "response" || tc.kind == "status" {
				checkResponse(t, rr, 500, reqres.APIErrorServerError)
				var apiErr reqres.APIError
				if err := json.Unmarshal(rr.Body.Bytes(), &apiErr); err != nil || apiErr.Message != "invalid provider policy response" {
					t.Fatalf("unexpected dependency response: %s", rr.Body.String())
				}
			}
		})
	}
}

func TestNotificationsSignatureRejectsTampering(t *testing.T) {
	const endpoint = "https://receiver.example.org:8443"
	const marker = "untrusted-key-marker"
	for _, tc := range []struct {
		name string
		kind string
		want int
	}{
		{"valid", "valid", http.StatusCreated},
		{"different method", "method", http.StatusUnauthorized},
		{"different public port", "port", http.StatusUnauthorized},
		{"missing signed length", "length", http.StatusUnauthorized},
		{"wrong key", "key", http.StatusUnauthorized},
		{"expired signature", "expired", http.StatusUnauthorized},
		{"future signature", "future", http.StatusUnauthorized},
		{"unknown key id", "keyid", http.StatusUnauthorized},
		{"malformed signature", "signature", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := newSigningKey(t)
			peer := signingServer(t, key)
			t.Cleanup(peer.Close)
			stampGateway(&notificationsMockGW{})
			s, err := New(context.Background(), map[string]any{
				"gatewaysvc": "localhost:0", "endpoint": endpoint,
				"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
				"http_sig_mode":  "strict",
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			sv := s.(*svc)
			sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})
			handled := &recordingHandler{}
			if err := sv.notifications.registry.SetHandler(NotificationShareUnshared, handled); err != nil {
				t.Fatal(err)
			}
			body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + peer.Listener.Addr().String() + `","resourceType":"file","notification":{"file":{"providerId":"42"}}}`
			signURL := endpoint + mount + notificationsPath
			method := http.MethodPost
			if tc.kind == "method" {
				method = http.MethodGet
			}
			if tc.kind == "port" {
				signURL = "https://receiver.example.org:9443" + mount + notificationsPath
			}
			req := httptest.NewRequest(method, signURL, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			signer := key
			if tc.kind == "key" {
				signer = newSigningKey(t)
			}
			if tc.kind == "expired" || tc.kind == "future" {
				when := time.Now().Add(-10 * time.Minute)
				if tc.kind == "future" {
					when = time.Now().Add(10 * time.Minute)
				}
				synctest.Test(t, func(t *testing.T) {
					time.Sleep(time.Until(when))
					if err := signer.Sign(req, []byte(body)); err != nil {
						t.Fatal(err)
					}
				})
			} else if err := signer.Sign(req, []byte(body)); err != nil {
				t.Fatal(err)
			}
			req.Method = http.MethodPost
			req.Host = "receiver.example.org:8443"
			if tc.kind == "length" {
				req.Header.Del("Content-Length")
			}
			if tc.kind == "keyid" {
				req.Header.Set("Signature-Input", strings.Replace(req.Header.Get("Signature-Input"), key.ID, marker, 1))
			}
			if tc.kind == "signature" {
				req.Header.Set("Signature", marker)
			}
			req.RemoteAddr = fmt.Sprintf("[2001:db8:%x::1]:1000", clientSeq.Add(1))
			var logs bytes.Buffer
			logger := zerolog.New(&logs)
			req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
			rr := httptest.NewRecorder()
			handlerFor(sv).ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusCreated && len(handled.seen) != 0 {
				t.Fatal("rejected request reached the notification handler")
			}
			if tc.want == http.StatusCreated && len(handled.seen) != 1 {
				t.Fatal("valid signed request did not reach the notification handler")
			}
			if strings.Contains(rr.Body.String(), marker) || strings.Contains(logs.String(), marker) {
				t.Fatal("untrusted signature input appeared in response or logs")
			}
		})
	}
}

func TestNotificationsUnimplementedRequestAuthenticatesFirst(t *testing.T) {
	key := newSigningKey(t)
	peer := signingServer(t, key)
	t.Cleanup(peer.Close)
	stampGateway(&notificationsMockGW{})
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc": "localhost:0", "http_sig_mode": "strict",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sv := s.(*svc)
	sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})
	for _, tc := range []struct {
		typ, payload string
	}{
		{NotificationRequestShare, `"owner":"a@a.example.org","shareWith":"b@b.example.org","shareId":"x","permissions":["read"]`},
		{NotificationRequestReshare, `"providerId":"42","shareWith":"b@b.example.org"`},
	} {
		for _, signature := range []string{"unsigned", "valid", "tampered"} {
			t.Run(tc.typ+"/"+signature, func(t *testing.T) {
				body := `{"notificationType":"` + tc.typ + `","senderDomain":"` + peer.Listener.Addr().String() + `","resourceType":"file","notification":{"file":{` + tc.payload + `}}}`
				req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = fmt.Sprintf("[2001:db8:%x::1]:1000", clientSeq.Add(1))
				if signature != "unsigned" {
					if err := key.Sign(req, []byte(body)); err != nil {
						t.Fatal(err)
					}
				}
				if signature == "tampered" {
					req.Header.Set("Signature", "invalid-signature")
				}
				rr := httptest.NewRecorder()
				handlerFor(sv).ServeHTTP(rr, req)
				want, code := http.StatusUnauthorized, reqres.APIErrorUnauthenticated
				if signature == "valid" {
					want, code = http.StatusNotImplemented, reqres.APIErrorUnimplemented
				}
				checkResponse(t, rr, want, code)
			})
		}
	}
}

func TestNotificationDiscoveryDoesNotLogPeerResponse(t *testing.T) {
	const marker = "untrusted-discovery-marker"
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(marker))
	}))
	t.Cleanup(peer.Close)
	stampGateway(&notificationsMockGW{})
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc": "localhost:0", "http_sig_mode": "strict",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sv := s.(*svc)
	sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: true})
	body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + peer.Listener.Addr().String() + `","resourceType":"file","notification":{"file":{"providerId":"42"}}}`
	req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	var logs bytes.Buffer
	logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
	req = req.WithContext(appctx.WithLogger(req.Context(), &logger))
	rr := httptest.NewRecorder()
	handlerFor(sv).ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rr.Code)
	}
	if strings.Contains(logs.String(), marker) || strings.Contains(rr.Body.String(), marker) {
		t.Fatal("discovery response appeared in the notification logs or response")
	}
}

func TestNotificationDiscoveryUsesNetworkPolicy(t *testing.T) {
	key := newSigningKey(t)
	peer := signingServer(t, key)
	t.Cleanup(peer.Close)
	stampGateway(&notificationsMockGW{})
	s, err := New(context.Background(), map[string]any{
		"gatewaysvc": "localhost:0", "http_sig_mode": "strict",
		"token_managers": map[string]any{"jwt": map[string]any{"secret": "test-only-jwt-secret"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sv := s.(*svc)
	body := `{"notificationType":"SHARE_UNSHARED","senderDomain":"` + peer.Listener.Addr().String() + `","resourceType":"file","notification":{"file":{"providerId":"42"}}}`
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowLoopback=%t", allow), func(t *testing.T) {
			sv.notifications.ocmClient = NewPublicOnlyClientWithConfig(client.TransportConfig{AllowLoopback: allow})
			req := httptest.NewRequest(http.MethodPost, mount+notificationsPath, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = fmt.Sprintf("[2001:db8:%x::1]:1000", clientSeq.Add(1))
			if err := key.Sign(req, []byte(body)); err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			handlerFor(sv).ServeHTTP(rr, req)
			want := http.StatusInternalServerError
			if allow {
				want = http.StatusCreated
			}
			if rr.Code != want {
				t.Fatalf("status %d, want %d: %s", rr.Code, want, rr.Body.String())
			}
		})
	}
}
