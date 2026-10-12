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
	"encoding/binary"
	"errors"
	"fmt"
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
	tr := requirePublicTransport(t, c.Transport)
	if tr.Proxy != nil {
		t.Error("public-only client must not use a proxy")
	}

	rt := NewPublicOnlyRoundTripper(TransportConfig{Timeout: time.Second})
	rtr := requirePublicTransport(t, rt)
	if rtr.Proxy != nil {
		t.Error("public-only round tripper must not use a proxy")
	}

	explicit := TransportConfig{Timeout: time.Second, UseEnvProxy: false}
	explicitTr := requirePublicTransport(t, NewPublicOnlyHTTPClient(explicit).Transport)
	if explicitTr.Proxy != nil {
		t.Error("UseEnvProxy false must leave the public-only client Proxy nil")
	}
	explicitRTR := requirePublicTransport(t, NewPublicOnlyRoundTripper(explicit))
	if explicitRTR.Proxy != nil {
		t.Error("UseEnvProxy false must leave the public-only round tripper Proxy nil")
	}
}

func TestPublicOnlyUseEnvProxyInstallsProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	cfg := TransportConfig{Timeout: time.Second, UseEnvProxy: true}
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()

	c := NewPublicOnlyHTTPClient(cfg)
	tr := requirePublicTransport(t, c.Transport)
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
	rtr := requirePublicTransport(t, rt)
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
	tr := requirePublicTransport(t, direct.Transport)
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

func TestTLSFloor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		insecure bool
	}{
		{name: "secure"},
		{name: "insecure retains floor", insecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := TransportConfig{Timeout: time.Second, Insecure: tt.insecure}

			trusted := NewTrustedHTTPClient(cfg)
			trustedTr, ok := trusted.Transport.(*http.Transport)
			if !ok {
				t.Fatalf("trusted transport: got %T, want *http.Transport", trusted.Transport)
			}
			assertTLSFloor(t, trustedTr, tt.insecure)

			publicTr := requirePublicTransport(t, NewPublicOnlyHTTPClient(cfg).Transport)
			assertTLSFloor(t, publicTr, tt.insecure)

			rtTr := requirePublicTransport(t, NewPublicOnlyRoundTripper(cfg))
			assertTLSFloor(t, rtTr, tt.insecure)
		})
	}
}

func assertTLSFloor(t *testing.T, tr *http.Transport, insecure bool) {
	t.Helper()
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want tls.VersionTLS12", tr.TLSClientConfig.MinVersion)
	}
	if tr.TLSClientConfig.InsecureSkipVerify != insecure {
		t.Errorf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, insecure)
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
		{name: "this network /8 start", ip: "0.0.0.0", want: false},
		{name: "this network /8 inside", ip: "0.0.0.1", want: false},
		{name: "this network /8 last", ip: "0.255.255.255", want: false},
		{name: "this network /8 outside", ip: "1.0.0.0", want: true},
		{name: "multicast", ip: "224.0.0.1", want: false},
		{name: "unique local v6", ip: "fd00::1", want: false},
		{name: "link local v6", ip: "fe80::1", want: false},
		{name: "carrier-grade nat", ip: "100.64.0.1", want: false},
		{name: "just outside carrier-grade nat", ip: "100.128.0.1", want: true},
		{name: "benchmark /15 start", ip: "198.18.0.0", want: false},
		{name: "benchmark /15 last", ip: "198.19.255.255", want: false},
		{name: "benchmark /15 outside", ip: "198.20.0.0", want: true},
		{name: "ietf protocol /24 start", ip: "192.0.0.0", want: false},
		{name: "ietf protocol neighbor 8", ip: "192.0.0.8", want: false},
		{name: "pcp anycast exception", ip: "192.0.0.9", want: true},
		{name: "turn anycast exception", ip: "192.0.0.10", want: true},
		{name: "ietf protocol neighbor 11", ip: "192.0.0.11", want: false},
		{name: "ietf protocol /24 last", ip: "192.0.0.255", want: false},
		{name: "ietf protocol /24 outside", ip: "192.0.1.1", want: true},
		{name: "test-net-1 start", ip: "192.0.2.0", want: false},
		{name: "test-net-1 last", ip: "192.0.2.255", want: false},
		{name: "test-net-1 outside", ip: "192.0.3.0", want: true},
		{name: "test-net-2 start", ip: "198.51.100.0", want: false},
		{name: "test-net-2 last", ip: "198.51.100.255", want: false},
		{name: "test-net-2 outside", ip: "198.51.101.0", want: true},
		{name: "test-net-3 start", ip: "203.0.113.0", want: false},
		{name: "test-net-3 last", ip: "203.0.113.255", want: false},
		{name: "test-net-3 outside", ip: "203.0.114.0", want: true},
		{name: "reserved v4 /4 start", ip: "240.0.0.0", want: false},
		{name: "reserved v4 /4 last", ip: "255.255.255.255", want: false},
		{name: "reserved v4 /4 outside", ip: "223.255.255.255", want: true},
		{name: "local-use nat64 start", ip: "64:ff9b:1::", want: false},
		{name: "local-use nat64 last", ip: "64:ff9b:1:ffff:ffff:ffff:ffff:ffff", want: false},
		{name: "local-use nat64 outside", ip: "64:ff9b:2::", want: true},
		{name: "documentation v6 start", ip: "2001:db8::", want: false},
		{name: "documentation v6 last", ip: "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff", want: false},
		{name: "documentation v6 outside", ip: "2001:db9::", want: true},
		{name: "ipv4-mapped metadata service", ip: "::ffff:169.254.169.254", want: false},
		{name: "ipv4-mapped loopback", ip: "::ffff:127.0.0.1", want: false},
		{name: "ipv4-mapped public", ip: "::ffff:93.184.216.34", want: true},
		{name: "ipv4-mapped pcp anycast", ip: "::ffff:192.0.0.9", want: true},
		{name: "ipv4-mapped turn anycast", ip: "::ffff:192.0.0.10", want: true},
		{name: "ipv4-mapped ietf neighbor", ip: "::ffff:192.0.0.8", want: false},
		{name: "ipv4-mapped test-net-1", ip: "::ffff:192.0.2.1", want: false},
		{name: "ipv4-mapped this network", ip: "::ffff:0.1.2.3", want: false},
		{name: "nat64 metadata service", ip: "64:ff9b::a9fe:a9fe", want: false},
		{name: "nat64 loopback", ip: "64:ff9b::7f00:1", want: false},
		{name: "nat64 public address", ip: "64:ff9b::5db8:d822", want: true},
		{name: "nat64 pcp anycast", ip: "64:ff9b::c000:9", want: true},
		{name: "nat64 test-net-1", ip: "64:ff9b::c000:201", want: false},
		{name: "local-use nat64 would-be public embed", ip: "64:ff9b:1::5db8:d822", want: false},
		{name: "local-use nat64 would-be pcp embed", ip: "64:ff9b:1::c000:9", want: false},
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
		{name: "nat64 pcp anycast", ip: "64:ff9b::c000:9", want: true},
		{name: "nat64 test-net-1", ip: "64:ff9b::c000:201", want: false},
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

