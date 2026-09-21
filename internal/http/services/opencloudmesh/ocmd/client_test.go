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

package ocmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
)

var proxyEnvKeys = []string{
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

func TestMain(m *testing.M) {
	for _, key := range proxyEnvKeys {
		_ = os.Unsetenv(key)
	}
	os.Exit(m.Run())
}

func TestExchangeTokenSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "jwt-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tok, exp, err := c.ExchangeToken(context.Background(), srv.URL, "code123", "client1")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "jwt-tok" {
		t.Errorf("access_token: got %q, want jwt-tok", tok)
	}
	if exp != 3600 {
		t.Errorf("expires_in: got %d, want 3600", exp)
	}
}

func TestExchangeTokenInvalidGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "bad-code", "")
	if err == nil {
		t.Fatal("expected error for invalid_grant")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func TestExchangeTokenUnsupportedGrantType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for unsupported_grant_type")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if _, ok := err.(errtypes.PermissionDenied); !ok {
		t.Errorf("expected PermissionDenied, got %T: %v", err, err)
	}
}

func TestExchangeTokenUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if _, ok := err.(errtypes.PermissionDenied); !ok {
		t.Errorf("expected PermissionDenied, got %T: %v", err, err)
	}
}

func TestExchangeTokenServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenMissingAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type": "Bearer",
			"expires_in": 3600,
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}

func TestCompatibilityConstructorsTimeoutAndInsecure(t *testing.T) {
	tests := []struct {
		name     string
		ocm      *OCMClient
		timeout  time.Duration
		insecure bool
	}{
		{
			name:     "NewClient",
			ocm:      NewClient(7*time.Second, true),
			timeout:  7 * time.Second,
			insecure: true,
		},
		{
			name:     "NewClient zero timeout",
			ocm:      NewClient(0, false),
			timeout:  10 * time.Second,
			insecure: false,
		},
		{
			name:     "NewPublicOnlyClient",
			ocm:      NewPublicOnlyClient(3*time.Second, true),
			timeout:  3 * time.Second,
			insecure: true,
		},
		{
			name: "NewClientWithConfig",
			ocm: NewClientWithConfig(client.TransportConfig{
				Timeout:  4 * time.Second,
				Insecure: true,
			}),
			timeout:  4 * time.Second,
			insecure: true,
		},
		{
			name: "NewPublicOnlyClientWithConfig",
			ocm: NewPublicOnlyClientWithConfig(client.TransportConfig{
				Timeout:  2 * time.Second,
				Insecure: false,
			}),
			timeout:  2 * time.Second,
			insecure: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ocm.client.Timeout != tt.timeout {
				t.Errorf("timeout = %v, want %v", tt.ocm.client.Timeout, tt.timeout)
			}
			tr, ok := tt.ocm.client.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("transport: got %T, want *http.Transport", tt.ocm.client.Transport)
			}
			if tr.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if tr.TLSClientConfig.InsecureSkipVerify != tt.insecure {
				t.Errorf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, tt.insecure)
			}
		})
	}
}

func TestClientWithConfigPolicy(t *testing.T) {
	trusted := NewClientWithConfig(client.TransportConfig{
		Timeout:  5 * time.Second,
		Insecure: true,
	})
	tr, ok := trusted.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport", trusted.client.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("NewClientWithConfig must remain trusted and keep a proxy callback")
	}

	pub := NewPublicOnlyClientWithConfig(client.TransportConfig{
		Timeout:  5 * time.Second,
		Insecure: true,
	})
	ptr, ok := pub.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("public-only transport: got %T, want *http.Transport", pub.client.Transport)
	}
	if ptr.Proxy != nil {
		t.Fatal("NewPublicOnlyClientWithConfig must be public-only and must not use a proxy")
	}
}

