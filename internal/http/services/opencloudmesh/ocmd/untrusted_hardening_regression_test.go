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
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studio-b12/gowebdav"
)

const (
	regressionPropfindMethod = "PROPFIND"
	regressionMetadataIP     = "169.254.169.254"
	regressionRebindHost     = "rebind.test"
	regressionTestNet3IP     = "203.0.113.1"
)

// TestUntrustedHardeningRegression is the T8 integrated sweep over the
// shared untrusted-client contract. Per-surface constructor proofs live
// next to received.New, open.New, embedded.New, and sciencemesh.New;
// this file stitches the ocmd-owned invariants those constructors share.
func TestUntrustedHardeningRegression(t *testing.T) {
	t.Run("public-only dial guard", testRegressionPublicOnlyDialGuard)
	t.Run("untrusted https redirect tls timeouts", testRegressionUntrustedClientContract)
	t.Run("trusted NewClient TLS isolation", testRegressionTrustedTLSIsolation)
	t.Run("sticky-IP reuse", testRegressionStickyIPReuse)
	t.Run("gowebdav HTTP redirect before dial", testRegressionGowebdavHTTPRedirect)
	t.Run("gowebdav HTTPS 307 redirect cap", testRegressionGowebdavHTTPSRedirectCap)
	t.Run("public HTTPS redirect cap", testRegressionPublicHTTPSRedirectCap)
	t.Run("hatch pin and link-local", testRegressionHatchPinLinkLocal)
	t.Run("response limit and F2 invite", testRegressionResponseLimitInvite)
}

func regressionPublicOnlyClient(timeout time.Duration) *OCMClient {
	return NewPublicOnlyClient(
		timeout,
		true,
		testSec(),
		0,
		0,
	)
}

func regressionWalkerClient(rt http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return nil
		},
	}
}

