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

package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNonPositiveTimeoutNormalizesTo10Seconds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "zero", timeout: 0},
		{name: "negative", timeout: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := TransportConfig{Timeout: tt.timeout}
			trusted := NewTrustedHTTPClient(cfg)
			if trusted.Timeout != defaultTimeout {
				t.Errorf("trusted timeout = %v, want %v", trusted.Timeout, defaultTimeout)
			}
			public := NewPublicOnlyHTTPClient(cfg)
			if public.Timeout != defaultTimeout {
				t.Errorf("public-only timeout = %v, want %v", public.Timeout, defaultTimeout)
			}
		})
	}

	defaults := DefaultTransportConfig()
	if defaults.Timeout != defaultTimeout {
		t.Errorf("DefaultTransportConfig timeout = %v, want %v", defaults.Timeout, defaultTimeout)
	}
	if defaults.Insecure {
		t.Error("DefaultTransportConfig Insecure must stay false")
	}
	if defaults.AllowLoopback {
		t.Error("DefaultTransportConfig AllowLoopback must stay false")
	}
	if defaults.UseEnvProxy {
		t.Error("DefaultTransportConfig UseEnvProxy must stay false")
	}
}

func TestTrustedClientRetainsProxy(t *testing.T) {
	t.Parallel()

	c := NewTrustedHTTPClient(TransportConfig{Timeout: time.Second})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("trusted client must retain a proxy callback")
	}
	got := reflect.ValueOf(tr.Proxy).Pointer()
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	if got != want {
		t.Error("trusted client Proxy must be http.ProxyFromEnvironment")
	}
}

func TestPublicOnlyClientHasNoProxy(t *testing.T) {
	t.Parallel()

	c := NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("public-only transport: got %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy != nil {
		t.Error("public-only client must not use a proxy")
	}

	rt := NewPublicOnlyRoundTripper(TransportConfig{Timeout: time.Second})
	rtr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("public-only round tripper: got %T, want *http.Transport", rt)
	}
	if rtr.Proxy != nil {
		t.Error("public-only round tripper must not use a proxy")
	}

	explicit := TransportConfig{Timeout: time.Second, UseEnvProxy: false}
	explicitClient := NewPublicOnlyHTTPClient(explicit)
	explicitTr, ok := explicitClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("explicit false transport: got %T, want *http.Transport", explicitClient.Transport)
	}
	if explicitTr.Proxy != nil {
		t.Error("UseEnvProxy false must leave the public-only client Proxy nil")
	}
	explicitRT := NewPublicOnlyRoundTripper(explicit)
	explicitRTR, ok := explicitRT.(*http.Transport)
	if !ok {
		t.Fatalf("explicit false round tripper: got %T, want *http.Transport", explicitRT)
	}
	if explicitRTR.Proxy != nil {
		t.Error("UseEnvProxy false must leave the public-only round tripper Proxy nil")
	}
}

func TestPublicOnlyUseEnvProxyInstallsProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	cfg := TransportConfig{Timeout: time.Second, UseEnvProxy: true}
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()

	c := NewPublicOnlyHTTPClient(cfg)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("public-only transport: got %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("UseEnvProxy true must install a proxy callback")
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != want {
		t.Error("UseEnvProxy true Proxy must be http.ProxyFromEnvironment")
	}
	if tr.DialContext == nil {
		t.Fatal("UseEnvProxy true must keep DialContext installed")
	}
	if err := dialThrough(t, tr, "127.0.0.1:9"); !errors.Is(err, ErrPolicyViolation) {
		t.Errorf("UseEnvProxy true dial 127.0.0.1:9: %v, want ErrPolicyViolation", err)
	}

	rt := NewPublicOnlyRoundTripper(cfg)
	rtr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("public-only round tripper: got %T, want *http.Transport", rt)
	}
	if rtr.Proxy == nil {
		t.Fatal("UseEnvProxy true round tripper must install a proxy callback")
	}
	if reflect.ValueOf(rtr.Proxy).Pointer() != want {
		t.Error("UseEnvProxy true round tripper Proxy must be http.ProxyFromEnvironment")
	}
	if rtr.DialContext == nil {
		t.Fatal("UseEnvProxy true round tripper must keep DialContext installed")
	}
}

