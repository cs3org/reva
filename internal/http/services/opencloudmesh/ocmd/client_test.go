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
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/errtypes"
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

// roundTripperFunc adapts a plain function to http.RoundTripper, used to make
// http.DefaultTransport a non-*http.Transport for the fallback test.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The trusted transport must keep http.ProxyFromEnvironment: some deployments
// only reach their peers through a corporate proxy.
func TestNewOCMTransportUsesProxyFromEnvironment(t *testing.T) {
	tr := newOCMTransport(false)
	if tr.Proxy == nil {
		t.Fatal("transport Proxy must not be nil")
	}
	got := reflect.ValueOf(tr.Proxy).Pointer()
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	if got != want {
		t.Error("transport Proxy must be http.ProxyFromEnvironment")
	}
}

// The public-only transport must not proxy, or the dial Control would see the
// proxy address instead of the target.
func TestNewPublicOnlyClientTransportProxyNil(t *testing.T) {
	c := NewPublicOnlyClient(5*time.Second, true)
	tr := publicOnlyHTTPTransport(t, c)
	if tr.Proxy != nil {
		t.Error("public-only client must not use a proxy")
	}
}

// TestNewOCMTransportInsecureSkipVerify checks the TLS contract is preserved.
func TestNewOCMTransportInsecureSkipVerify(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		tr := newOCMTransport(insecure)
		if tr.TLSClientConfig == nil {
			t.Fatalf("insecure=%v: TLSClientConfig is nil", insecure)
		}
		if tr.TLSClientConfig.InsecureSkipVerify != insecure {
			t.Errorf("insecure=%v: InsecureSkipVerify = %v, want %v", insecure, tr.TLSClientConfig.InsecureSkipVerify, insecure)
		}
	}
}

// TestNewOCMTransportFallback covers the branch where http.DefaultTransport is
// not a *http.Transport, so the helper builds the transport directly.
func TestNewOCMTransportFallback(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})

	tr := newOCMTransport(true)
	if tr.Proxy == nil {
		t.Fatal("fallback transport Proxy must not be nil")
	}
	if got, want := reflect.ValueOf(tr.Proxy).Pointer(), reflect.ValueOf(http.ProxyFromEnvironment).Pointer(); got != want {
		t.Error("fallback transport Proxy must be http.ProxyFromEnvironment")
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("fallback transport must set InsecureSkipVerify=true")
	}
}

// TestNewClientUsesOCMTransport confirms the public constructor wires the
// proxy-aware transport and request timeout into the HTTP client.
func TestNewClientUsesOCMTransport(t *testing.T) {
	c := NewClient(7*time.Second, true)
	if c.client.Timeout != 7*time.Second {
		t.Errorf("client timeout: got %v, want %v", c.client.Timeout, 7*time.Second)
	}
	tr, ok := c.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport: got %T, want *http.Transport", c.client.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("client transport Proxy must not be nil")
	}
}

// Both constructors must set a TLS 1.2 floor on the outbound transport.
func TestOCMClientTransportsRequireTLS12(t *testing.T) {
	tests := []struct {
		name   string
		client *OCMClient
	}{
		{name: "NewClient", client: NewClient(5*time.Second, false)},
		{name: "NewPublicOnlyClient", client: NewPublicOnlyClient(5*time.Second, false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := publicOnlyHTTPTransport(t, tt.client)
			if tr.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
				t.Errorf("MinVersion = %#x, want at least TLS 1.2 (%#x)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
			}
		})
	}
}

func TestIsPublicIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "public v4", ip: "93.184.216.34", want: true},
		{name: "public v6", ip: "2606:2800:220:1:248:1893:25c8:1946", want: true},
		{name: "loopback", ip: "127.0.0.1", want: false},
		{name: "loopback v6", ip: "::1", want: false},
		{name: "private 10/8", ip: "10.1.2.3", want: false},
		{name: "private 172.16/12", ip: "172.16.5.4", want: false},
		{name: "private 192.168/16", ip: "192.168.1.1", want: false},
		{name: "cloud metadata service", ip: "169.254.169.254", want: false},
		{name: "unspecified", ip: "0.0.0.0", want: false},
		{name: "multicast", ip: "224.0.0.1", want: false},
		{name: "unique local v6", ip: "fd00::1", want: false},
		{name: "link local v6", ip: "fe80::1", want: false},
		{name: "carrier-grade nat", ip: "100.64.0.1", want: false},
		{name: "just outside carrier-grade nat", ip: "100.128.0.1", want: true},
		{name: "ipv4-mapped metadata service", ip: "::ffff:169.254.169.254", want: false},
		{name: "ipv4-mapped loopback", ip: "::ffff:127.0.0.1", want: false},
		{name: "nat64 metadata service", ip: "64:ff9b::a9fe:a9fe", want: false},
		{name: "nat64 loopback", ip: "64:ff9b::7f00:1", want: false},
		{name: "nat64 public address", ip: "64:ff9b::5db8:d822", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("could not parse %q", tt.ip)
			}
			if got := isPublicIP(ip); got != tt.want {
				t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestRefuseNonPublicAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "public host", address: "93.184.216.34:443"},
		{name: "loopback", address: "127.0.0.1:8080", wantErr: true},
		{name: "metadata service", address: "169.254.169.254:80", wantErr: true},
		{name: "private range", address: "10.0.0.5:9000", wantErr: true},
		{name: "ipv6 loopback", address: "[::1]:8080", wantErr: true},
		{name: "no port", address: "93.184.216.34", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseNonPublicAddr("tcp", tt.address, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("refuseNonPublicAddr(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
			}
		})
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

	tests := []struct {
		name    string
		client  *OCMClient
		wantErr bool
	}{
		{name: "public-only client refuses the loopback target", client: NewPublicOnlyClient(5*time.Second, true), wantErr: true},
		{name: "plain client still reaches it", client: NewClient(5*time.Second, true), wantErr: false},
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

// 2 MiB is above the 1 MiB OCM response cap; using a fixed oversize keeps the
// tests valid before the named constant exists and after it is added.
const testOversizedOCMBody = 2 << 20

func oversizedJSON(t *testing.T, extra map[string]any, largeField string) []byte {
	t.Helper()
	payload := map[string]any{}
	for k, v := range extra {
		payload[k] = v
	}
	payload[largeField] = strings.Repeat("a", testOversizedOCMBody)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func testNewShareRequest() *NewShareRequest {
	return &NewShareRequest{
		ShareWith:    "einstein@remote.example",
		Name:         "notes.txt",
		ProviderID:   "id-1",
		Owner:        "marie@local.example",
		Sender:       "marie@local.example",
		ShareType:    "user",
		ResourceType: "file",
		Protocols: Protocols{
			&WebDAV{
				SharedSecret: "secret",
				Permissions:  []string{"read"},
				URI:          "https://local.example/dav/notes.txt",
			},
		},
	}
}

func assertOCMResponseTooLarge(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected oversized OCM response to be rejected")
	}
	if !errors.Is(err, errOCMResponseTooLarge) {
		t.Fatalf("got error %q, want errOCMResponseTooLarge", err)
	}
}

func TestDiscoverNormalSizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":    true,
			"apiVersion": "1.1",
			"provider":   "reva",
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	disco, err := c.Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if disco == nil || !disco.Enabled || disco.APIVersion != "1.1" {
		t.Fatalf("unexpected discovery payload: %+v", disco)
	}
}

func TestDiscoverOversizedBody(t *testing.T) {
	body := oversizedJSON(t, map[string]any{
		"enabled":    true,
		"apiVersion": "1.1",
	}, "provider")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	disco, err := c.Discover(context.Background(), srv.URL)
	assertOCMResponseTooLarge(t, err)
	if disco != nil {
		t.Fatalf("oversized discovery body must not be decoded, got %+v", disco)
	}
}

func TestNewShareNormalSizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"recipientDisplayName": "Alice",
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	resp, err := c.NewShare(context.Background(), srv.URL, testNewShareRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.RecipientDisplayName != "Alice" {
		t.Fatalf("unexpected share response: %+v", resp)
	}
}

func TestNewShareOversizedBody(t *testing.T) {
	body := oversizedJSON(t, nil, "recipientDisplayName")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	resp, err := c.NewShare(context.Background(), srv.URL, testNewShareRequest())
	assertOCMResponseTooLarge(t, err)
	if resp != nil {
		t.Fatalf("oversized share body must not be decoded, got %+v", resp)
	}
}

func TestNewShareOversizedTrailingBody(t *testing.T) {
	body := []byte(`{"recipientDisplayName":"Alice"}` + strings.Repeat(" ", testOversizedOCMBody))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	resp, err := c.NewShare(context.Background(), srv.URL, testNewShareRequest())
	assertOCMResponseTooLarge(t, err)
	if resp != nil {
		t.Fatalf("oversized trailing body must not return a decoded share, got %+v", resp)
	}
}