func testRegressionPublicOnlyDialGuard(t *testing.T) {
	t.Parallel()
	sec := testSec()

	refuse := []struct {
		name string
		addr string
	}{
		{name: "private 10/8", addr: "10.1.2.3:443"},
		{name: "loopback", addr: "127.0.0.1:443"},
		{name: "cgnat", addr: "100.64.0.1:443"},
		{name: "nat64", addr: "[64:ff9b::a9fe:a9fe]:443"},
		{name: "link-local", addr: "169.254.1.1:443"},
		{name: "multicast", addr: "224.0.0.1:443"},
		{name: "metadata service", addr: regressionMetadataIP + ":443"},
	}
	for _, tt := range refuse {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := sec.CheckDialAddr(tt.addr); !errors.Is(err, ErrNonPublicAddr) {
				t.Fatalf("CheckDialAddr(%q) = %v, want ErrNonPublicAddr", tt.addr, err)
			}
		})
	}

	t.Run("public address accepted", func(t *testing.T) {
		t.Parallel()
		if err := sec.CheckDialAddr("8.8.8.8:443"); err != nil {
			t.Fatalf("public CheckDialAddr: %v", err)
		}
	})

	t.Run("live public HTTPS is accepted", func(t *testing.T) {
		srv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		c := regressionPublicOnlyClient(5 * time.Second)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			t.Fatalf("public HTTPS GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})
}

func testRegressionUntrustedClientContract(t *testing.T) {
	t.Parallel()

	t.Run("HTTP is refused", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("untrusted client must not fetch HTTP")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := regressionPublicOnlyClient(5 * time.Second)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		closeResponse(resp)
		if !errors.Is(err, ErrNonHTTPS) {
			t.Fatalf("got %v, want ErrNonHTTPS", err)
		}
	})

	t.Run("TLS 1.2 default and configured 1.3", func(t *testing.T) {
		t.Parallel()
		def := publicOnlyHTTPTransport(t, regressionPublicOnlyClient(5*time.Second))
		if def.TLSClientConfig == nil || def.TLSClientConfig.MinVersion != tls.VersionTLS12 {
			t.Fatalf("default MinVersion = %v, want TLS 1.2", def.TLSClientConfig)
		}
		raised := NewPublicOnlyClient(
			5*time.Second,
			true,
			testSec(),
			0,
			tls.VersionTLS13,
		)
		got := publicOnlyHTTPTransport(t, raised)
		if got.TLSClientConfig == nil || got.TLSClientConfig.MinVersion != tls.VersionTLS13 {
			t.Fatalf("configured MinVersion = %v, want TLS 1.3", got.TLSClientConfig)
		}
	})

	t.Run("below TLS 1.2 is rejected", func(t *testing.T) {
		t.Parallel()
		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("newOCMTransport must reject TLS 1.1")
			}
			msg, ok := got.(string)
			if !ok || !strings.Contains(msg, "below TLS 1.2") {
				t.Fatalf("panic = %v, want TLS 1.2 floor rejection", got)
			}
		}()
		_ = newOCMTransport(false, tls.VersionTLS11)
	})

	t.Run("TLS 1.1 handshake is refused", func(t *testing.T) {
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

		rt := UntrustedHTTPTransport(
			5*time.Second,
			true,
			testSec(),
			0,
		)
		allowUntrustedLoopback(t, rt)
		resp, err := doUntrustedRequest(t, rt, srv.URL)
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected TLS 1.1 handshake refusal")
		}
		assertTLSProtocolVersionError(t, err)
	})

	t.Run("split timeouts are bounded on metadata client", func(t *testing.T) {
		const (
			requestTimeout = 400 * time.Millisecond
			dialTimeout    = 5 * time.Second
			peerSleep      = 500 * time.Millisecond
		)
		started := make(chan struct{})
		srv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			time.Sleep(peerSleep)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))

		sec := testSec()
		c := &http.Client{
			Transport: UntrustedHTTPTransport(
				dialTimeout,
				true,
				sec,
				0,
			),
			Timeout:       requestTimeout,
			CheckRedirect: sec.CheckRedirect,
		}
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		resp, err := c.Do(req)
		elapsed := time.Since(start)
		closeResponse(resp)
		if err == nil {
			t.Fatal("expected the request deadline to fire against a slow peer")
		}
		assertHTTPClientTimeout(t, err)
		if elapsed < requestTimeout/2 {
			t.Fatalf("failed too quickly to be the request deadline: elapsed %v", elapsed)
		}
		if elapsed >= dialTimeout/2 {
			t.Fatalf("elapsed %v looks like the dial deadline, want request timeout %v", elapsed, requestTimeout)
		}
		select {
		case <-started:
		default:
			t.Fatal("expected the slow metadata peer to accept the connection")
		}
	})
}

func testRegressionTrustedTLSIsolation(t *testing.T) {
	t.Parallel()

	trusted := NewClient(5*time.Second, false, 0)
	tr, ok := trusted.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("trusted transport: got %T, want *http.Transport", trusted.client.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("NewClient MinVersion = %v, want TLS 1.2", tr.TLSClientConfig)
	}

	untrusted := NewPublicOnlyClient(
		5*time.Second,
		false,
		testSec(),
		0,
		tls.VersionTLS13,
	)
	got := publicOnlyHTTPTransport(t, untrusted)
	if got.TLSClientConfig == nil || got.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("NewPublicOnlyClient MinVersion = %v, want TLS 1.3", got.TLSClientConfig)
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("NewClient must not inherit the untrusted TLS knob")
	}

	t.Run("trusted NewClient completes TLS 1.2 handshake", func(t *testing.T) {
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		srv.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS12,
		}
		srv.StartTLS()
		t.Cleanup(srv.Close)

		trustedLive := NewClient(5*time.Second, true, 0)
		var negotiated uint16
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			TLSHandshakeDone: func(state tls.ConnectionState, err error) {
				if err != nil {
					return
				}
				negotiated = state.Version
			},
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := trustedLive.client.Do(req)
		if err != nil {
			t.Fatalf("trusted NewClient TLS 1.2 handshake: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if negotiated != tls.VersionTLS12 {
			t.Fatalf("negotiated TLS %#x, want TLS 1.2 (trusted floor must not rise to 1.3)", negotiated)
		}
	})
}