func TestTrustedClientIgnoresUseEnvProxy(t *testing.T) {
	t.Parallel()

	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	tests := []struct {
		name        string
		useEnvProxy bool
	}{
		{name: "false", useEnvProxy: false},
		{name: "true", useEnvProxy: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewTrustedHTTPClient(TransportConfig{
				Timeout:     time.Second,
				UseEnvProxy: tt.useEnvProxy,
			})
			tr, ok := c.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("trusted transport: got %T, want *http.Transport", c.Transport)
			}
			if tr.Proxy == nil {
				t.Fatal("trusted client must retain a proxy callback")
			}
			if reflect.ValueOf(tr.Proxy).Pointer() != want {
				t.Error("trusted client Proxy must be http.ProxyFromEnvironment")
			}
		})
	}
}

func TestPublicOnlyProxyHopAddressPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		allowLoopback bool
		proxy         string
	}{
		{name: "loopback denied without AllowLoopback", allowLoopback: false, proxy: "http://127.0.0.1:9"},
		{name: "rfc1918 denied with AllowLoopback", allowLoopback: true, proxy: "http://10.1.2.3:9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewPublicOnlyHTTPClient(TransportConfig{
				Timeout:       2 * time.Second,
				AllowLoopback: tt.allowLoopback,
				Insecure:      true,
			})
			setTestProxy(t, c.Transport, tt.proxy)
			err := doHTTPS(t, c, envProxyTarget)
			if !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("Do(%s via %s) = %v, want ErrPolicyViolation", envProxyTarget, tt.proxy, err)
			}
		})
	}
}

func TestPublicOnlyLoopbackProxyReceivesCONNECT(t *testing.T) {
	t.Parallel()

	spy := startCONNECTSpy(t)
	c := NewPublicOnlyHTTPClient(TransportConfig{
		Timeout:       2 * time.Second,
		AllowLoopback: true,
		Insecure:      true,
	})
	setTestProxy(t, c.Transport, "http://"+spy.addr())
	_ = doHTTPS(t, c, envProxyTarget)

	want := "CONNECT " + envProxyHost + ":443"
	for _, got := range spy.seenRequests() {
		if got == want {
			return
		}
	}
	t.Fatalf("loopback proxy requests = %q, want %q", spy.seenRequests(), want)
}