func TestExchangeTokenOversizedBody(t *testing.T) {
	body := oversizedJSON(t, map[string]any{"expires_in": 3600}, "access_token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tok, exp, err := c.ExchangeToken(context.Background(), srv.URL, "code123", "client1")
	assertOCMResponseTooLarge(t, err)
	if tok != "" || exp != 0 {
		t.Fatalf("oversized token body must not be decoded, got token %q expires_in %d", tok, exp)
	}
}

func TestExchangeTokenHTTP400OversizedBody(t *testing.T) {
	body := oversizedJSON(t, map[string]any{"error": "invalid_grant"}, "error_description")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tok, exp, err := c.ExchangeToken(context.Background(), srv.URL, "code123", "client1")
	assertOCMResponseTooLarge(t, err)
	if tok != "" || exp != 0 {
		t.Fatalf("oversized HTTP 400 token body must not be decoded, got token %q expires_in %d", tok, exp)
	}
}

// httptest servers bind loopback, which the public-only dial guard refuses.
// Redirect and scheme tests keep the constructor's CheckRedirect, https gate,
// and Proxy=nil, and only drop Control so the TLS server is reachable.
func allowPublicOnlyLoopback(t *testing.T, c *OCMClient) {
	t.Helper()
	allowUntrustedLoopback(t, c.client.Transport)
}

func publicOnlyHTTPTransport(t *testing.T, c *OCMClient) *http.Transport {
	t.Helper()
	return innerUntrustedTransport(t, c.client.Transport)
}

func closeResponse(resp *http.Response) {
	if resp != nil {
		resp.Body.Close()
	}
}

func TestPublicOnlyClientRedirectCapAndHTTPSScheme(t *testing.T) {
	t.Run("https redirecting more than 3 times is refused", func(t *testing.T) {
		hops := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops++
			if hops <= 4 {
				http.Redirect(w, r, "/hop", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewPublicOnlyClient(5*time.Second, true)
		allowPublicOnlyLoopback(t, c)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected public-only client to refuse more than 3 redirects")
		}
		if !errors.Is(err, errPublicOnlyTooManyRedirects) {
			t.Fatalf("got %v, want errPublicOnlyTooManyRedirects", err)
		}
	})

	t.Run("http non-https initial URL is refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("public-only client must not fetch a non-https URL")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewPublicOnlyClient(5*time.Second, true)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected public-only client to refuse a non-https URL")
		}
		if !errors.Is(err, errPublicOnlyNonHTTPS) {
			t.Fatalf("got %v, want errPublicOnlyNonHTTPS", err)
		}
	})

	t.Run("redirect to http is refused at the hop", func(t *testing.T) {
		httpFetched := false
		httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpFetched = true
			w.WriteHeader(http.StatusOK)
		}))
		defer httpSrv.Close()

		tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, httpSrv.URL, http.StatusFound)
		}))
		defer tlsSrv.Close()

		c := NewPublicOnlyClient(5*time.Second, true)
		allowPublicOnlyLoopback(t, c)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tlsSrv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		closeResponse(resp)
		if httpFetched {
			t.Error("public-only client followed a redirect to http")
		}
		if err == nil {
			t.Fatal("expected public-only client to refuse an http redirect hop")
		}
		if !errors.Is(err, errPublicOnlyNonHTTPS) {
			t.Fatalf("got %v, want errPublicOnlyNonHTTPS", err)
		}
	})

	t.Run("https with at most 3 redirects succeeds", func(t *testing.T) {
		hops := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops++
			if hops <= 3 {
				http.Redirect(w, r, "/hop", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer srv.Close()

		c := NewPublicOnlyClient(5*time.Second, true)
		allowPublicOnlyLoopback(t, c)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			t.Fatalf("expected public-only client to follow <=3 https redirects: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})
}

func TestNewClientAllowsHTTPAndFollowsMoreThanThreeRedirects(t *testing.T) {
	hops := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops <= 4 {
			http.Redirect(w, r, "/hop", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient(5*time.Second, true)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("trusted client must still allow http and more than 3 redirects: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// allowPublicOnlyTestServer keeps the public-only HTTPS gate, redirect cap, and
// non-public dial refusal, and only permits the httptest listener so the first
// hop is reachable. Redirects to other addresses still hit refuseNonPublicAddr.
func allowPublicOnlyTestServer(t *testing.T, c *OCMClient, allowed string) {
	t.Helper()
	allowedHost, allowedPort, err := net.SplitHostPort(allowed)
	if err != nil {
		t.Fatal(err)
	}
	tr := publicOnlyHTTPTransport(t, c)
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host == allowedHost && port == allowedPort {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
		}
		if err := refuseNonPublicAddr(network, address, nil); err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
	}
}

func TestPublicOnlyDiscoverSucceedsOnAllowedHTTPSHost(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/ocm" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":       true,
			"apiVersion":    "1.2.0",
			"endPoint":      srv.URL + "/ocm",
			"provider":      "reva",
			"resourceTypes": []any{},
			"tokenEndPoint": srv.URL + "/ocm/token",
		})
	}))
	defer srv.Close()

	c := NewPublicOnlyClient(5*time.Second, true)
	allowPublicOnlyTestServer(t, c, srv.Listener.Addr().String())
	disco, err := c.Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Discover() on an allowed HTTPS host: %v", err)
	}
	if disco == nil || disco.TokenEndPoint != srv.URL+"/ocm/token" {
		t.Fatalf("unexpected discovery payload: %+v", disco)
	}
}

func TestPublicOnlyDiscoverRefusesRedirectToLinkLocal(t *testing.T) {
	var dialed []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://169.254.169.254/.well-known/ocm", http.StatusFound)
	}))
	defer srv.Close()

	c := NewPublicOnlyClient(5*time.Second, true)
	allowed := srv.Listener.Addr().String()
	allowedHost, allowedPort, err := net.SplitHostPort(allowed)
	if err != nil {
		t.Fatal(err)
	}
	tr := publicOnlyHTTPTransport(t, c)
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host == allowedHost && port == allowedPort {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
		}
		if err := refuseNonPublicAddr(network, address, nil); err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
	}

	_, err = c.Discover(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected Discover to refuse a redirect to a link-local address")
	}
	sawLinkLocal := false
	for _, address := range dialed {
		if strings.HasPrefix(address, "169.254.169.254:") {
			sawLinkLocal = true
			break
		}
	}
	if !sawLinkLocal {
		t.Fatalf("expected a redirect-hop dial to 169.254.169.254, dialed %v", dialed)
	}
}