func testRegressionStickyIPReuse(t *testing.T) {
	t.Run("same IP dialed on every redirect hop", func(t *testing.T) {
		runStickyIPRedirectChain(t, false)
	})
	t.Run("Control refuses mid-chain rebind without reuse", func(t *testing.T) {
		runStickyIPRedirectChain(t, true)
	})

	t.Run("metadata IP is refused after a public hop", func(t *testing.T) {
		srv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))

		c := regressionPublicOnlyClient(5 * time.Second)
		tr := publicOnlyHTTPTransport(t, c)
		if tr.DialContext == nil {
			t.Fatal("untrusted transport must set DialContext")
		}

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			srv.URL,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			t.Fatalf("public warmup GET: %v", err)
		}
		resp.Body.Close()

		var established bool
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			GotConn: func(httptrace.GotConnInfo) {
				established = true
			},
		})
		priv, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"https://"+regressionMetadataIP+"/",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		privResp, err := c.client.Do(priv)
		closeResponse(privResp)
		if established {
			t.Fatal("sticky-IP reuse must not connect to 169.254.169.254")
		}
		if !errors.Is(err, ErrNonPublicAddr) {
			t.Fatalf("metadata dial: got %v, want ErrNonPublicAddr", err)
		}
	})
}

// runStickyIPRedirectChain drives a PROPFIND/307 chain through the production
// UntrustedHTTPTransport DialContext (including Control). Test DNS maps
// rebind.test onto the public listener first and a reachable loopback peer
// later. Production Control still sees those resolved IPs.
func runStickyIPRedirectChain(t *testing.T, disableKeepAlive bool) {
	t.Helper()
	const hopRequests = 4
	var (
		mu               sync.Mutex
		hops             int
		methods          = []string{}
		privateContacted bool
	)
	srv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hops++
		n := hops
		methods = append(methods, r.Method)
		mu.Unlock()
		if n < hopRequests {
			http.Redirect(w, r, "/hop", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	priv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		privateContacted = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("rebound"))
	}))
	t.Cleanup(priv.Close)

	publicHost, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	privAddr := priv.Listener.Addr().String()

	c := regressionPublicOnlyClient(5 * time.Second)
	tr := publicOnlyHTTPTransport(t, c)
	if tr.DialContext == nil {
		t.Fatal("untrusted transport must set DialContext")
	}
	if !disableKeepAlive && tr.DisableKeepAlives {
		t.Fatal("sticky-IP reuse requires keep-alive; DisableKeepAlives must stay false")
	}
	if disableKeepAlive {
		tr.DisableKeepAlives = true
	}

	var (
		dialed  = []string{}
		lookups int
		reused  = []bool{}
	)
	inner := tr.DialContext
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			return nil, splitErr
		}
		resolved := address
		if host == regressionRebindHost {
			mu.Lock()
			lookups++
			n := lookups
			mu.Unlock()
			if n == 1 {
				resolved = net.JoinHostPort(publicHost, port)
			} else {
				resolved = privAddr
			}
		}
		mu.Lock()
		dialed = append(dialed, resolved)
		mu.Unlock()
		// Production Control runs on the resolved IP; this wrap only
		// observes and supplies test DNS.
		return inner(ctx, network, resolved)
	}

	startURL := "https://" + net.JoinHostPort(regressionRebindHost, listenerPort(t, srv)) + "/"
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			mu.Lock()
			reused = append(reused, info.Reused)
			mu.Unlock()
		},
	})
	req, err := http.NewRequestWithContext(
		ctx,
		regressionPropfindMethod,
		startURL,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.client.Do(req)
	closeResponse(resp)

	mu.Lock()
	defer mu.Unlock()
	if privateContacted {
		t.Fatal("mid-chain DNS rebind reached the private peer; production Control must refuse it")
	}
	if disableKeepAlive {
		if err == nil {
			t.Fatal("expected production Control to refuse the private rebind when reuse is disabled")
		}
		if !errors.Is(err, ErrNonPublicAddr) {
			t.Fatalf("rebind without reuse: got %v, want ErrNonPublicAddr", err)
		}
		if lookups < 2 {
			t.Fatalf("name lookups = %d, want at least 2 when keep-alive is disabled", lookups)
		}
		return
	}
	if err != nil {
		t.Fatalf("sticky-IP redirect chain: %v", err)
	}
	if hops != hopRequests {
		t.Fatalf("hop requests = %d, want %d PROPFIND/307 hops", hops, hopRequests)
	}
	if len(methods) != hopRequests {
		t.Fatalf("methods = %d, want %d", len(methods), hopRequests)
	}
	for _, m := range methods {
		if m != regressionPropfindMethod {
			t.Fatalf("307 must preserve PROPFIND, got %q", m)
		}
	}
	if lookups != 1 {
		t.Fatalf("name lookups = %d, want 1 (keep-alive must reuse the first resolved IP)", lookups)
	}
	if len(dialed) != 1 {
		t.Fatalf("dials = %d, want 1 reused connection, got %v", len(dialed), dialed)
	}
	gotHost, _, splitErr := net.SplitHostPort(dialed[0])
	if splitErr != nil {
		t.Fatal(splitErr)
	}
	if gotHost != publicHost {
		t.Fatalf("dialed %q, want sticky public IP %q", dialed[0], publicHost)
	}
	if len(reused) != hopRequests {
		t.Fatalf("GotConn events = %d, want %d", len(reused), hopRequests)
	}
	if reused[0] {
		t.Fatal("first hop must open a new connection")
	}
	for i := 1; i < len(reused); i++ {
		if !reused[i] {
			t.Fatalf("hop %d must reuse the first connection; a fresh dial could rebind the IP", i+1)
		}
	}
}

func listenerPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func regressionTestNet3URL(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	if path == "" {
		path = "/"
	}
	return "https://" + net.JoinHostPort(regressionTestNet3IP, listenerPort(t, srv)) + path
}

// installTestNet3Dial remaps TEST-NET-3 onto the local httptest listener so
// public-only clients can exercise HTTPS without binding a real public IP.
func installTestNet3Dial(t *testing.T, tr *http.Transport, listenerAddr string) {
	t.Helper()
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if host != regressionTestNet3IP {
			return nil, errors.New("test dialer expected TEST-NET-3 203.0.113.1")
		}
		ip := net.ParseIP(host)
		if ip == nil || !isPublicIP(ip) {
			return nil, ErrNonPublicAddr
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, listenerAddr)
	}
}

func testRegressionGowebdavHTTPRedirect(t *testing.T) {
	httpFetched := false
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpFetched = true
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	tlsSrv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httpSrv.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))

	dav := gowebdav.NewClient(tlsSrv.URL, "", "")
	dav.SetTransport(UntrustedHTTPTransport(
		5*time.Second,
		true,
		testSec(),
		0,
	))
	_, err := dav.Stat("")
	if httpFetched {
		t.Fatal("gowebdav HTTP redirect must be refused before dialing")
	}
	if err == nil {
		t.Fatal("expected gowebdav HTTP redirect refusal")
	}
	if !errors.Is(err, ErrNonHTTPS) {
		t.Fatalf("got %v, want ErrNonHTTPS", err)
	}
}