func TestPublicOnlyDirectModeDialsTarget(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	spy := startCONNECTSpy(t)
	proxyURL := "http://" + spy.addr()
	cfg := TransportConfig{
		Timeout:       2 * time.Second,
		AllowLoopback: true,
		Insecure:      true,
		UseEnvProxy:   false,
	}

	direct := NewPublicOnlyHTTPClient(cfg)
	tr, ok := direct.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("public-only transport: got %T, want *http.Transport", direct.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("direct mode must leave Proxy nil")
	}

	resp, err := direct.Get(srv.URL)
	if err != nil {
		t.Fatalf("direct Get(%s): %v", srv.URL, err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("direct mode target hits = %d, want 1", hits.Load())
	}
	if seen := spy.seenRequests(); len(seen) != 0 {
		t.Fatalf("direct mode contacted the configured proxy: %q", seen)
	}

	proxied := NewPublicOnlyHTTPClient(cfg)
	setTestProxy(t, proxied.Transport, proxyURL)
	_ = doHTTPS(t, proxied, srv.URL)
	if hits.Load() != 1 {
		t.Fatalf("configured proxy reached the target: hits = %d, want 1", hits.Load())
	}
	want := "CONNECT " + target.Host
	for _, got := range spy.seenRequests() {
		if got == want {
			return
		}
	}
	t.Fatalf("configured proxy requests = %q, want %q", spy.seenRequests(), want)
}

// ProxyFromEnvironment reads HTTP_PROXY, HTTPS_PROXY, and NO_PROXY once per
// process. Each snapshot runs in its own subprocess. The target is a
// non-loopback hostname because Go bypasses proxies for localhost.
func TestPublicOnlyEnvProxySnapshots(t *testing.T) {
	if scenario := os.Getenv(envProxyChildKey); scenario != "" {
		runPublicOnlyEnvProxyChild(t, scenario)
		return
	}

	t.Run("HTTPS_PROXY", func(t *testing.T) {
		spy := startCONNECTSpy(t)
		runEnvProxyChild(t, "HTTPS_PROXY", map[string]string{
			"HTTPS_PROXY": "http://" + spy.addr(),
		})
		want := "CONNECT " + envProxyHost + ":443"
		for _, got := range spy.seenRequests() {
			if got == want {
				return
			}
		}
		t.Fatalf("HTTPS_PROXY child requests = %q, want %q", spy.seenRequests(), want)
	})

	t.Run("NO_PROXY", func(t *testing.T) {
		spy := startCONNECTSpy(t)
		runEnvProxyChild(t, "NO_PROXY", map[string]string{
			"HTTPS_PROXY": "http://" + spy.addr(),
			"NO_PROXY":    envProxyHost,
		})
		if seen := spy.seenRequests(); len(seen) != 0 {
			t.Fatalf("NO_PROXY child contacted the proxy: %q", seen)
		}
	})
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
			t.Parallel()
			ip, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("could not parse %q: %v", tt.ip, err)
			}
			if got := isPublicIP(ip); got != tt.want {
				t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestWellKnownNAT64Unwrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "nat64 metadata service", ip: "64:ff9b::a9fe:a9fe", want: false},
		{name: "nat64 loopback", ip: "64:ff9b::7f00:1", want: false},
		{name: "nat64 public address", ip: "64:ff9b::5db8:d822", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("could not parse %q: %v", tt.ip, err)
			}
			if got := isPublicIP(ip); got != tt.want {
				t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
			unwrapped := effectiveAddr(ip)
			if !unwrapped.Is4() {
				t.Fatalf("NAT64 unwrap of %s = %s, want IPv4", tt.ip, unwrapped)
			}
		})
	}
}

func TestPublicOnlyDialAddressPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "public host", address: "93.184.216.34:443"},
		{name: "loopback", address: "127.0.0.1:8080", wantErr: true},
		{name: "loopback v6", address: "[::1]:8080", wantErr: true},
		{name: "rfc1918 10/8", address: "10.0.0.5:9000", wantErr: true},
		{name: "rfc1918 192.168/16", address: "192.168.1.1:443", wantErr: true},
		{name: "link-local metadata", address: "169.254.169.254:80", wantErr: true},
		{name: "link-local v6", address: "[fe80::1]:80", wantErr: true},
		{name: "cgnat", address: "100.64.0.1:443", wantErr: true},
		{name: "ipv4-mapped private", address: "[::ffff:10.1.2.3]:443", wantErr: true},
		{name: "ipv4-mapped loopback", address: "[::ffff:127.0.0.1]:8080", wantErr: true},
		{name: "no port", address: "93.184.216.34", wantErr: true},
	}

	control := refuseNonPublicAddr(destinationPolicy{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := control("tcp", tt.address, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("refuseNonPublicAddr(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
			}
			if !tt.wantErr || err == nil {
				return
			}
			_, _, splitErr := net.SplitHostPort(tt.address)
			if splitErr == nil && !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("blocked dial %q: %v, want errors.Is ErrPolicyViolation", tt.address, err)
			}
		})
	}
}

func TestAllowLoopbackPermitsOnlyLoopback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "loopback v4", address: "127.0.0.1:8080"},
		{name: "loopback v6", address: "[::1]:8080"},
		{name: "ipv4-mapped loopback", address: "[::ffff:127.0.0.1]:8080"},
		{name: "rfc1918", address: "10.1.2.3:443", wantErr: true},
		{name: "rfc1918 192.168", address: "192.168.1.1:443", wantErr: true},
		{name: "link-local", address: "169.254.169.254:80", wantErr: true},
		{name: "link-local v6", address: "[fe80::1]:80", wantErr: true},
	}

	control := refuseNonPublicAddr(destinationPolicy{allowLoopback: true})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := control("tcp", tt.address, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("AllowLoopback refuseNonPublicAddr(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("blocked dial %q: %v, want errors.Is ErrPolicyViolation", tt.address, err)
			}
		})
	}
}