func TestLocalUseNAT64IsNotUnwrapped(t *testing.T) {
	t.Parallel()

	ip, err := netip.ParseAddr("64:ff9b:1::5db8:d822")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if isPublicIP(ip) {
		t.Fatal("local-use NAT64 must be denied wholesale")
	}
	got := effectiveAddr(ip)
	if got.Is4() {
		t.Fatalf("local-use NAT64 unwrap of %s = %s, want IPv6", ip, got)
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
		{name: "this network", address: "0.1.2.3:80", wantErr: true},
		{name: "benchmark", address: "198.18.0.1:443", wantErr: true},
		{name: "ietf protocol", address: "192.0.0.1:443", wantErr: true},
		{name: "pcp anycast", address: "192.0.0.9:443"},
		{name: "turn anycast", address: "192.0.0.10:443"},
		{name: "test-net-1", address: "192.0.2.1:443", wantErr: true},
		{name: "reserved v4", address: "240.0.0.1:443", wantErr: true},
		{name: "documentation v6", address: "[2001:db8::1]:443", wantErr: true},
		{name: "local-use nat64", address: "[64:ff9b:1::1]:443", wantErr: true},
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
		"0.1.2.3:80",
		"192.0.2.1:443",
		"198.18.0.1:443",
		"240.0.0.1:443",
		"[2001:db8::1]:443",
		"[64:ff9b:1::1]:443",
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
		"192.0.2.1:9",
		"[2001:db8::1]:9",
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
	publicTr := HTTPTransport(public.Transport)
	if publicTr == nil {
		t.Fatalf("public-only transport: got %T, want *http.Transport or public-only wrapper", public.Transport)
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
		tr := HTTPTransport(rt)
		if tr == nil {
			t.Fatalf("transport: got %T, want *http.Transport or public-only wrapper", rt)
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
	if shared.MinVersion != 0 {
		t.Error("pre-existing TLS config MinVersion was mutated")
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
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Error("constructed client must set the TLS 1.2 floor on its own TLS config")
	}
	if tr.TLSClientConfig.ServerName != "ocm.example" {
		t.Error("cloned TLS config should keep ServerName")
	}
}

func TestPublicOnlySchemePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawURL        string
		allowLoopback bool
		redirected    bool
		wantAllowed   bool
	}{
		{name: "https allowed", rawURL: "https://example.com/", wantAllowed: true},
		{name: "public http rejected", rawURL: "http://93.184.216.34/"},
		{name: "literal loopback http without opt-in", rawURL: "http://127.0.0.1/"},
		{name: "literal loopback http opt-in", rawURL: "http://127.0.0.1/", allowLoopback: true, wantAllowed: true},
		{name: "literal loopback v6 http opt-in", rawURL: "http://[::1]/", allowLoopback: true, wantAllowed: true},
		{name: "loopback opt-in does not allow public http", rawURL: "http://93.184.216.34/", allowLoopback: true, wantAllowed: false},
		{name: "hostname localhost rejected", rawURL: "http://localhost/", allowLoopback: true},
		{name: "hostname loopback.local rejected", rawURL: "http://loopback.local/", allowLoopback: true},
		{name: "unknown scheme rejected", rawURL: "ftp://127.0.0.1/", allowLoopback: true},
		{
			name:          "https-to-http redirect rejected even to loopback",
			rawURL:        "http://127.0.0.1/",
			allowLoopback: true,
			redirected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := mustRequest(t, tt.rawURL)
			if tt.redirected {
				prior := mustRequest(t, "https://example.com/")
				req.Response = &http.Response{StatusCode: http.StatusFound, Request: prior}
			}
			err := checkRequestScheme(req, tt.allowLoopback)
			if tt.wantAllowed {
				if err != nil {
					t.Fatalf("checkRequestScheme() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrPolicyViolation) {
				t.Fatalf("checkRequestScheme() error = %v, want errors.Is ErrPolicyViolation", err)
			}
		})
	}
}

func TestNewPublicOnlyRoundTripperSchemePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawURL        string
		allowLoopback bool
		redirected    bool
		wantPolicyErr bool
		wantDial      bool
	}{
		{name: "https-only production", rawURL: "https://example.com/", wantDial: true},
		{name: "production http rejected", rawURL: "http://example.com/", wantPolicyErr: true},
		{name: "public http rejected", rawURL: "http://93.184.216.34/", wantPolicyErr: true},
		{name: "loopback http opt-in initial", rawURL: "http://127.0.0.1/", allowLoopback: true, wantDial: true},
		{
			name:          "loopback http rejected when redirected",
			rawURL:        "http://127.0.0.1/",
			allowLoopback: true,
			redirected:    true,
			wantPolicyErr: true,
		},
		{name: "hostname localhost rejected", rawURL: "http://localhost/", allowLoopback: true, wantPolicyErr: true},
		{name: "unknown scheme rejected before dial", rawURL: "ftp://127.0.0.1/", wantPolicyErr: true},
		{name: "gopher scheme rejected before dial", rawURL: "gopher://127.0.0.1/", wantPolicyErr: true},
		{name: "file scheme rejected before dial", rawURL: "file:///etc/hosts", wantPolicyErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := NewPublicOnlyRoundTripper(TransportConfig{
				Timeout:       time.Second,
				AllowLoopback: tt.allowLoopback,
			})
			tr := requirePublicTransport(t, rt)
			probe := installFakeDial(tr)

			req := mustRequest(t, tt.rawURL)
			if tt.redirected {
				prior := mustRequest(t, "https://example.com/")
				req.Response = &http.Response{StatusCode: http.StatusFound, Request: prior}
			}
			_, err := rt.RoundTrip(req)
			if err == nil {
				t.Fatal("RoundTrip() error = nil, want dial or policy error")
			}
			if tt.wantPolicyErr != errors.Is(err, ErrPolicyViolation) {
				t.Errorf("errors.Is(ErrPolicyViolation) = %v, want %v (err=%v)",
					errors.Is(err, ErrPolicyViolation), tt.wantPolicyErr, err)
			}
			if tt.wantDial != (probe.n > 0) {
				t.Errorf("dial attempts = %d, wantDial %v", probe.n, tt.wantDial)
			}
			if !tt.wantDial && probe.n != 0 {
				t.Errorf("denied request still dialed (%d times)", probe.n)
			}
		})
	}
}

