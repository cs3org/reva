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

package sciencemesh

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
)

const invalidFederationCIDRText = "invalid federation CIDR"

func TestPublicOCMTransportConfigScalesTimeoutAndCopiesTLS(t *testing.T) {
	t.Parallel()

	c := &config{
		OCMClientTimeout:       7,
		OCMClientInsecure:      true,
		AllowedFederationCIDRs: []string{"10.1.2.0/24"},
	}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	if cfg.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s (integer seconds scaled by time.Second)", cfg.Timeout)
	}
	if !cfg.Insecure {
		t.Error("Insecure = false, want true (copied from service config)")
	}
	if cfg.AllowLoopback {
		t.Error("AllowLoopback must stay false for ScienceMesh public discovery")
	}
	if cfg.UseEnvProxy {
		t.Error("UseEnvProxy must stay false for ScienceMesh public discovery")
	}

	// Exercise the actual constructor wiring: the helper output, fed to the
	// public-only constructor, installs a guarded dialer. An in-range address
	// passes the guard (then fails to connect); an unrelated private range is
	// denied by policy. This does not duplicate H1's normalization rules.
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	if httpClient.Timeout != 7*time.Second {
		t.Errorf("HTTP client Timeout = %v, want 7s", httpClient.Timeout)
	}
	tr := client.HTTPTransport(httpClient.Transport)
	if tr == nil {
		t.Fatalf("transport: got %T, want public-only base *http.Transport", httpClient.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want TLS 1.2", tr.TLSClientConfig.MinVersion)
	}
	if tr.Proxy != nil {
		t.Error("Proxy is set, want nil")
	}
	if err := dialTransport(tr, "10.1.2.3:9"); errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("in-range 10.1.2.3 denied by policy = %v, want a non-policy dial error", err)
	}
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("unlisted 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}
}

func TestPublicOCMTransportConfigRejectsInvalidCIDRs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
	}{
		{name: "invalid after valid", values: []string{"10.0.0.0/8", "nope"}},
		{name: "public range", values: []string{"8.8.8.8/32"}},
		{name: "host bits", values: []string{"10.1.2.3/8"}},
		{name: "loopback", values: []string{"127.0.0.0/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &config{AllowedFederationCIDRs: tt.values}
			_, err := c.publicOCMTransportConfig()
			if err == nil {
				t.Fatalf("publicOCMTransportConfig: expected error for %v", tt.values)
			}
			if !strings.Contains(err.Error(), invalidFederationCIDRText) {
				t.Fatalf("error = %v, want %q in message", err, invalidFederationCIDRText)
			}
		})
	}
}

func TestPublicOCMTransportConfigDefaultDeniesPrivate(t *testing.T) {
	t.Parallel()

	c := &config{}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	if cfg.AllowLoopback || cfg.UseEnvProxy {
		t.Fatal("default policy must keep AllowLoopback and UseEnvProxy false")
	}
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	tr := client.HTTPTransport(httpClient.Transport)
	if tr == nil {
		t.Fatalf("transport: got %T, want public-only base *http.Transport", httpClient.Transport)
	}
	if err := dialTransport(tr, "192.168.1.50:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("default 192.168.1.50 = %v, want ErrPolicyViolation", err)
	}
	if err := dialTransport(tr, "127.0.0.1:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("default loopback = %v, want ErrPolicyViolation", err)
	}
}