func TestBlockedDialsWrapErrPolicyViolation(t *testing.T) {
	t.Parallel()

	addresses := []string{
		"127.0.0.1:8080",
		"10.0.0.5:9000",
		"169.254.169.254:80",
		"100.64.0.1:443",
		"[::ffff:192.168.1.1]:443",
	}

	control := refuseNonPublicAddr(destinationPolicy{})
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			err := control("tcp", address, nil)
			if !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("blocked dial %q: %v, want errors.Is ErrPolicyViolation", address, err)
			}
		})
	}
}

func TestPublicOnlyConstructorsEnforceSameAddressPolicy(t *testing.T) {
	t.Parallel()

	cfg := TransportConfig{Timeout: time.Second}
	clientRT := NewPublicOnlyHTTPClient(cfg).Transport
	tripperRT := NewPublicOnlyRoundTripper(cfg)

	blocked := []string{
		"127.0.0.1:9",
		"10.1.2.3:9",
		"169.254.1.1:9",
		"100.64.0.1:9",
		"[::ffff:192.168.0.1]:9",
	}

	for _, address := range blocked {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			clientErr := dialThrough(t, clientRT, address)
			tripperErr := dialThrough(t, tripperRT, address)
			if !errors.Is(clientErr, ErrPolicyViolation) {
				t.Errorf("NewPublicOnlyHTTPClient dial %q: %v, want ErrPolicyViolation", address, clientErr)
			}
			if !errors.Is(tripperErr, ErrPolicyViolation) {
				t.Errorf("NewPublicOnlyRoundTripper dial %q: %v, want ErrPolicyViolation", address, tripperErr)
			}
		})
	}
}

func TestMalformedDialAddressWrapsPolicyViolation(t *testing.T) {
	t.Parallel()

	addresses := []string{
		"93.184.216.34",
		"10.1.2.3",
		"[::1]",
		"not-a-socket",
	}
	policy := destinationPolicy{}
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			err := checkResolvedAddr(address, policy)
			if !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("checkResolvedAddr(%q) = %v, want errors.Is ErrPolicyViolation", address, err)
			}
		})
	}
}

func TestDefaultTransportConfigHasNoFederationCIDRs(t *testing.T) {
	t.Parallel()

	defaults := DefaultTransportConfig()
	if len(defaults.AllowedFederationCIDRs.prefixes) != 0 {
		t.Fatalf("default federation CIDRs = %v, want none", defaults.AllowedFederationCIDRs.prefixes)
	}
	if defaults.AllowedFederationCIDRs.contains(netip.MustParseAddr("10.1.2.3")) {
		t.Fatal("zero federation policy admitted a private address")
	}
}

func TestTrustedClientIgnoresFederationCIDRs(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// Loopback is never admitted by a CIDR, only by AllowLoopback. Dialing it
	// through the real trusted DialContext fails if that path grows a CIDR guard.
	cidrs := mustFederationCIDRs(t, "10.1.0.0/16")
	c := NewTrustedHTTPClient(TransportConfig{
		Timeout:                time.Second,
		UseEnvProxy:            false,
		AllowLoopback:          false,
		AllowedFederationCIDRs: cidrs,
	})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("trusted client must retain a proxy callback")
	}
	got := reflect.ValueOf(tr.Proxy).Pointer()
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	if got != want {
		t.Error("trusted client Proxy must stay http.ProxyFromEnvironment")
	}
	if tr.DialContext == nil {
		t.Fatal("trusted client must retain the standard dialer")
	}
	if err := dialThrough(t, tr, ln.Addr().String()); err != nil {
		t.Fatalf(
			"trusted dial %s: %v, want nil (trusted path must be unguarded)",
			ln.Addr().String(),
			err,
		)
	}

	public := NewPublicOnlyHTTPClient(TransportConfig{
		Timeout:                time.Second,
		AllowLoopback:          false,
		AllowedFederationCIDRs: cidrs,
	})
	publicTr, ok := public.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("public-only transport: got %T, want *http.Transport", public.Transport)
	}
	if err := dialThrough(t, publicTr, ln.Addr().String()); !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("public-only dial %s = %v, want ErrPolicyViolation", ln.Addr().String(), err)
	}
}

