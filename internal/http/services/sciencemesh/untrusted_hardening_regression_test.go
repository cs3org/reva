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
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

// TestUntrustedHardeningRegression proves sciencemesh.New wires the
// shared public-only discover client. Per-host matrices stay in
// wayf_test.go.
func TestUntrustedHardeningRegression(t *testing.T) {
	m := requiredScienceMeshConfig()
	m["ocm_client_insecure"] = true
	got, err := New(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := got.(*svc)
	if !ok {
		t.Fatalf("got %T, want *svc", got)
	}
	if s.wayf == nil || s.wayf.untrustedClient == nil {
		t.Fatal("sciencemesh.New must construct the untrusted discover client")
	}
	c := s.wayf.untrustedClient

	t.Run("HTTP is refused", func(t *testing.T) {
		fetched := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		resp, err := roundTripConstructor(t, c, srv.URL)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if fetched {
			t.Fatal("sciencemesh.New client must not fetch HTTP")
		}
		if !errors.Is(err, ocmd.ErrNonHTTPS) {
			t.Fatalf("got %v, want ocmd.ErrNonHTTPS", err)
		}
	})

	t.Run("metadata host is refused", func(t *testing.T) {
		ctx, tr := contextWithPeerDialTrace(context.Background())
		resp, err := roundTripConstructorCtx(t, c, ctx, "https://169.254.169.254/")
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		assertDialAttemptedHost(t, tr, "169.254.169.254")
		assertNoEstablishedHost(t, tr, "169.254.169.254")
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})

	t.Run("loopback peer is refused before contact", func(t *testing.T) {
		contacted := false
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contacted = true
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		ctx, tr := contextWithPeerDialTrace(context.Background())
		resp, err := roundTripConstructorCtx(t, c, ctx, srv.URL)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if contacted {
			t.Fatal("sciencemesh.New client contacted a loopback peer; public-only guard must refuse before contact")
		}
		host, _, splitErr := net.SplitHostPort(srv.Listener.Addr().String())
		if splitErr != nil {
			t.Fatal(splitErr)
		}
		assertDialAttemptedHost(t, tr, host)
		assertNoEstablishedHost(t, tr, host)
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})

	t.Run("public HTTPS discovery succeeds", func(t *testing.T) {
		var srv *httptest.Server
		srv = startScienceMeshPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/ocm" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":            true,
				"apiVersion":         "1.1",
				"inviteAcceptDialog": "/accept",
			})
		}))

		disco, err := c.Discover(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("Discover on a public HTTPS host: %v", err)
		}
		if disco == nil || disco.InviteAcceptDialog != "/accept" {
			t.Fatalf("unexpected discovery payload: %+v", disco)
		}
	})
}

func startScienceMeshPublicTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ip, ok := firstScienceMeshPublicIPv4()
	if !ok {
		t.Skip("no public IPv4 address available for public-only httptest")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skipf("cannot listen on public IP %s: %v", ip, err)
	}
	srv := httptest.NewUnstartedServer(h)
	if srv.Listener != nil {
		_ = srv.Listener.Close()
	}
	srv.Listener = ln
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func firstScienceMeshPublicIPv4() (string, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP == nil {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			continue
		}
		if ip[0] == 100 && ip[1]&0xc0 == 64 {
			continue
		}
		return ip.String(), true
	}
	return "", false
}

func roundTripConstructor(t *testing.T, c *ocmd.OCMClient, rawURL string) (*http.Response, error) {
	t.Helper()
	return roundTripConstructorCtx(t, c, context.Background(), rawURL)
}

func roundTripConstructorCtx(
	t *testing.T,
	c *ocmd.OCMClient,
	ctx context.Context,
	rawURL string,
) (*http.Response, error) {
	t.Helper()
	if c == nil || c.HTTPTransport() == nil {
		t.Fatal("constructor-wired client has no transport")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.HTTPTransport().RoundTrip(req)
}

type peerDialTrace struct {
	mu          sync.Mutex
	attempted   []string
	established []string
}

func contextWithPeerDialTrace(ctx context.Context) (context.Context, *peerDialTrace) {
	tr := &peerDialTrace{attempted: []string{}, established: []string{}}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		ConnectStart: func(_, addr string) {
			tr.mu.Lock()
			tr.attempted = append(tr.attempted, addr)
			tr.mu.Unlock()
		},
		ConnectDone: func(_, addr string, err error) {
			if err != nil {
				return
			}
			tr.mu.Lock()
			tr.established = append(tr.established, addr)
			tr.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Conn == nil {
				return
			}
			tr.mu.Lock()
			tr.established = append(tr.established, info.Conn.RemoteAddr().String())
			tr.mu.Unlock()
		},
	})
	return ctx, tr
}

func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
		return ip.String()
	}
	return host
}

func (tr *peerDialTrace) sawHost(kind string, host string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	list := tr.established
	if kind == "attempted" {
		list = tr.attempted
	}
	for _, addr := range list {
		if hostOf(addr) == host {
			return true
		}
	}
	return false
}

func assertDialAttemptedHost(t *testing.T, tr *peerDialTrace, host string) {
	t.Helper()
	if !tr.sawHost("attempted", host) {
		tr.mu.Lock()
		attempted := append([]string{}, tr.attempted...)
		tr.mu.Unlock()
		t.Fatalf("public-only guard must dial %s before refusing; attempted %v", host, attempted)
	}
}

func assertNoEstablishedHost(t *testing.T, tr *peerDialTrace, host string) {
	t.Helper()
	if tr.sawHost("established", host) {
		t.Fatalf("public-only client established a connection to %s", host)
	}
}