func TestPublicOnlyExchangeTokenSucceedsOnAllowedHTTPSHost(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "jwt-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	c := NewPublicOnlyClient(5*time.Second, true)
	allowPublicOnlyTestServer(t, c, srv.Listener.Addr().String())
	tok, exp, err := c.ExchangeToken(context.Background(), srv.URL, "code123", "client1")
	if err != nil {
		t.Fatalf("ExchangeToken() on an allowed HTTPS host: %v", err)
	}
	if tok != "jwt-tok" || exp != 3600 {
		t.Fatalf("got token %q expires_in %d, want jwt-tok / 3600", tok, exp)
	}
}

func TestPublicOnlyClientStillRefusesHTTPSLoopback(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("public-only dial guard must not connect to loopback")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewPublicOnlyClient(5*time.Second, true)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Do(req)
	closeResponse(resp)
	if err == nil {
		t.Fatal("expected public-only dial guard to refuse https loopback")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("got %v, want the existing non-public dial error", err)
	}
}

func doUntrustedRequest(t *testing.T, rt http.RoundTripper, rawURL string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return (&http.Client{Transport: rt}).Do(req)
}

func innerUntrustedTransport(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	switch tr := rt.(type) {
	case *http.Transport:
		return tr
	case *publicOnlyTransport:
		if tr.Transport == nil {
			t.Fatal("untrusted transport is missing an inner *http.Transport")
		}
		return tr.Transport
	default:
		t.Fatalf("untrusted transport: got %T, want *http.Transport or *publicOnlyTransport", rt)
		return nil
	}
}

func allowUntrustedLoopback(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	tr := innerUntrustedTransport(t, rt)
	tr.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
}

func TestUntrustedHTTPTransportRefusesNonPublicHosts(t *testing.T) {
	t.Parallel()

	t.Run("https loopback is refused at dial", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("untrusted transport must not connect to loopback")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		resp, err := doUntrustedRequest(t, UntrustedHTTPTransport(5*time.Second, true), srv.URL)
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected untrusted transport to refuse https loopback at dial")
		}
		if !strings.Contains(err.Error(), "non-public") {
			t.Fatalf("got %v, want the existing non-public dial error", err)
		}
	})

	t.Run("cloud metadata host is refused at dial", func(t *testing.T) {
		resp, err := doUntrustedRequest(t, UntrustedHTTPTransport(5*time.Second, true), "https://169.254.169.254/")
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected untrusted transport to refuse the metadata host at dial")
		}
		if !strings.Contains(err.Error(), "non-public") {
			t.Fatalf("got %v, want the existing non-public dial error", err)
		}
	})
}