func TestPublicOnlyConstructorsKeepGuardedDefaultDeny(t *testing.T) {
	t.Parallel()

	cfg := TransportConfig{Timeout: time.Second}
	clientRT := NewPublicOnlyHTTPClient(cfg).Transport
	tripperRT := NewPublicOnlyRoundTripper(cfg)
	blocked := []string{
		"10.9.8.7:9",
		"192.168.4.4:9",
		"127.0.0.1:9",
		"[fd00::1]:9",
	}

	for _, rt := range []http.RoundTripper{clientRT, tripperRT} {
		tr, ok := rt.(*http.Transport)
		if !ok {
			t.Fatalf("transport: got %T, want *http.Transport", rt)
		}
		if tr.DialContext == nil {
			t.Fatal("public constructor must install a guarded DialContext")
		}
		for _, address := range blocked {
			err := dialThrough(t, rt, address)
			if !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("default-deny dial %q: %v, want ErrPolicyViolation", address, err)
			}
		}
	}
}

func TestFederationCIDRDestinationPolicy(t *testing.T) {
	t.Parallel()

	cidrs := mustFederationCIDRs(t, "10.1.2.0/24", "fd12:3456:789a::/48")
	policy := destinationPolicy{cidrs: cidrs}
	loopbackPolicy := destinationPolicy{allowLoopback: true}
	both := destinationPolicy{allowLoopback: true, cidrs: cidrs}

	tests := []struct {
		name    string
		policy  destinationPolicy
		address string
		wantErr bool
	}{
		{name: "exact subnet base", policy: policy, address: "10.1.2.0:443"},
		{name: "exact subnet end", policy: policy, address: "10.1.2.255:443"},
		{name: "below subnet", policy: policy, address: "10.1.1.255:443", wantErr: true},
		{name: "above subnet", policy: policy, address: "10.1.3.0:443", wantErr: true},
		{name: "ipv4-mapped in cidr", policy: policy, address: "[::ffff:10.1.2.3]:443"},
		{name: "nat64 in cidr", policy: policy, address: "[64:ff9b::a01:203]:443"},
		{name: "nat64 outside cidr", policy: policy, address: "[64:ff9b::a01:303]:443", wantErr: true},
		{name: "mapped outside cidr", policy: policy, address: "[::ffff:10.9.9.9]:443", wantErr: true},
		{name: "unlisted rfc1918", policy: policy, address: "192.168.1.1:443", wantErr: true},
		{name: "unlisted ula", policy: policy, address: "[fd00::1]:443", wantErr: true},
		{name: "listed ula", policy: policy, address: "[fd12:3456:789a::1]:443"},
		{name: "public still allowed", policy: policy, address: "93.184.216.34:443"},
		{name: "loopback not granted by cidr", policy: policy, address: "127.0.0.1:8080", wantErr: true},
		{name: "v6 loopback not granted by cidr", policy: policy, address: "[::1]:8080", wantErr: true},
		{name: "mapped loopback not granted by cidr", policy: policy, address: "[::ffff:127.0.0.1]:8080", wantErr: true},
		{name: "nat64 loopback not granted by cidr", policy: policy, address: "[64:ff9b::7f00:1]:8080", wantErr: true},
		{name: "link-local still denied", policy: policy, address: "169.254.169.254:80", wantErr: true},
		{name: "cgnat still denied", policy: policy, address: "100.64.0.1:443", wantErr: true},
		{name: "allow loopback without cidr", policy: loopbackPolicy, address: "127.0.0.1:8080"},
		{
			name:    "allow loopback does not admit ula",
			policy:  loopbackPolicy,
			address: "[fd12:3456:789a::1]:443",
			wantErr: true,
		},
		{name: "allow loopback does not admit rfc1918", policy: loopbackPolicy, address: "10.1.2.3:443", wantErr: true},
		{name: "both flags admit loopback and cidr", policy: both, address: "[::1]:8080"},
		{name: "both flags admit listed private", policy: both, address: "10.1.2.9:443"},
		{name: "both flags deny unlisted private", policy: both, address: "172.16.0.1:443", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkResolvedAddr(tt.address, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkResolvedAddr(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("blocked dial %q: %v, want errors.Is ErrPolicyViolation", tt.address, err)
			}
		})
	}
}