func testRegressionGowebdavHTTPSRedirectCap(t *testing.T) {
	const redirectCap = 3
	// gowebdav.NewClient starts with nullAuth, which inhibits the first
	// redirect so it can negotiate auth. The following noAuth Stat is the
	// hop chain the transport walker must cap.
	const authProbeHops = 1

	newDav := func(t *testing.T, srv *httptest.Server) *gowebdav.Client {
		t.Helper()
		sec := UntrustedClientSecurity{MaxRedirects: redirectCap}
		sec.ApplyDefaults()
		if err := sec.Compile(); err != nil {
			t.Fatal(err)
		}
		rt := UntrustedHTTPTransport(
			5*time.Second,
			true,
			sec,
			0,
		)
		installTestNet3Dial(t, innerUntrustedTransport(t, rt), srv.Listener.Addr().String())
		dav := gowebdav.NewClient(regressionTestNet3URL(t, srv, "/"), "", "")
		dav.SetTransport(rt)
		return dav
	}

	t.Run("refuses above configured cap", func(t *testing.T) {
		hops := 0
		methods := []string{}
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			methods = append(methods, r.Method)
			hops++
			http.Redirect(w, r, "/hop", http.StatusTemporaryRedirect)
		}))
		t.Cleanup(srv.Close)

		_, err := newDav(t, srv).Stat("")
		if !errors.Is(err, ErrTooManyRedirects) {
			t.Fatalf("got %v, want ErrTooManyRedirects", err)
		}
		wantHops := authProbeHops + redirectCap + 1
		if hops != wantHops {
			t.Fatalf("hops = %d, want %d (auth probe plus cap+1 HTTPS 307 hops)", hops, wantHops)
		}
		if len(methods) == 0 {
			t.Fatal("expected gowebdav to issue at least one PROPFIND")
		}
		for _, m := range methods {
			if m != regressionPropfindMethod {
				t.Fatalf("307 must preserve PROPFIND, got %q", m)
			}
		}
	})

	t.Run("allows exactly the configured cap", func(t *testing.T) {
		hops := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != regressionPropfindMethod {
				t.Errorf("unexpected method %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			hops++
			followed := hops - authProbeHops
			if hops == authProbeHops || followed <= redirectCap {
				http.Redirect(w, r, "/hop", http.StatusTemporaryRedirect)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(207)
			_, _ = w.Write([]byte(ingestWebDAVPropfindXML))
		}))
		t.Cleanup(srv.Close)

		info, err := newDav(t, srv).Stat("")
		if err != nil {
			t.Fatalf("expected gowebdav to allow %d HTTPS 307 redirects: %v", redirectCap, err)
		}
		if info == nil {
			t.Fatal("expected WebDAV stat info after the capped 307 chain")
		}
		wantHops := authProbeHops + redirectCap + 1
		if hops != wantHops {
			t.Fatalf("hops = %d, want %d (auth probe plus exactly %d HTTPS 307 redirects then 207)", hops, wantHops, redirectCap)
		}
	})
}

