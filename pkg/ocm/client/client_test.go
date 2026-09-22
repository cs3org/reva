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

	control := refuseNonPublicAddr(false)
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

	control := refuseNonPublicAddr(true)
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

	control := refuseNonPublicAddr(false)
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