func TestFederationCIDRClientIsolation(t *testing.T) {
	t.Parallel()

	ten := mustFederationCIDRs(t, "10.1.0.0/16")
	lan := mustFederationCIDRs(t, "192.168.5.0/24")
	cfgTen := TransportConfig{Timeout: time.Second, AllowedFederationCIDRs: ten}
	cfgLan := TransportConfig{Timeout: time.Second, AllowedFederationCIDRs: lan}

	clientTen := NewPublicOnlyHTTPClient(cfgTen)
	clientLan := NewPublicOnlyHTTPClient(cfgLan)
	tripperTen := NewPublicOnlyRoundTripper(cfgTen)
	if err := dialThrough(t, clientTen.Transport, "192.168.5.9:9"); !errors.Is(err, ErrPolicyViolation) {
		t.Errorf("10/16 client dialed 192.168.5.9: %v", err)
	}
	if err := dialThrough(t, clientLan.Transport, "10.1.2.3:9"); !errors.Is(err, ErrPolicyViolation) {
		t.Errorf("192.168 client dialed 10.1.2.3: %v", err)
	}
	if err := dialThrough(t, tripperTen, "192.168.5.9:9"); !errors.Is(err, ErrPolicyViolation) {
		t.Errorf("10/16 round tripper dialed 192.168.5.9: %v", err)
	}

	wide := mustFederationCIDRs(t, "10.0.0.0/8")
	cfgWide := TransportConfig{Timeout: time.Second, AllowedFederationCIDRs: wide}
	dialer := newPublicOnlyDialer(cfgWide)
	sentinel := guardDialerControlSentinel(t, dialer)
	const (
		allowed = "10.1.2.3:443"
		denied  = "192.168.9.9:443"
	)
	assertGuardedCIDRDial(t, dialer, allowed, denied, sentinel)
	cfgWide.AllowedFederationCIDRs = lan
	wide.prefixes[0] = netip.MustParsePrefix("11.0.0.0/8")
	assertGuardedCIDRDial(t, dialer, allowed, denied, sentinel)
}

// guardDialerControlSentinel wraps the production Control callback. The
// original runs first. A policy denial is returned as-is. An allowed address
// returns the sentinel so DialContext stops before connect.
func guardDialerControlSentinel(t *testing.T, dialer *net.Dialer) error {
	t.Helper()
	if dialer.Control == nil {
		t.Fatal("production dialer must install Control")
	}
	sentinel := errors.New("in-range federation dial sentinel")
	orig := dialer.Control
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		if err := orig(network, address, c); err != nil {
			return err
		}
		return sentinel
	}
	return sentinel
}

func assertGuardedCIDRDial(t *testing.T, dialer *net.Dialer, allowed, denied string, sentinel error) {
	t.Helper()

	probe := func(address string, wantSentinel bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if conn != nil {
			_ = conn.Close()
			t.Fatalf("dial %q returned a connection", address)
		}
		if wantSentinel {
			if !errors.Is(err, sentinel) {
				t.Fatalf("in-range dial %q = %v, want sentinel", address, err)
			}
			if errors.Is(err, ErrPolicyViolation) {
				t.Fatalf("in-range dial %q = %v, want no policy violation", address, err)
			}
			return
		}
		if !errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("denied dial %q = %v, want ErrPolicyViolation", address, err)
		}
		if errors.Is(err, sentinel) {
			t.Fatalf("denied dial %q returned the allow sentinel", address)
		}
	}

	probe(allowed, true)
	probe(denied, false)
}