func TestPublicOCMTransportConfigImmutableToConfigMutation(t *testing.T) {
	t.Parallel()

	c := &config{
		OCMClientTimeout:       2,
		AllowedFederationCIDRs: []string{"10.1.2.0/24"},
	}
	cfg, err := c.publicOCMTransportConfig()
	if err != nil {
		t.Fatalf("publicOCMTransportConfig: %v", err)
	}
	httpClient := client.NewPublicOnlyHTTPClient(cfg)
	tr := client.HTTPTransport(httpClient.Transport)
	if tr == nil {
		t.Fatalf("transport: got %T, want public-only base *http.Transport", httpClient.Transport)
	}

	// Mutating the service config after the helper ran must not broaden the
	// built client's policy: 192.168.5.9 stays denied.
	c.AllowedFederationCIDRs[0] = "192.168.5.0/24"
	c.AllowedFederationCIDRs = append(c.AllowedFederationCIDRs, "fd00::/8")
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("after config mutation 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}

	// Reassigning the runtime config's policy field after the client is built
	// must not affect the already-installed dialer either.
	other, err := client.ParseFederationCIDRs([]string{"192.168.5.0/24"})
	if err != nil {
		t.Fatalf("ParseFederationCIDRs: %v", err)
	}
	cfg.AllowedFederationCIDRs = other
	if err := dialTransport(tr, "192.168.5.9:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("after runtime reassignment 192.168.5.9 = %v, want ErrPolicyViolation", err)
	}
}

func TestPublicOCMTransportConfigUseEnvProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "false", enabled: false},
		{name: "true", enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &config{
				OCMClientTimeout:       3,
				OCMClientInsecure:      true,
				OCMClientUseEnvProxy:   tt.enabled,
				AllowedFederationCIDRs: []string{"10.1.2.0/24"},
			}
			cfg, err := c.publicOCMTransportConfig()
			if err != nil {
				t.Fatalf("publicOCMTransportConfig: %v", err)
			}
			if cfg.UseEnvProxy != tt.enabled {
				t.Fatalf("UseEnvProxy = %v, want %v", cfg.UseEnvProxy, tt.enabled)
			}
			if cfg.AllowLoopback {
				t.Fatal("AllowLoopback must stay false")
			}
			if !cfg.Insecure {
				t.Fatal("Insecure = false, want true (copied from service config)")
			}
			httpClient := client.NewPublicOnlyHTTPClient(cfg)
			tr := client.HTTPTransport(httpClient.Transport)
			if tr == nil {
				t.Fatalf("transport: got %T, want public-only base *http.Transport", httpClient.Transport)
			}
			if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
				t.Fatal("InsecureSkipVerify = false, want true (from OCMClientInsecure)")
			}
			assertProxyMode(t, tr, tt.enabled)
			if err := dialTransport(tr, "10.1.2.3:9"); errors.Is(err, client.ErrPolicyViolation) {
				t.Errorf("in-range 10.1.2.3 denied by policy = %v, want a non-policy dial error", err)
			}
		})
	}
}

// ocmClientTransport returns the base *http.Transport of an OCM client.
// The client may be the guarded public-only client or the trusted
// directory client.
func ocmClientTransport(t *testing.T, c *ocmd.OCMClient) *http.Transport {
	t.Helper()
	if c == nil {
		t.Fatal("nil OCM client")
	}
	tr := client.HTTPTransport(c.Transport())
	if tr == nil {
		t.Fatalf("transport: got %T, want public-only base *http.Transport", c.Transport())
	}
	return tr
}

// assertProxyMode checks the public-only proxy callback and that the policy
// dialer still rejects loopback. enabled false leaves Proxy nil.
func assertProxyMode(t *testing.T, tr *http.Transport, enabled bool) {
	t.Helper()
	if tr.DialContext == nil {
		t.Fatal("DialContext is nil, want the policy dialer")
	}
	if err := dialTransport(tr, "127.0.0.1:9"); !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("dial 127.0.0.1:9 = %v, want ErrPolicyViolation", err)
	}
	if !enabled {
		if tr.Proxy != nil {
			t.Error("Proxy is set, want nil")
		}
		return
	}
	assertProxyFromEnvironment(t, tr)
}

func assertProxyFromEnvironment(t *testing.T, tr *http.Transport) {
	t.Helper()
	if tr.Proxy == nil {
		t.Fatal("Proxy is nil, want http.ProxyFromEnvironment")
	}
	got := reflect.ValueOf(tr.Proxy).Pointer()
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	if got != want {
		t.Errorf("Proxy function = %#x, want http.ProxyFromEnvironment (%#x)", got, want)
	}
}

// dialTransport invokes the transport's guarded DialContext with a short
// timeout. A denied address returns ErrPolicyViolation; an admitted address
// fails to connect with a non-policy network or timeout error.
func dialTransport(tr *http.Transport, address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}