func TestNewPublicOnlyHTTPClientSchemePolicy(t *testing.T) {
	t.Parallel()

	c := NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second})
	tr := requirePublicTransport(t, c.Transport)
	probe := installFakeDial(tr)

	req := mustRequest(t, "https://example.com/")
	_, err := c.Transport.RoundTrip(req)
	if errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("https RoundTrip() error = %v, must not be a scheme violation", err)
	}
	if probe.n == 0 {
		t.Fatal("https must use the default transport path")
	}

	probe.n = 0
	req = mustRequest(t, "http://93.184.216.34/")
	_, err = c.Transport.RoundTrip(req)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("http RoundTrip() error = %v, want ErrPolicyViolation", err)
	}
	if probe.n != 0 {
		t.Fatalf("rejected http still dialed (%d times)", probe.n)
	}
}

func TestHTTPTransportContract(t *testing.T) {
	t.Parallel()

	cfg := TransportConfig{
		Timeout:  time.Second,
		Insecure: true,
	}

	t.Run("public-only client", func(t *testing.T) {
		t.Parallel()
		c := NewPublicOnlyHTTPClient(cfg)
		pt, ok := c.Transport.(*publicOnlyTransport)
		if !ok {
			t.Fatalf("transport type = %T, want *publicOnlyTransport", c.Transport)
		}
		got := HTTPTransport(c.Transport)
		if got != pt.base {
			t.Fatalf("HTTPTransport() = %p, want guarded base %p", got, pt.base)
		}
	})

	t.Run("public-only round tripper", func(t *testing.T) {
		t.Parallel()
		rt := NewPublicOnlyRoundTripper(cfg)
		pt, ok := rt.(*publicOnlyTransport)
		if !ok {
			t.Fatalf("transport type = %T, want *publicOnlyTransport", rt)
		}
		got := HTTPTransport(rt)
		if got != pt.base {
			t.Fatalf("HTTPTransport() = %p, want guarded base %p", got, pt.base)
		}
	})

	t.Run("trusted client", func(t *testing.T) {
		t.Parallel()
		c := NewTrustedHTTPClient(cfg)
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("transport type = %T, want *http.Transport", c.Transport)
		}
		got := HTTPTransport(c.Transport)
		if got != tr {
			t.Fatalf("HTTPTransport() = %p, want the transport itself %p", got, tr)
		}
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		if got := HTTPTransport(nil); got != nil {
			t.Fatalf("HTTPTransport(nil) = %v, want nil", got)
		}
	})

	t.Run("stub round tripper", func(t *testing.T) {
		t.Parallel()
		if got := HTTPTransport(&stubRoundTripper{}); got != nil {
			t.Fatalf("HTTPTransport(stub) = %v, want nil", got)
		}
	})
}

func TestPublicOnlyCheckRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		allowLoopback bool
		rawURL        string
		viaHops       int
		wantErr       bool
	}{
		{name: "https hostname allowed", rawURL: "https://example.com/next", viaHops: 1},
		{name: "http redirect rejected", rawURL: "http://127.0.0.1/next", viaHops: 1, wantErr: true},
		{
			name:          "http redirect to loopback rejected",
			rawURL:        "http://127.0.0.1/next",
			allowLoopback: true,
			viaHops:       1,
			wantErr:       true,
		},
		{name: "private https ip rejected", rawURL: "https://192.168.1.1/", viaHops: 1, wantErr: true},
		{name: "loopback https ip rejected", rawURL: "https://127.0.0.1/", viaHops: 1, wantErr: true},
		{name: "loopback https ip opt-in", rawURL: "https://127.0.0.1/", allowLoopback: true, viaHops: 1},
		{name: "public https ip allowed", rawURL: "https://93.184.216.34/", viaHops: 1},
		{name: "pcp anycast https allowed", rawURL: "https://192.0.0.9/", viaHops: 1},
		{name: "test-net https rejected", rawURL: "https://192.0.2.1/", viaHops: 1, wantErr: true},
		{name: "nine hops allowed", rawURL: "https://example.com/next", viaHops: 9},
		{name: "ten hops rejected", rawURL: "https://example.com/next", viaHops: 10, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := NewPublicOnlyHTTPClient(TransportConfig{
				Timeout:       time.Second,
				AllowLoopback: tt.allowLoopback,
			})
			if c.CheckRedirect == nil {
				t.Fatal("NewPublicOnlyHTTPClient must install CheckRedirect")
			}
			req := mustRequest(t, tt.rawURL)
			via := make([]*http.Request, tt.viaHops)
			for i := range via {
				via[i] = mustRequest(t, "https://example.com/")
			}
			err := c.CheckRedirect(req, via)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckRedirect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrPolicyViolation) {
				t.Errorf("CheckRedirect() error = %v, want errors.Is ErrPolicyViolation", err)
			}
		})
	}
}