func TestFederationCIDRConcurrentClassification(t *testing.T) {
	t.Parallel()

	ten := mustFederationCIDRs(t, "10.8.0.0/16")
	ula := mustFederationCIDRs(t, "fd00:abcd::/32")
	policyTen := destinationPolicy{cidrs: ten}
	policyULA := destinationPolicy{cidrs: ula}
	dialer := newPublicOnlyDialer(TransportConfig{
		Timeout:                time.Second,
		AllowedFederationCIDRs: ten,
	})
	sentinel := errors.New("concurrent federation dial sentinel")
	orig := dialer.Control
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		if err := orig(network, address, c); err != nil {
			return err
		}
		return sentinel
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if err := checkResolvedAddr("10.8.1.1:443", policyTen); err != nil {
					t.Errorf("listed private: %v", err)
				}
				if err := checkResolvedAddr("192.168.9.9:443", policyTen); !errors.Is(err, ErrPolicyViolation) {
					t.Errorf("unlisted private: %v", err)
				}
				if err := checkResolvedAddr("[fd00:abcd::5]:443", policyULA); err != nil {
					t.Errorf("listed ula: %v", err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				conn, err := dialer.DialContext(ctx, "tcp", "10.8.1.1:443")
				cancel()
				if conn != nil {
					_ = conn.Close()
					t.Error("concurrent allow returned a connection")
				}
				if !errors.Is(err, sentinel) {
					t.Errorf("concurrent allow = %v, want sentinel", err)
				}
				ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
				conn, err = dialer.DialContext(ctx, "tcp", "192.168.9.9:443")
				cancel()
				if conn != nil {
					_ = conn.Close()
				}
				if !errors.Is(err, ErrPolicyViolation) {
					t.Errorf("concurrent deny = %v, want ErrPolicyViolation", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestPublicOnlyDialerCIDRSentinelDoesNotConnect(t *testing.T) {
	t.Parallel()

	cidrs := mustFederationCIDRs(t, "10.50.0.0/16")
	dialer := newPublicOnlyDialer(TransportConfig{
		Timeout:                time.Second,
		AllowedFederationCIDRs: cidrs,
	})
	assertDialSentinel(t, dialer, "10.50.1.1:443", "10.51.1.1:443")
}

func assertDialSentinel(t *testing.T, dialer *net.Dialer, allowed, denied string) {
	t.Helper()
	if dialer.Control == nil {
		t.Fatal("production dialer must install Control")
	}
	sentinel := errors.New("federation dial sentinel")
	orig := dialer.Control
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		if err := orig(network, address, c); err != nil {
			return err
		}
		return sentinel
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", allowed)
	if conn != nil {
		_ = conn.Close()
		t.Fatal("allowed dial returned a connection")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("allowed dial %q = %v, want sentinel", allowed, err)
	}
	if errors.Is(err, ErrPolicyViolation) {
		t.Fatal("allowed dial wrapped policy violation")
	}

	conn, err = dialer.DialContext(ctx, "tcp", denied)
	if conn != nil {
		_ = conn.Close()
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("denied dial %q = %v, want ErrPolicyViolation", denied, err)
	}
	if errors.Is(err, sentinel) {
		t.Fatal("denied dial returned the allow sentinel")
	}
}

func TestConstructionDoesNotMutateDefaultTransport(t *testing.T) {
	orig := http.DefaultTransport
	origTr, ok := orig.(*http.Transport)
	if !ok {
		t.Fatal("http.DefaultTransport is not *http.Transport")
	}
	tlsBefore := origTr.TLSClientConfig
	proxyBefore := origTr.Proxy

	_ = NewTrustedHTTPClient(TransportConfig{Timeout: time.Second, Insecure: true})
	_ = NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second, Insecure: true})

	if http.DefaultTransport != orig {
		t.Fatal("construction replaced http.DefaultTransport")
	}
	if origTr.TLSClientConfig != tlsBefore {
		t.Error("construction mutated http.DefaultTransport TLSClientConfig")
	}
	if reflect.ValueOf(origTr.Proxy).Pointer() != reflect.ValueOf(proxyBefore).Pointer() {
		t.Error("construction mutated http.DefaultTransport Proxy")
	}
	if tlsBefore != nil && tlsBefore.InsecureSkipVerify {
		t.Error("construction set InsecureSkipVerify on http.DefaultTransport")
	}
}

func TestTrustedConstructionDoesNotMutateExistingTLSConfig(t *testing.T) {
	origTLS := defaultTransportTemplate.TLSClientConfig
	shared := &tls.Config{ServerName: "ocm.example"}
	defaultTransportTemplate.TLSClientConfig = shared
	t.Cleanup(func() {
		defaultTransportTemplate.TLSClientConfig = origTLS
	})

	c := NewTrustedHTTPClient(TransportConfig{Timeout: time.Second, Insecure: true})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport", c.Transport)
	}
	if shared.InsecureSkipVerify {
		t.Error("pre-existing TLS config InsecureSkipVerify was mutated")
	}
	if shared.ServerName != "ocm.example" {
		t.Error("pre-existing TLS config ServerName was mutated")
	}
	if tr.TLSClientConfig == shared {
		t.Error("constructed client must not reuse the template TLS config pointer")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("constructed client must apply Insecure on its own TLS config")
	}
	if tr.TLSClientConfig.ServerName != "ocm.example" {
		t.Error("cloned TLS config should keep ServerName")
	}
}

func dialThrough(t *testing.T, rt http.RoundTripper, address string) error {
	t.Helper()
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("got %T, want *http.Transport", rt)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

const (
	envProxyChildKey = "OCM_CLIENT_TEST_PROXY_CHILD"
	envProxyTarget   = "https://ocm-target.example/"
	envProxyHost     = "ocm-target.example"
)

func doHTTPS(t *testing.T, c *http.Client, rawURL string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return err
}

func setTestProxy(t *testing.T, rt http.RoundTripper, rawURL string) {
	t.Helper()
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("got %T, want *http.Transport", rt)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	tr.Proxy = http.ProxyURL(u)
}

type connectSpy struct {
	ln   net.Listener
	mu   sync.Mutex
	seen []string
}

func startCONNECTSpy(t *testing.T) *connectSpy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &connectSpy{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go s.accept()
	return s
}

func (s *connectSpy) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *connectSpy) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	s.mu.Lock()
	s.seen = append(s.seen, req.Method+" "+req.Host)
	s.mu.Unlock()
	_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n"))
}

func (s *connectSpy) addr() string {
	return s.ln.Addr().String()
}

func (s *connectSpy) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.seen))
	copy(out, s.seen)
	return out
}