func TestUntrustedHTTPTransportRefusesHTTP(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("untrusted transport must not fetch a non-https URL")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := doUntrustedRequest(t, UntrustedHTTPTransport(5*time.Second, true), srv.URL)
	closeResponse(resp)
	if err == nil {
		t.Fatal("expected untrusted transport to refuse a non-https URL")
	}
	if !errors.Is(err, errPublicOnlyNonHTTPS) {
		t.Fatalf("got %v, want errPublicOnlyNonHTTPS", err)
	}
}

func TestUntrustedHTTPTransportRequiresTLS12(t *testing.T) {
	rt := UntrustedHTTPTransport(5*time.Second, false)
	tr := innerUntrustedTransport(t, rt)
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want at least TLS 1.2 (%#x)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure=false must not set InsecureSkipVerify")
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("TLS 1.1 handshake must not succeed")
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		MinVersion: tls.VersionTLS11,
		MaxVersion: tls.VersionTLS11,
	}
	srv.StartTLS()
	defer srv.Close()

	insecureRT := UntrustedHTTPTransport(5*time.Second, true)
	allowUntrustedLoopback(t, insecureRT)
	resp, err := doUntrustedRequest(t, insecureRT, srv.URL)
	closeResponse(resp)
	if err == nil {
		t.Fatal("expected untrusted transport to refuse a TLS 1.1 handshake")
	}
	errMsg := strings.ToLower(err.Error())
	mentionsTLS := strings.Contains(errMsg, "tls")
	mentionsProtocol := strings.Contains(errMsg, "protocol")
	mentionsHandshake := strings.Contains(errMsg, "handshake")
	if !mentionsTLS && !mentionsProtocol && !mentionsHandshake {
		t.Fatalf("got %v, want a TLS-version or handshake failure", err)
	}
}

func TestUntrustedHTTPTransportMirrorsInsecureFlag(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		tr := innerUntrustedTransport(t, UntrustedHTTPTransport(5*time.Second, insecure))
		if tr.TLSClientConfig == nil {
			t.Fatalf("insecure=%v: TLSClientConfig is nil", insecure)
		}
		if tr.TLSClientConfig.InsecureSkipVerify != insecure {
			t.Errorf("insecure=%v: InsecureSkipVerify = %v, want %v", insecure, tr.TLSClientConfig.InsecureSkipVerify, insecure)
		}
		if tr.Proxy != nil {
			t.Errorf("insecure=%v: untrusted transport must not use a proxy", insecure)
		}
	}
}

func TestUntrustedHTTPTransportFitsGowebdavSetTransport(t *testing.T) {
	var rt http.RoundTripper = UntrustedHTTPTransport(5*time.Second, false)
	if rt == nil {
		t.Fatal("UntrustedHTTPTransport returned nil")
	}
}

func TestPublicOnlyCheckRedirectCapsAtThree(t *testing.T) {
	hops := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops <= 4 {
			http.Redirect(w, r, "/hop", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := UntrustedHTTPTransport(5*time.Second, true)
	allowUntrustedLoopback(t, rt)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{
		Transport:     rt,
		CheckRedirect: PublicOnlyCheckRedirect,
	}).Do(req)
	closeResponse(resp)
	if err == nil {
		t.Fatal("expected PublicOnlyCheckRedirect to refuse more than 3 redirects")
	}
	if !errors.Is(err, errPublicOnlyTooManyRedirects) {
		t.Fatalf("got %v, want errPublicOnlyTooManyRedirects", err)
	}
}

func TestNewPublicOnlyClientKeepsPublicOnlyPolicy(t *testing.T) {
	c := NewPublicOnlyClient(5*time.Second, true)
	if c.client.Timeout != 5*time.Second {
		t.Errorf("client timeout: got %v, want %v", c.client.Timeout, 5*time.Second)
	}
	if _, ok := c.client.Transport.(*publicOnlyTransport); !ok {
		t.Fatalf("transport: got %T, want *publicOnlyTransport", c.client.Transport)
	}
	if c.client.CheckRedirect == nil {
		t.Fatal("NewPublicOnlyClient must keep CheckRedirect")
	}
	got := reflect.ValueOf(c.client.CheckRedirect).Pointer()
	want := reflect.ValueOf(PublicOnlyCheckRedirect).Pointer()
	if got != want {
		t.Error("NewPublicOnlyClient CheckRedirect must stay PublicOnlyCheckRedirect")
	}

	tr := publicOnlyHTTPTransport(t, c)
	if tr.Proxy != nil {
		t.Error("public-only client must not use a proxy")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want at least TLS 1.2 (%#x)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure=true must set InsecureSkipVerify")
	}
}