func TestPublicOnlyCIDRSchemeAndRedirect(t *testing.T) {
	t.Parallel()

	cidrs := mustFederationCIDRs(t, "10.50.0.0/16", "fd42:8c6d:7a10:23::/64")
	for _, insecure := range []bool{false, true} {
		t.Run(fmt.Sprintf("insecure=%t", insecure), func(t *testing.T) {
			t.Parallel()
			cfg := TransportConfig{
				Timeout:                time.Second,
				Insecure:               insecure,
				AllowLoopback:          false,
				AllowedFederationCIDRs: cidrs,
			}
			c := NewPublicOnlyHTTPClient(cfg)
			rt := NewPublicOnlyRoundTripper(cfg)
			assertTLSFloor(t, requirePublicTransport(t, c.Transport), insecure)
			assertTLSFloor(t, requirePublicTransport(t, rt), insecure)

			httpTargets := []string{
				"http://10.50.1.1/",
				"http://[fd42:8c6d:7a10:23::1]/",
				"http://127.0.0.1/",
				"http://93.184.216.34/",
			}
			for _, raw := range httpTargets {
				assertRoundTripSchemeDenied(t, cfg, raw, false)
				assertRoundTripSchemeDenied(t, cfg, raw, true)
			}

			loopCfg := cfg
			loopCfg.AllowLoopback = true
			assertRoundTripSchemeDenied(t, loopCfg, "http://10.50.1.1/", false)
			assertRoundTripSchemeDenied(t, loopCfg, "http://127.0.0.1/", true)

			allowed := []string{
				"https://10.50.1.1/",
				"https://10.50.0.0/",
				"https://10.50.255.255/",
				"https://[fd42:8c6d:7a10:23::1]/",
				"https://[::ffff:10.50.1.1]/",
				"https://[64:ff9b::a32:101]/",
				"https://192.0.0.9/",
				"https://192.0.0.10/",
				"https://rebind.ocm.test/next",
			}
			denied := []string{
				"http://10.50.1.1/next",
				"https://10.51.1.1/",
				"https://192.168.1.1/",
				"https://[fd00::1]/",
				"https://[::ffff:192.168.1.1]/",
				"https://[64:ff9b::c0a8:101]/",
				"https://[64:ff9b:1::1]/",
				"https://127.0.0.1/",
				"https://[fd42:8c6d:7a10:23::1%25eth0]/",
				"https://[::ffff:10.50.1.1%25eth0]/",
			}
			for _, raw := range allowed {
				assertClientRedirect(t, c, raw, 1, false)
			}
			for _, raw := range denied {
				assertClientRedirect(t, c, raw, 1, true)
			}
			assertClientRedirect(t, c, "https://10.50.1.1/next", 10, true)
			assertClientRedirect(t, c, "https://example.com/next", 9, false)

			// Sentinel stops before connect. The same cfg builds the client dialer.
			assertDialSentinel(t, newPublicOnlyDialer(cfg), "10.50.1.1:443", "10.51.1.1:443")
			assertDialSentinel(t, newPublicOnlyDialer(cfg), "[fd42:8c6d:7a10:23::1]:443", "[fd00::1]:443")
			assertDialSentinel(t, newPublicOnlyDialer(cfg), "[::ffff:10.50.1.1]:443", "[::ffff:192.168.1.1]:443")
			assertDialSentinel(t, newPublicOnlyDialer(cfg), "[64:ff9b::a32:101]:443", "[64:ff9b:1::1]:443")
			assertGuardDenied(t, c.Transport, "192.168.1.1:9")
			assertGuardDenied(t, rt, "10.51.1.1:9")
			assertGuardDenied(t, c.Transport, "[fd00::1]:9")
			assertGuardDenied(t, rt, "[fd42:8c6d:7a10:23::1%eth0]:9")
			assertGuardDenied(t, c.Transport, "127.0.0.1:9")
			assertGuardDenied(t, rt, "[64:ff9b::c0a8:101]:9")

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tr := requirePublicTransport(t, c.Transport)
			conn, err := tr.DialContext(ctx, "tcp", "10.50.1.1:443")
			if conn != nil {
				_ = conn.Close()
				t.Fatal("cancelled dial returned a connection")
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled dial = %v, want context.Canceled", err)
			}
		})
	}
}

func TestCIDRLiteralRedirectAgreesWithDial(t *testing.T) {
	t.Parallel()

	cidrs := mustFederationCIDRs(t, "10.50.0.0/16", "fd42:8c6d:7a10:23::/64")
	cfg := TransportConfig{
		Timeout:                time.Second,
		AllowLoopback:          false,
		AllowedFederationCIDRs: cidrs,
	}

	t.Run("in-range literal is followed until the redirect limit", func(t *testing.T) {
		t.Parallel()
		stub := &redirectStub{status: http.StatusFound, location: "https://10.50.1.1/next"}
		c := NewPublicOnlyHTTPClient(cfg)
		c.Transport = stub
		_, err := c.Do(mustRequest(t, "https://example.com/"))
		if !errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("Do() error = %v, want ErrPolicyViolation", err)
		}
		if strings.Contains(err.Error(), "non-public address") {
			t.Fatalf("in-range redirect error = %v, want the hop limit", err)
		}
		if stub.trips != 10 {
			t.Errorf("trips = %d, want 10", stub.trips)
		}
	})

	t.Run("out-of-range literal stops before a second request", func(t *testing.T) {
		t.Parallel()
		stub := &redirectStub{status: http.StatusFound, location: "https://192.168.1.1/next"}
		c := NewPublicOnlyHTTPClient(cfg)
		c.Transport = stub
		_, err := c.Do(mustRequest(t, "https://example.com/"))
		if !errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("Do() error = %v, want ErrPolicyViolation", err)
		}
		if !strings.Contains(err.Error(), "non-public address") {
			t.Fatalf("out-of-range redirect error = %q, want non-public address", err)
		}
		if stub.trips != 1 {
			t.Errorf("trips = %d, want 1", stub.trips)
		}
	})

	t.Run("http redirect inside a CIDR stops", func(t *testing.T) {
		t.Parallel()
		stub := &redirectStub{status: http.StatusFound, location: "http://10.50.1.1/next"}
		c := NewPublicOnlyHTTPClient(cfg)
		c.Transport = stub
		_, err := c.Do(mustRequest(t, "https://example.com/"))
		if !errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("Do() error = %v, want ErrPolicyViolation", err)
		}
		if !strings.Contains(err.Error(), "refusing non-https redirect") {
			t.Fatalf("http redirect error = %q, want refusing non-https redirect", err)
		}
		if stub.trips != 1 {
			t.Errorf("trips = %d, want 1", stub.trips)
		}
	})

	t.Run("captured policy ignores later config mutation", func(t *testing.T) {
		t.Parallel()
		local := mustFederationCIDRs(t, "10.50.0.0/16")
		localCfg := TransportConfig{
			Timeout:                time.Second,
			AllowedFederationCIDRs: local,
		}
		c := NewPublicOnlyHTTPClient(localCfg)
		dialer := newPublicOnlyDialer(localCfg)
		localCfg.AllowedFederationCIDRs = mustFederationCIDRs(t, "192.168.0.0/16")
		local.prefixes[0] = netip.MustParsePrefix("11.0.0.0/8")

		assertClientRedirect(t, c, "https://10.50.1.1/", 1, false)
		assertClientRedirect(t, c, "https://192.168.1.1/", 1, true)
		assertDialSentinel(t, dialer, "10.50.1.1:443", "192.168.1.1:443")
		assertGuardDenied(t, c.Transport, "192.168.1.1:9")
	})
}