func dropProxyEnvKey(key string) bool {
	switch strings.ToLower(key) {
	case "http_proxy", "https_proxy", "no_proxy", "cgi_no_proxy":
		return true
	default:
		return key == envProxyChildKey
	}
}

func childTestEnv(extra map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if dropProxyEnvKey(key) {
			continue
		}
		if _, ok := extra[key]; ok {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func runEnvProxyChild(t *testing.T, scenario string, extra map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestPublicOnlyEnvProxySnapshots$",
		"-test.v=true",
	)
	env := map[string]string{envProxyChildKey: scenario}
	for k, v := range extra {
		env[k] = v
	}
	cmd.Env = childTestEnv(env)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child %s timed out: %v\n%s", scenario, err, out)
	}
	if err != nil {
		t.Fatalf("child %s failed: %v\n%s", scenario, err, out)
	}
}

func runPublicOnlyEnvProxyChild(t *testing.T, scenario string) {
	t.Helper()
	c := NewPublicOnlyHTTPClient(TransportConfig{
		Timeout:       2 * time.Second,
		AllowLoopback: true,
		Insecure:      true,
		UseEnvProxy:   true,
	})
	err := doHTTPS(t, c, envProxyTarget)
	switch scenario {
	case "HTTPS_PROXY":
		if errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("HTTPS_PROXY child denied the proxy hop: %v", err)
		}
	case "NO_PROXY":
		// Direct dial of a non-loopback hostname. The parent asserts that
		// the spy received no CONNECT.
	default:
		t.Fatalf("unknown child scenario %q", scenario)
	}
}
