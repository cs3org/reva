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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/rs/zerolog"
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
		if r.URL.RawQuery != "" {
			t.Fatalf("token request used a query: %s", r.URL.RawQuery)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form := string(body)
		if !strings.Contains(form, "grant_type=authorization_code") ||
			!strings.Contains(form, "code=code123") ||
			!strings.Contains(form, "client_id=client1") {
			t.Fatalf("form body %s", form)
		}
		if r.PostForm.Get("grant_type") != "authorization_code" ||
			r.PostForm.Get("code") != "code123" ||
			r.PostForm.Get("client_id") != "client1" {
			t.Fatalf(
				"form values grant_type=%q code=%q client_id=%q",
				r.PostForm.Get("grant_type"),
				r.PostForm.Get("code"),
				r.PostForm.Get("client_id"),
			)
		}
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
	var internal errtypes.InternalError
	if !errors.As(err, &internal) {
		t.Fatalf("expected InternalError, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "not json") {
		t.Fatalf("decode error leaked remote material: %v", err)
	}
}

func TestExchangeTokenTransportAndConstructionOmitRemoteMaterial(t *testing.T) {
	const endpoint = "https://token.example/ocm/token"
	const code = "code-secret"
	c := &OCMClient{client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial " + endpoint + " " + code)
	})}}
	_, _, err := c.ExchangeToken(context.Background(), endpoint, code, "client")
	if err == nil {
		t.Fatal("expected transport error")
	}
	var internal errtypes.InternalError
	if !errors.As(err, &internal) {
		t.Fatalf("expected InternalError, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), endpoint) || strings.Contains(err.Error(), code) {
		t.Fatalf("transport error leaked remote material: %v", err)
	}

	_, _, err = c.ExchangeToken(context.Background(), "http://[::1", code, "client")
	if err == nil {
		t.Fatal("expected construction error")
	}
	if !errors.As(err, &internal) {
		t.Fatalf("expected InternalError, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), code) || strings.Contains(err.Error(), "[::1") {
		t.Fatalf("construction error leaked remote material: %v", err)
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
	tr, ok := c.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("public-only client must use an *http.Transport")
	}
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

func TestNewShareLogsOmitSecretMarker(t *testing.T) {
	const marker = "synthetic-secret-marker"
	newReq := func() *NewShareRequest {
		return &NewShareRequest{
			ShareWith:    "marie@remote.example",
			Name:         "notes.txt",
			ProviderID:   "provider-1",
			Owner:        "einstein@sender.example",
			Sender:       "einstein@sender.example",
			ShareType:    "user",
			ResourceType: "file",
			Protocols: Protocols{
				&WebDAV{
					URI:          "https://sender.example/remote.php/dav/ocm/share-opaque",
					SharedSecret: marker,
					Permissions:  []string{"read"},
					Requirements: []string{"must-exchange-token"},
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		var seen bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if !strings.Contains(string(body), marker) {
				t.Errorf("posted body missing marker: %s", body)
			}
			seen = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"recipientDisplayName":"Marie"}`))
		}))
		defer srv.Close()

		logs := captureNewShareLogs(t, srv.URL, http.StatusCreated, newReq())
		if !seen {
			t.Fatal("server did not see the share request")
		}
		if strings.Contains(logs, marker) {
			t.Fatalf("logs contain secret marker: %s", logs)
		}
		if !strings.Contains(logs, "Sending OCM share") || !strings.Contains(logs, "provider-1") {
			t.Fatalf("logs missing outcome diagnostic: %s", logs)
		}
		if !strings.Contains(logs, "remote OCM server responded") {
			t.Fatalf("logs missing response diagnostic: %s", logs)
		}
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if !strings.Contains(string(body), marker) {
				t.Errorf("posted body missing marker: %s", body)
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(marker))
		}))
		defer srv.Close()

		var logs string
		var err error
		logs, err = captureNewShareLogsResult(t, srv.URL, newReq())
		if err == nil || !strings.Contains(err.Error(), marker) {
			t.Fatalf("error = %v, want the response marker in the returned error only", err)
		}
		if strings.Contains(logs, marker) {
			t.Fatalf("error logs contain secret marker: %s", logs)
		}
		if !strings.Contains(logs, "error in remote OCM server response") {
			t.Fatalf("logs missing error diagnostic: %s", logs)
		}
	})
}

func captureNewShareLogs(t *testing.T, endpoint string, wantStatus int, req *NewShareRequest) string {
	t.Helper()
	logs, status, err := observeNewShareLogs(t, endpoint, req)
	if err != nil {
		t.Fatal(err)
	}
	if status != wantStatus {
		t.Fatalf("status = %d, want %d", status, wantStatus)
	}
	return logs
}

func captureNewShareLogsResult(t *testing.T, endpoint string, req *NewShareRequest) (string, error) {
	t.Helper()
	logs, _, err := observeNewShareLogs(t, endpoint, req)
	return logs, err
}

func observeNewShareLogs(t *testing.T, endpoint string, req *NewShareRequest) (string, int, error) {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	ctx := appctx.WithLogger(context.Background(), &logger)
	client := NewClient(2*time.Second, true)
	capture := &statusCaptureTransport{base: client.client.Transport}
	client.client.Transport = capture
	_, err := client.NewShare(ctx, endpoint, req)
	return buf.String(), capture.status, err
}

type statusCaptureTransport struct {
	base   http.RoundTripper
	status int
}

func (s *statusCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := s.base.RoundTrip(req)
	if resp != nil {
		s.status = resp.StatusCode
	}
	return resp, err
}