func TestPublicOnlyHostnameDialUsesNumericAddress(t *testing.T) {
	t.Parallel()

	cidrs := mustFederationCIDRs(t, "10.50.0.0/16", "fd42:8c6d:7a10:23::/64")
	dns := startFlipDNS(t, netip.MustParseAddr("10.50.1.1"))
	dialer := newPublicOnlyDialer(TransportConfig{
		Timeout:                time.Second,
		AllowLoopback:          false,
		AllowedFederationCIDRs: cidrs,
	})
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial:     dns.dialContext,
	}
	sentinel := errors.New("hostname numeric dial sentinel")
	var mu sync.Mutex
	seen := []string{}
	orig := dialer.Control
	if orig == nil {
		t.Fatal("production dialer must install Control")
	}
	dialer.Control = func(network, address string, c syscall.RawConn) error {
		mu.Lock()
		seen = append(seen, address)
		mu.Unlock()
		if err := orig(network, address, c); err != nil {
			return err
		}
		return sentinel
	}

	const host = "rebind.ocm.test:443"
	assertHostnameDial(t, dialer, host, netip.MustParseAddr("10.50.1.1"), sentinel, &mu, &seen)

	dns.set(netip.MustParseAddr("192.168.9.9"))
	assertHostnameDial(t, dialer, host, netip.MustParseAddr("192.168.9.9"), nil, &mu, &seen)

	dns.set(netip.MustParseAddr("fd42:8c6d:7a10:23::5"))
	assertHostnameDial(t, dialer, host, netip.MustParseAddr("fd42:8c6d:7a10:23::5"), sentinel, &mu, &seen)

	dns.set(netip.MustParseAddr("fd00::9"))
	assertHostnameDial(t, dialer, host, netip.MustParseAddr("fd00::9"), nil, &mu, &seen)
}

func TestPublicOnlyRedirectDowngradeViaClient(t *testing.T) {
	t.Parallel()

	stub := &redirectStub{status: http.StatusFound, location: "http://127.0.0.1/"}
	c := NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second, AllowLoopback: true})
	c.Transport = stub

	req := mustRequest(t, "https://example.com/")
	_, err := c.Do(req)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("Do() error = %v, want errors.Is ErrPolicyViolation", err)
	}
	if stub.trips != 1 {
		t.Errorf("trips = %d, want 1 (redirect must not be followed)", stub.trips)
	}
}

func TestPublicOnlyRedirectLimitViaClient(t *testing.T) {
	t.Parallel()

	stub := &redirectStub{status: http.StatusFound, location: "https://example.com/next"}
	c := NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second})
	c.Transport = stub

	req := mustRequest(t, "https://example.com/")
	_, err := c.Do(req)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("Do() error = %v, want errors.Is ErrPolicyViolation", err)
	}
	if stub.trips != 10 {
		t.Errorf("trips = %d, want 10 (stop before the eleventh request)", stub.trips)
	}
}

func TestPublicOnlyRedirectKeepsDefaultHeaderFiltering(t *testing.T) {
	t.Parallel()

	rec := &recordingTripper{
		respond: func(_ *http.Request, n int) *http.Response {
			if n == 0 {
				h := make(http.Header)
				h.Set("Location", "https://other.example/")
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     h,
					Body:       http.NoBody,
				}
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}
		},
	}
	c := NewPublicOnlyHTTPClient(TransportConfig{Timeout: time.Second})
	c.Transport = rec

	req := mustRequest(t, "https://first.example/")
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	_ = resp.Body.Close()
	if len(rec.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(rec.reqs))
	}
	if rec.reqs[1].Header.Get("Authorization") != "" {
		t.Fatal("redirected request must not copy Authorization across hosts")
	}
}