func testRegressionPublicHTTPSRedirectCap(t *testing.T) {
	const redirectCap = 3

	newCapWalker := func(t *testing.T, srv *httptest.Server) *http.Client {
		t.Helper()
		sec := UntrustedClientSecurity{MaxRedirects: redirectCap}
		sec.ApplyDefaults()
		if err := sec.Compile(); err != nil {
			t.Fatal(err)
		}
		rt := UntrustedHTTPTransport(5*time.Second, true, sec, 0)
		installTestNet3Dial(t, innerUntrustedTransport(t, rt), srv.Listener.Addr().String())
		return regressionWalkerClient(rt)
	}

	t.Run("refuses above configured cap", func(t *testing.T) {
		hops := 0
		methods := []string{}
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			methods = append(methods, r.Method)
			hops++
			if hops <= redirectCap+1 {
				http.Redirect(w, r, "/hop", http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		req, err := http.NewRequestWithContext(
			context.Background(),
			regressionPropfindMethod,
			regressionTestNet3URL(t, srv, "/"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := newCapWalker(t, srv).Do(req)
		closeResponse(resp)
		if !errors.Is(err, ErrTooManyRedirects) {
			t.Fatalf("got %v, want ErrTooManyRedirects", err)
		}
		if hops != redirectCap+1 {
			t.Fatalf("hops = %d, want %d (cap enforced after %d redirects)", hops, redirectCap+1, redirectCap)
		}
		if len(methods) == 0 {
			t.Fatal("expected the walker to issue at least one PROPFIND")
		}
		for _, m := range methods {
			if m != regressionPropfindMethod {
				t.Fatalf("307 must preserve PROPFIND, got %q", m)
			}
		}
	})

	t.Run("allows exactly the configured cap", func(t *testing.T) {
		hops := 0
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != regressionPropfindMethod {
				t.Errorf("unexpected method %s", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			hops++
			if hops <= redirectCap {
				http.Redirect(w, r, "/hop", http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		t.Cleanup(srv.Close)

		req, err := http.NewRequestWithContext(
			context.Background(),
			regressionPropfindMethod,
			regressionTestNet3URL(t, srv, "/"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := newCapWalker(t, srv).Do(req)
		if err != nil {
			t.Fatalf("expected transport walker to allow %d redirects: %v", redirectCap, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if hops != redirectCap+1 {
			t.Fatalf("hops = %d, want %d (exactly %d redirects then 200)", hops, redirectCap+1, redirectCap)
		}
	})
}

func testRegressionHatchPinLinkLocal(t *testing.T) {
	t.Parallel()

	t.Run("PIN stays exclusive", func(t *testing.T) {
		t.Parallel()
		s := UntrustedClientSecurity{
			AllowedCIDRs:  []string{"10.0.0.0/8"},
			AllowLoopback: true,
		}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if err := s.CheckDialAddr("10.1.2.3:443"); err != nil {
			t.Fatalf("PIN must allow a listed address: %v", err)
		}
		if err := s.CheckDialAddr("8.8.8.8:443"); !errors.Is(err, ErrNonPublicAddr) {
			t.Fatalf("PIN must refuse an unlisted public IP: %v", err)
		}
		if err := s.CheckDialAddr("127.0.0.1:443"); !errors.Is(err, ErrNonPublicAddr) {
			t.Fatalf("PIN must ignore AllowLoopback: %v", err)
		}
	})

	t.Run("link-local hard-deny without exact host", func(t *testing.T) {
		t.Parallel()
		s := UntrustedClientSecurity{AllowedCIDRs: []string{"169.254.0.0/16"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if err := s.CheckDialAddr("169.254.1.1:443"); !errors.Is(err, ErrNonPublicAddr) {
			t.Fatalf("link-local /16 must stay denied: %v", err)
		}

		s = UntrustedClientSecurity{AllowedCIDRs: []string{regressionMetadataIP + "/32"}}
		if err := s.Compile(); err != nil {
			t.Fatal(err)
		}
		if err := s.CheckDialAddr(regressionMetadataIP + ":443"); err != nil {
			t.Fatalf("exact /32 must allow the metadata host: %v", err)
		}
	})
}

func testRegressionResponseLimitInvite(t *testing.T) {
	t.Parallel()

	t.Run("zero and negative select the default", func(t *testing.T) {
		t.Parallel()
		for _, limit := range []int64{0, -1} {
			c := NewPublicOnlyClient(
				time.Second,
				true,
				testSec(),
				limit,
				0,
			)
			if c.responseLimit != maxOCMResponseBytes {
				t.Fatalf("limit %d: responseLimit = %d, want %d", limit, c.responseLimit, maxOCMResponseBytes)
			}
		}
	})

	t.Run("bounded F2 invite response", func(t *testing.T) {
		body := oversizedJSON(t, map[string]any{"email": "a@b.c"}, "name")
		srv := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))

		c := NewPublicOnlyClient(
			5*time.Second,
			true,
			testSec(),
			0,
			0,
		)
		u, err := c.InviteAccepted(context.Background(), srv.URL, &InviteAcceptedRequest{
			UserID: "alice",
			Token:  "tok",
		})
		assertOCMResponseTooLarge(t, err)
		if u != nil {
			t.Fatalf("oversized invite-accepted body must not be decoded, got %+v", u)
		}
	})
}

func assertTLSProtocolVersionError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a TLS protocol-version error")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "tls") || !strings.Contains(msg, "protocol version") {
		t.Fatalf("got %v, want a TLS protocol-version failure (server below the configured floor)", err)
	}
}

func assertHTTPClientTimeout(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected request deadline")
	}
	if strings.Contains(err.Error(), "Client.Timeout") {
		return
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	t.Fatalf("got %v, want request deadline, not a dial failure", err)
}