// only the public-only client refuses internal targets; the plain one still reaches them.
func TestPublicOnlyClientRefusesInternalHosts(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"enabled":true,"apiVersion":"1.1"}`))
	}))
	defer srv.Close()

	cfg := client.TransportConfig{Timeout: 5 * time.Second, Insecure: true}
	tests := []struct {
		name    string
		client  *OCMClient
		wantErr bool
	}{
		{name: "public-only client refuses the loopback target", client: NewPublicOnlyClient(5*time.Second, true), wantErr: true},
		{name: "plain client still reaches it", client: NewClient(5*time.Second, true), wantErr: false},
		{name: "NewPublicOnlyClientWithConfig refuses the loopback target", client: NewPublicOnlyClientWithConfig(cfg), wantErr: true},
		{name: "NewClientWithConfig still reaches it", client: NewClientWithConfig(cfg), wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.client.Discover(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("Discover() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type hideLenReader struct{ io.Reader }

type bodyCloseTracker struct {
	mu     sync.Mutex
	opened int
	closed int
}

type trackingRoundTripper struct {
	base    http.RoundTripper
	tracker *bodyCloseTracker
}

func (t *trackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.tracker.mu.Lock()
	t.tracker.opened++
	t.tracker.mu.Unlock()
	resp.Body = &trackedBody{ReadCloser: resp.Body, tracker: t.tracker}
	return resp, nil
}

type trackedBody struct {
	io.ReadCloser
	tracker *bodyCloseTracker
	once    sync.Once
}

func (b *trackedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		b.tracker.mu.Lock()
		b.tracker.closed++
		b.tracker.mu.Unlock()
	})
	return err
}

func trackClientBodies(c *OCMClient) *bodyCloseTracker {
	tr := &bodyCloseTracker{}
	c.client.Transport = &trackingRoundTripper{
		base:    c.client.Transport,
		tracker: tr,
	}
	return tr
}

func (t *bodyCloseTracker) assertClosed(tt *testing.T) {
	tt.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.opened == 0 {
		tt.Fatal("expected at least one response body")
	}
	if t.closed != t.opened {
		tt.Fatalf("response bodies opened=%d closed=%d", t.opened, t.closed)
	}
}

func testShareRequest() *NewShareRequest {
	return &NewShareRequest{
		ShareWith:    "user@remote.example",
		Name:         "notes.txt",
		ProviderID:   "provider-1",
		Owner:        "owner@example",
		Sender:       "sender@example",
		ShareType:    "user",
		ResourceType: "file",
		Protocols: Protocols{
			&WebDAV{
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				URI:          "https://example.com/dav",
			},
		},
	}
}

func testInviteRequest() *InviteAcceptedRequest {
	return &InviteAcceptedRequest{
		UserID: "alice",
		Token:  "invite-token",
		Name:   "Alice",
		Email:  "alice@example.com",
	}
}

func padJSON(t *testing.T, raw []byte, n int) []byte {
	t.Helper()
	if len(raw) > n {
		t.Fatalf("fixture length %d exceeds target %d", len(raw), n)
	}
	out := bytes.Repeat([]byte(" "), n)
	copy(out, raw)
	return out
}

func oversizedMarker(n int) []byte {
	marker := []byte("OVERSIZED-OCM-BODY-")
	out := bytes.Repeat(marker, (n/len(marker))+1)
	return out[:n]
}

func writeChunkedBody(t *testing.T, w http.ResponseWriter, status int, body []byte) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("httptest ResponseWriter is not a Flusher")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(body) == 0 {
		flusher.Flush()
		return
	}
	_, _ = w.Write(body[:1])
	flusher.Flush()
	if len(body) > 1 {
		_, _ = w.Write(body[1:])
	}
}

func callOCMMethod(c *OCMClient, method, base string) error {
	ctx := context.Background()
	switch method {
	case "discover":
		_, err := c.Discover(ctx, base)
		return err
	case "directory":
		_, err := c.GetDirectoryService(ctx, base)
		return err
	case "newshare":
		_, err := c.NewShare(ctx, base, testShareRequest())
		return err
	case "invite":
		_, err := c.InviteAccepted(ctx, base, testInviteRequest())
		return err
	case "token":
		_, _, err := c.ExchangeToken(ctx, base, "code", "client1")
		return err
	default:
		return errors.New("unknown OCM method " + method)
	}
}

func TestReadOCMBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		limit   int64
		want    string
		wantErr error
	}{
		{name: "below cap", body: "hello", limit: 8, want: "hello"},
		{name: "exactly at limit", body: "12345678", limit: 8, want: "12345678"},
		{name: "limit plus one", body: "123456789", limit: 8, wantErr: ErrResponseTooLarge},
		{name: "empty body", body: "", limit: 8, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := readOCMBody(hideLenReader{strings.NewReader(tt.body)}, tt.limit)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Fatalf("data = %q, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("data = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeOCMJSON(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []struct {
		name         string
		body         string
		limit        int64
		want         payload
		wantTooLarge bool
		wantDecode   bool
	}{
		{
			name:  "valid json below cap",
			body:  `{"name":"ada","age":1}`,
			limit: 64,
			want:  payload{Name: "ada", Age: 1},
		},
		{
			name:  "exactly at limit",
			body:  `{"name":"ada"}` + strings.Repeat(" ", 64-len(`{"name":"ada"}`)),
			limit: 64,
			want:  payload{Name: "ada"},
		},
		{
			name:  "unknown fields remain accepted",
			body:  `{"name":"ada","age":1,"extra":true}`,
			limit: 64,
			want:  payload{Name: "ada", Age: 1},
		},
		{
			name:       "malformed json",
			body:       `{not json`,
			limit:      64,
			wantDecode: true,
		},
		{
			name:         "oversized malformed wins over decode",
			body:         strings.Repeat("x", 9),
			limit:        8,
			wantTooLarge: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got payload
			err := decodeOCMJSON(hideLenReader{strings.NewReader(tt.body)}, tt.limit, &got)
			switch {
			case tt.wantTooLarge:
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("error = %v, want ErrResponseTooLarge", err)
				}
			case tt.wantDecode:
				if err == nil {
					t.Fatal("expected decode error")
				}
				if errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("got ErrResponseTooLarge, want decode error: %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want {
					t.Fatalf("decoded %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestOCMClientAcceptsValidControlPlaneJSON(t *testing.T) {
	t.Parallel()

	discoveryJSON := []byte(`{"enabled":true,"apiVersion":"1.1"}`)
	directoryJSON := []byte(`{"federation":"https://fed.example","servers":[]}`)
	shareJSON := []byte(`{"recipientDisplayName":"Ada"}`)
	inviteJSON := []byte(`{"userID":"ada","email":"ada@example.com","name":"Ada"}`)
	tokenJSON := []byte(`{"access_token":"jwt-tok","token_type":"Bearer","expires_in":3600}`)

	tests := []struct {
		name   string
		status int
		body   []byte
		check  func(*testing.T, *OCMClient, string)
	}{
		{
			name:   "discovery below cap",
			status: http.StatusOK,
			body:   discoveryJSON,
			check: func(t *testing.T, c *OCMClient, base string) {
				disco, err := c.Discover(context.Background(), base)
				if err != nil {
					t.Fatal(err)
				}
				if !disco.Enabled || disco.APIVersion != "1.1" {
					t.Fatalf("discovery = %+v", disco)
				}
			},
		},
		{
			name:   "directory below cap",
			status: http.StatusOK,
			body:   directoryJSON,
			check: func(t *testing.T, c *OCMClient, base string) {
				dir, err := c.GetDirectoryService(context.Background(), base)
				if err != nil {
					t.Fatal(err)
				}
				if dir.Federation != "https://fed.example" {
					t.Fatalf("federation = %q", dir.Federation)
				}
			},
		},
		{
			name:   "NewShare success",
			status: http.StatusOK,
			body:   shareJSON,
			check: func(t *testing.T, c *OCMClient, base string) {
				res, err := c.NewShare(context.Background(), base, testShareRequest())
				if err != nil {
					t.Fatal(err)
				}
				if res.RecipientDisplayName != "Ada" {
					t.Fatalf("recipient = %q", res.RecipientDisplayName)
				}
			},
		},
		{
			name:   "InviteAccepted success",
			status: http.StatusOK,
			body:   inviteJSON,
			check: func(t *testing.T, c *OCMClient, base string) {
				u, err := c.InviteAccepted(context.Background(), base, testInviteRequest())
				if err != nil {
					t.Fatal(err)
				}
				if u.UserID != "ada" || u.Name != "Ada" {
					t.Fatalf("remote user = %+v", u)
				}
			},
		},
		{
			name:   "ExchangeToken success",
			status: http.StatusOK,
			body:   tokenJSON,
			check: func(t *testing.T, c *OCMClient, base string) {
				tok, exp, err := c.ExchangeToken(context.Background(), base, "code", "client1")
				if err != nil {
					t.Fatal(err)
				}
				if tok != "jwt-tok" || exp != 3600 {
					t.Fatalf("token=%q exp=%d", tok, exp)
				}
			},
		},
		{
			name:   "discovery exactly at limit",
			status: http.StatusOK,
			body:   padJSON(t, discoveryJSON, int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				if _, err := c.Discover(context.Background(), base); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "directory exactly at limit",
			status: http.StatusOK,
			body:   padJSON(t, directoryJSON, int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				if _, err := c.GetDirectoryService(context.Background(), base); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "NewShare exactly at limit",
			status: http.StatusCreated,
			body:   padJSON(t, shareJSON, int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				if _, err := c.NewShare(context.Background(), base, testShareRequest()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "InviteAccepted exactly at limit",
			status: http.StatusOK,
			body:   padJSON(t, inviteJSON, int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				if _, err := c.InviteAccepted(context.Background(), base, testInviteRequest()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "ExchangeToken exactly at limit",
			status: http.StatusOK,
			body:   padJSON(t, tokenJSON, int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				if _, _, err := c.ExchangeToken(context.Background(), base, "code", "client1"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "ExchangeToken invalid_grant exactly at limit",
			status: http.StatusBadRequest,
			body:   padJSON(t, []byte(`{"error":"invalid_grant"}`), int(DefaultResponseLimit)),
			check: func(t *testing.T, c *OCMClient, base string) {
				_, _, err := c.ExchangeToken(context.Background(), base, "code", "client1")
				if err == nil {
					t.Fatal("expected error for invalid_grant")
				}
				if errors.Is(err, ErrResponseTooLarge) {
					t.Fatal("exactly-at-limit invalid_grant was rejected as oversized")
				}
				if _, ok := err.(errtypes.InvalidCredentials); !ok {
					t.Fatalf("expected InvalidCredentials, got %T: %v", err, err)
				}
			},
		},
		{
			name:   "discovery unknown fields remain accepted",
			status: http.StatusOK,
			body:   []byte(`{"enabled":true,"apiVersion":"1.1","extraField":"x"}`),
			check: func(t *testing.T, c *OCMClient, base string) {
				disco, err := c.Discover(context.Background(), base)
				if err != nil {
					t.Fatal(err)
				}
				if !disco.Enabled {
					t.Fatal("expected enabled discovery")
				}
			},
		},
		{
			name:   "token unknown fields remain accepted",
			status: http.StatusOK,
			body:   []byte(`{"access_token":"jwt-tok","expires_in":9,"surprise":true}`),
			check: func(t *testing.T, c *OCMClient, base string) {
				tok, exp, err := c.ExchangeToken(context.Background(), base, "code", "client1")
				if err != nil {
					t.Fatal(err)
				}
				if tok != "jwt-tok" || exp != 9 {
					t.Fatalf("token=%q exp=%d", tok, exp)
				}
			},
		},
		{
			name:   "NewShare unknown fields remain accepted",
			status: http.StatusOK,
			body:   []byte(`{"recipientDisplayName":"Ada","extra":true}`),
			check: func(t *testing.T, c *OCMClient, base string) {
				res, err := c.NewShare(context.Background(), base, testShareRequest())
				if err != nil {
					t.Fatal(err)
				}
				if res.RecipientDisplayName != "Ada" {
					t.Fatalf("recipient = %q", res.RecipientDisplayName)
				}
			},
		},
		{
			name:   "InviteAccepted unknown fields remain accepted",
			status: http.StatusOK,
			body:   []byte(`{"userID":"ada","email":"ada@example.com","name":"Ada","extra":1}`),
			check: func(t *testing.T, c *OCMClient, base string) {
				u, err := c.InviteAccepted(context.Background(), base, testInviteRequest())
				if err != nil {
					t.Fatal(err)
				}
				if u.UserID != "ada" {
					t.Fatalf("remote user = %+v", u)
				}
			},
		},
		{
			name:   "directory unknown fields remain accepted",
			status: http.StatusOK,
			body:   []byte(`{"federation":"https://fed.example","servers":[],"extra":{}}`),
			check: func(t *testing.T, c *OCMClient, base string) {
				dir, err := c.GetDirectoryService(context.Background(), base)
				if err != nil {
					t.Fatal(err)
				}
				if dir.Federation != "https://fed.example" {
					t.Fatalf("federation = %q", dir.Federation)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer srv.Close()

			c := NewClient(10*time.Second, true)
			tracker := trackClientBodies(c)
			tt.check(t, c, srv.URL)
			tracker.assertClosed(t)
		})
	}
}

func TestOCMClientRejectsOversizedAndMalformedBodies(t *testing.T) {
	t.Parallel()

	over := oversizedMarker(int(DefaultResponseLimit) + 1)
	overJSON := padJSON(t, []byte(`{"enabled":true,"apiVersion":"1.1"}`), int(DefaultResponseLimit)+1)
	overShare := padJSON(t, []byte(`{"recipientDisplayName":"Ada"}`), int(DefaultResponseLimit)+1)
	overInvite := padJSON(t, []byte(`{"userID":"ada"}`), int(DefaultResponseLimit)+1)
	overToken := padJSON(t, []byte(`{"access_token":"jwt-tok","expires_in":1}`), int(DefaultResponseLimit)+1)
	overGrant := padJSON(t, []byte(`{"error":"invalid_grant"}`), int(DefaultResponseLimit)+1)
	overDir := padJSON(t, []byte(`{"federation":"https://fed.example","servers":[]}`), int(DefaultResponseLimit)+1)

	tests := []struct {
		name     string
		method   string
		status   int
		body     []byte
		chunked  bool
		wantKind string
	}{
		{name: "directory limit plus one", method: "directory", status: http.StatusOK, body: overDir, wantKind: "too-large"},
		{name: "NewShare limit plus one", method: "newshare", status: http.StatusOK, body: overShare, wantKind: "too-large"},
		{name: "InviteAccepted limit plus one", method: "invite", status: http.StatusOK, body: overInvite, wantKind: "too-large"},
		{name: "ExchangeToken limit plus one", method: "token", status: http.StatusOK, body: overToken, wantKind: "too-large"},
		{name: "chunked oversized discovery is rejected after fallback", method: "discover", status: http.StatusOK, body: overJSON, chunked: true, wantKind: "internal"},
		{name: "chunked oversized NewShare", method: "newshare", status: http.StatusOK, body: overShare, chunked: true, wantKind: "too-large"},
		{name: "chunked oversized directory", method: "directory", status: http.StatusOK, body: overDir, chunked: true, wantKind: "too-large"},
		{name: "malformed discovery", method: "discover", status: http.StatusOK, body: []byte("not json"), wantKind: "internal"},
		{name: "malformed directory", method: "directory", status: http.StatusOK, body: []byte("{bad"), wantKind: "decode"},
		{name: "malformed NewShare", method: "newshare", status: http.StatusOK, body: []byte("{bad"), wantKind: "decode"},
		{name: "malformed InviteAccepted", method: "invite", status: http.StatusOK, body: []byte("{bad"), wantKind: "decode"},
		{name: "malformed ExchangeToken", method: "token", status: http.StatusOK, body: []byte("not json"), wantKind: "decode"},
		{name: "oversized NewShare error body", method: "newshare", status: http.StatusInternalServerError, body: over, wantKind: "too-large"},
		{name: "oversized InviteAccepted error body", method: "invite", status: http.StatusInternalServerError, body: over, wantKind: "too-large"},
		{name: "oversized unknown-status directory", method: "directory", status: http.StatusInternalServerError, body: over, wantKind: "too-large"},
		{name: "oversized token-default", method: "token", status: http.StatusInternalServerError, body: over, wantKind: "too-large"},
		{name: "oversized token-error body", method: "token", status: http.StatusBadRequest, body: overGrant, wantKind: "too-large"},
		{name: "oversized malformed NewShare", method: "newshare", status: http.StatusOK, body: over, wantKind: "too-large"},
		{name: "oversized malformed token", method: "token", status: http.StatusOK, body: over, wantKind: "too-large"},
		{name: "oversized malformed token-error", method: "token", status: http.StatusBadRequest, body: over, wantKind: "too-large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.chunked {
					writeChunkedBody(t, w, tt.status, tt.body)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer srv.Close()

			c := NewClient(10*time.Second, true)
			tracker := trackClientBodies(c)
			err := callOCMMethod(c, tt.method, srv.URL)
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, io.EOF) {
				t.Fatal("decoded the body more than once")
			}
			switch tt.wantKind {
			case "too-large":
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("error = %v (%T), want ErrResponseTooLarge", err, err)
				}
				if strings.Contains(err.Error(), "OVERSIZED-OCM-BODY-") {
					t.Fatalf("oversized body was truncated into the error string: %v", err)
				}
			case "decode":
				if errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("got ErrResponseTooLarge, want decode error: %v", err)
				}
			case "internal":
				if _, ok := err.(errtypes.InternalError); !ok {
					t.Fatalf("error = %T %v, want InternalError", err, err)
				}
			default:
				t.Fatalf("unknown wantKind %q", tt.wantKind)
			}
			tracker.assertClosed(t)
		})
	}
}

func TestDiscoverFallsBackToLegacyEndpoint(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/ocm-provider", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"apiVersion":"1.1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tracker := trackClientBodies(c)
	disco, err := c.Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !disco.Enabled || disco.APIVersion != "1.1" {
		t.Fatalf("discovery = %+v", disco)
	}
	tracker.assertClosed(t)
}

func TestDiscoverFallsBackToLegacyAfterOversizedWellKnown(t *testing.T) {
	t.Parallel()

	overJSON := padJSON(t, []byte(`{"enabled":true,"apiVersion":"well-known"}`), int(DefaultResponseLimit)+1)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(overJSON)
	})
	mux.HandleFunc("/ocm-provider", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enabled":true,"apiVersion":"1.1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tracker := trackClientBodies(c)
	disco, err := c.Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !disco.Enabled || disco.APIVersion != "1.1" {
		t.Fatalf("discovery = %+v", disco)
	}
	tracker.assertClosed(t)
}

func TestOCMClientClosesResponseBodiesOnMappedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		status int
		body   []byte
	}{
		{name: "NewShare 400", method: "newshare", status: http.StatusBadRequest, body: []byte(`{"error":"bad"}`)},
		{name: "NewShare 403", method: "newshare", status: http.StatusForbidden, body: []byte(`denied`)},
		{name: "InviteAccepted 400", method: "invite", status: http.StatusBadRequest, body: []byte(`{"error":"bad"}`)},
		{name: "InviteAccepted 409", method: "invite", status: http.StatusConflict, body: []byte(`{"error":"conflict"}`)},
		{name: "InviteAccepted 403", method: "invite", status: http.StatusForbidden, body: []byte(`denied`)},
		{name: "ExchangeToken 401", method: "token", status: http.StatusUnauthorized, body: []byte(`denied`)},
		{name: "ExchangeToken 403", method: "token", status: http.StatusForbidden, body: []byte(`denied`)},
		{name: "ExchangeToken 400 invalid_grant", method: "token", status: http.StatusBadRequest, body: []byte(`{"error":"invalid_grant"}`)},
		{name: "ExchangeToken 500", method: "token", status: http.StatusInternalServerError, body: []byte(`oops`)},
		{name: "directory 500", method: "directory", status: http.StatusInternalServerError, body: []byte(`oops`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer srv.Close()

			c := NewClient(10*time.Second, true)
			tracker := trackClientBodies(c)
			if err := callOCMMethod(c, tt.method, srv.URL); err == nil {
				t.Fatal("expected error")
			}
			tracker.assertClosed(t)
		})
	}
}

func TestNewShareAndInviteAcceptedDecodeOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		body   []byte
		invoke func(*testing.T, *OCMClient, string)
	}{
		{
			name: "NewShare",
			path: "/shares",
			body: []byte(`{"recipientDisplayName":"Ada"}`),
			invoke: func(t *testing.T, c *OCMClient, base string) {
				res, err := c.NewShare(context.Background(), base, testShareRequest())
				if errors.Is(err, io.EOF) {
					t.Fatal("NewShare decoded the body more than once")
				}
				if err != nil {
					t.Fatal(err)
				}
				if res == nil || res.RecipientDisplayName != "Ada" {
					t.Fatalf("share response = %+v", res)
				}
			},
		},
		{
			name: "InviteAccepted",
			path: "/invite-accepted",
			body: []byte(`{"userID":"ada","email":"ada@example.com","name":"Ada"}`),
			invoke: func(t *testing.T, c *OCMClient, base string) {
				u, err := c.InviteAccepted(context.Background(), base, testInviteRequest())
				if errors.Is(err, io.EOF) {
					t.Fatal("InviteAccepted decoded the body more than once")
				}
				if err != nil {
					t.Fatal(err)
				}
				if u == nil || u.UserID != "ada" {
					t.Fatalf("invite response = %+v", u)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tt.path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tt.body)
			}))
			defer srv.Close()

			c := NewClient(10*time.Second, true)
			tracker := trackClientBodies(c)
			tt.invoke(t, c, srv.URL)
			tracker.assertClosed(t)
		})
	}
}