func TestTrustedClientRetainsSchemeAndProxyBehavior(t *testing.T) {
	t.Parallel()

	c := NewTrustedHTTPClient(TransportConfig{Timeout: time.Second})
	if _, ok := c.Transport.(*http.Transport); !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport (unwrapped)", c.Transport)
	}
	if c.CheckRedirect != nil {
		t.Fatal("trusted client must keep Go default redirect behavior")
	}

	stub := &stubRoundTripper{}
	c.Transport = stub
	req := mustRequest(t, "http://example.com/")
	if _, err := c.Do(req); err != nil {
		t.Fatalf("trusted client http Do() error = %v", err)
	}
	if stub.trips != 1 {
		t.Errorf("trusted client inner trips = %d, want 1", stub.trips)
	}

	rec := &recordingTripper{
		respond: func(_ *http.Request, n int) *http.Response {
			if n == 0 {
				h := make(http.Header)
				h.Set("Location", "http://example.com/next")
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     h,
					Body:       http.NoBody,
				}
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}
		},
	}
	c = NewTrustedHTTPClient(TransportConfig{Timeout: time.Second})
	c.Transport = rec
	req = mustRequest(t, "https://example.com/")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("trusted client redirect Do() error = %v", err)
	}
	_ = resp.Body.Close()
	if len(rec.reqs) != 2 {
		t.Errorf("trusted client must follow http redirect, requests = %d", len(rec.reqs))
	}
	if rec.reqs[1].URL.Scheme != "http" {
		t.Errorf("trusted follow-up scheme = %q, want http", rec.reqs[1].URL.Scheme)
	}
}

func dialThrough(t *testing.T, rt http.RoundTripper, address string) error {
	t.Helper()
	tr := requirePublicTransport(t, rt)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

func requirePublicTransport(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	tr := HTTPTransport(rt)
	if tr == nil {
		t.Fatalf("got %T, want *http.Transport or public-only wrapper", rt)
	}
	return tr
}

var errFakeDial = errors.New("fake dial")

type dialProbe struct {
	n int
}

func installFakeDial(tr *http.Transport) *dialProbe {
	d := &dialProbe{}
	tr.DialContext = func(context.Context, string, string) (net.Conn, error) {
		d.n++
		return nil, errFakeDial
	}
	return d
}

func mustRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest(%q): %v", rawURL, err)
	}
	return req
}

type stubRoundTripper struct {
	trips int
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.trips++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

type redirectStub struct {
	status   int
	location string
	trips    int
}

func (r *redirectStub) RoundTrip(req *http.Request) (*http.Response, error) {
	r.trips++
	h := make(http.Header)
	if r.location != "" {
		h.Set("Location", r.location)
	}
	return &http.Response{
		StatusCode: r.status,
		Header:     h,
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

type recordingTripper struct {
	reqs    []*http.Request
	respond func(*http.Request, int) *http.Response
}

func (r *recordingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	r.reqs = append(r.reqs, cloned)
	resp := r.respond(req, len(r.reqs)-1)
	resp.Request = req
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
	return resp, nil
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
	tr := requirePublicTransport(t, rt)
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

func assertRoundTripSchemeDenied(t *testing.T, cfg TransportConfig, rawURL string, redirected bool) {
	t.Helper()
	roundTrippers := []http.RoundTripper{
		NewPublicOnlyHTTPClient(cfg).Transport,
		NewPublicOnlyRoundTripper(cfg),
	}
	for _, rt := range roundTrippers {
		tr := requirePublicTransport(t, rt)
		probe := installFakeDial(tr)
		req := mustRequest(t, rawURL)
		if redirected {
			prior := mustRequest(t, "https://example.com/")
			req.Response = &http.Response{StatusCode: http.StatusFound, Request: prior}
		}
		_, err := rt.RoundTrip(req)
		if !errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("RoundTrip(%q redirected=%t) = %v, want ErrPolicyViolation", rawURL, redirected, err)
		}
		if probe.n != 0 {
			t.Fatalf("RoundTrip(%q redirected=%t) dialed %d times", rawURL, redirected, probe.n)
		}
	}
}

func assertClientRedirect(t *testing.T, c *http.Client, rawURL string, viaHops int, wantErr bool) {
	t.Helper()
	if c.CheckRedirect == nil {
		t.Fatal("client must install CheckRedirect")
	}
	req := mustRequest(t, rawURL)
	via := make([]*http.Request, viaHops)
	for i := range via {
		via[i] = mustRequest(t, "https://example.com/")
	}
	err := c.CheckRedirect(req, via)
	if (err != nil) != wantErr {
		t.Fatalf("CheckRedirect(%q) error = %v, wantErr %v", rawURL, err, wantErr)
	}
	if wantErr && !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("CheckRedirect(%q) error = %v, want ErrPolicyViolation", rawURL, err)
	}
}

func assertGuardDenied(t *testing.T, rt http.RoundTripper, address string) {
	t.Helper()
	err := dialThrough(t, rt, address)
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("dial %q = %v, want ErrPolicyViolation", address, err)
	}
}

func assertHostnameDial(
	t *testing.T,
	dialer *net.Dialer,
	host string,
	want netip.Addr,
	sentinel error,
	mu *sync.Mutex,
	seen *[]string,
) {
	t.Helper()
	mu.Lock()
	*seen = []string{}
	mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if conn != nil {
		_ = conn.Close()
		t.Fatalf("dial %q returned a connection", host)
	}
	mu.Lock()
	got := append([]string{}, *seen...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatalf("dial %q recorded no connect address (err=%v)", host, err)
	}
	for _, address := range got {
		ipHost, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			t.Fatalf("connect address %q: %v", address, splitErr)
		}
		ip, parseErr := netip.ParseAddr(ipHost)
		if parseErr != nil {
			t.Fatalf("connect address %q: %v", address, parseErr)
		}
		if ip.Compare(want) != 0 {
			t.Fatalf("connect address %q, want %s", address, want)
		}
	}
	if sentinel != nil {
		if !errors.Is(err, sentinel) {
			t.Fatalf("in-range hostname dial = %v, want sentinel", err)
		}
		if errors.Is(err, ErrPolicyViolation) {
			t.Fatalf("in-range hostname dial = %v, want no policy violation", err)
		}
		return
	}
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("out-of-range hostname dial = %v, want ErrPolicyViolation", err)
	}
}

