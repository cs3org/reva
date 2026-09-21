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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