type flipDNS struct {
	pc net.PacketConn
	mu sync.Mutex
	ip netip.Addr
}

func startFlipDNS(t *testing.T, initial netip.Addr) *flipDNS {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &flipDNS{pc: pc, ip: initial}
	t.Cleanup(func() { _ = pc.Close() })
	go s.serve()
	return s
}

func (s *flipDNS) set(ip netip.Addr) {
	s.mu.Lock()
	s.ip = ip
	s.mu.Unlock()
}

func (s *flipDNS) current() netip.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ip
}

func (s *flipDNS) dialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	var d net.Dialer
	switch network {
	case "tcp", "tcp4", "tcp6":
		return nil, errors.New("test dns is udp only")
	default:
		return d.DialContext(ctx, "udp", s.pc.LocalAddr().String())
	}
}

func (s *flipDNS) serve() {
	buf := make([]byte, 1500)
	for {
		n, addr, err := s.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		resp := dnsAnswer(buf[:n], s.current())
		if resp == nil {
			continue
		}
		_, _ = s.pc.WriteTo(resp, addr)
	}
}

func dnsAnswer(query []byte, ip netip.Addr) []byte {
	if len(query) < 12 {
		return nil
	}
	qEnd, qtype, ok := dnsQuestion(query)
	if !ok {
		return nil
	}
	resp := make([]byte, qEnd)
	copy(resp, query[:qEnd])
	flags := binary.BigEndian.Uint16(resp[2:4])
	flags |= 0x8000 | 0x0400
	flags &^= 0x0200
	flags &^= 0x000F
	binary.BigEndian.PutUint16(resp[2:4], flags)
	binary.BigEndian.PutUint16(resp[4:6], 1)
	binary.BigEndian.PutUint16(resp[6:8], 0)
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)

	var rdata []byte
	var typ uint16
	switch {
	case qtype == 1 && ip.Is4():
		typ = 1
		b := ip.As4()
		rdata = b[:]
	case qtype == 28 && ip.Is6():
		typ = 28
		b := ip.As16()
		rdata = b[:]
	default:
		return resp
	}
	binary.BigEndian.PutUint16(resp[6:8], 1)
	ans := make([]byte, 12+len(rdata))
	ans[0], ans[1] = 0xC0, 0x0C
	binary.BigEndian.PutUint16(ans[2:4], typ)
	binary.BigEndian.PutUint16(ans[4:6], 1)
	binary.BigEndian.PutUint16(ans[10:12], uint16(len(rdata)))
	copy(ans[12:], rdata)
	return append(resp, ans...)
}

func dnsQuestion(query []byte) (int, uint16, bool) {
	off, ok := skipDNSName(query, 12)
	if !ok || off+4 > len(query) {
		return 0, 0, false
	}
	qtype := binary.BigEndian.Uint16(query[off : off+2])
	return off + 4, qtype, true
}

func skipDNSName(msg []byte, off int) (int, bool) {
	for off < len(msg) {
		n := int(msg[off])
		if n == 0 {
			return off + 1, true
		}
		if n&0xC0 == 0xC0 {
			if off+1 >= len(msg) {
				return 0, false
			}
			return off + 2, true
		}
		if n&0xC0 != 0 {
			return 0, false
		}
		off += 1 + n
	}
	return 0, false
}
