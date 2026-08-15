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

package open

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

func TestNewRejectsHatch(t *testing.T) {
	t.Parallel()

	t.Run("allow_http", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"insecure": true,
			"ocm_client_security": map[string]any{
				"allow_http": true,
			},
		})
		if err == nil {
			t.Fatal("expected open.New to reject allow_http")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowHTTP) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowHTTP", err)
		}
	})

	t.Run("allowed_cidrs", func(t *testing.T) {
		_, err := New(context.Background(), map[string]any{
			"insecure": true,
			"ocm_client_security": map[string]any{
				"allowed_cidrs": []string{"10.0.0.0/8"},
			},
		})
		if err == nil {
			t.Fatal("expected open.New to reject allowed_cidrs")
		}
		if !errors.Is(err, ocmd.ErrHatchAllowedCIDRs) {
			t.Fatalf("got %v, want ocmd.ErrHatchAllowedCIDRs", err)
		}
	})
}

func newOpenAuthorizer(t *testing.T) *authorizer {
	t.Helper()
	got, err := New(context.Background(), map[string]any{"insecure": true})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := got.(*authorizer)
	if !ok {
		t.Fatalf("got %T, want *authorizer", got)
	}
	return a
}

func firstPublicIPv4() (string, bool) {
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

func startPublicTLSServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ip, ok := firstPublicIPv4()
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

// hostDialTrace records hosts the HTTP client actually connected to.
type hostDialTrace struct {
	mu          sync.Mutex
	established []string
}

func contextWithHostDialTrace(ctx context.Context) (context.Context, *hostDialTrace) {
	tr := &hostDialTrace{established: []string{}}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
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

func (tr *hostDialTrace) establishedHost(host string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for _, addr := range tr.established {
		if hostOf(addr) == host {
			return true
		}
	}
	return false
}

func assertNoEstablishedHost(t *testing.T, tr *hostDialTrace, host string) {
	t.Helper()
	if tr.establishedHost(host) {
		t.Fatalf("public-only client established a connection to %s", host)
	}
}

func TestGetInfoByDomainUsesPublicOnlyClient(t *testing.T) {
	var srv *httptest.Server
	fetched := false
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":    true,
			"apiVersion": "1.2.0",
			"endPoint":   srv.URL + "/ocm",
			"provider":   "loopback",
			"resourceTypes": []any{
				map[string]any{
					"name":      "file",
					"protocols": map[string]any{"webdav": "/remote.php/dav/ocm"},
				},
			},
		})
	}))
	defer srv.Close()

	a := newOpenAuthorizer(t)
	_, err := a.GetInfoByDomain(context.Background(), srv.URL)
	if fetched {
		t.Fatal("GetInfoByDomain fetched a loopback discovery host; the public-only client must refuse it at dial")
	}
	if err == nil {
		t.Fatal("GetInfoByDomain must refuse body-supplied discovery to a private loopback host")
	}
}

func TestGetInfoByDomainRefusesPrivateAndMetadataHosts(t *testing.T) {
	a := newOpenAuthorizer(t)

	tests := []struct {
		name   string
		domain string
		host   string
	}{
		{name: "cloud metadata service", domain: "169.254.169.254", host: "169.254.169.254"},
		{name: "loopback", domain: "127.0.0.1", host: "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, tr := contextWithHostDialTrace(context.Background())
			_, err := a.GetInfoByDomain(ctx, tt.domain)
			assertNoEstablishedHost(t, tr, tt.host)
			if err == nil {
				t.Fatal("expected body-supplied discovery to a private or metadata host to fail")
			}
		})
	}

	t.Run("reachable private host is not contacted", func(t *testing.T) {
		fetched := false
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":    true,
				"apiVersion": "1.2.0",
				"endPoint":   "https://127.0.0.1/ocm",
				"provider":   "loopback",
			})
		}))
		defer srv.Close()

		loopHost, _, err := net.SplitHostPort(srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		ctx, tr := contextWithHostDialTrace(context.Background())
		_, err = a.GetInfoByDomain(ctx, srv.URL)
		if fetched {
			t.Fatal("GetInfoByDomain fetched a private discovery host; the public-only client must refuse it at dial")
		}
		assertNoEstablishedHost(t, tr, loopHost)
		if err == nil {
			t.Fatal("expected body-supplied discovery to a private host to fail")
		}
	})
}

func TestGetInfoByDomainPublicHTTPSHostSucceeds(t *testing.T) {
	var srv *httptest.Server
	srv = startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/ocm" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":    true,
			"apiVersion": "1.2.0",
			"endPoint":   srv.URL + "/ocm",
			"provider":   "public-host",
			"resourceTypes": []any{
				map[string]any{
					"name":      "file",
					"protocols": map[string]any{"webdav": "/remote.php/dav/ocm"},
				},
			},
		})
	}))

	a := newOpenAuthorizer(t)
	info, err := a.GetInfoByDomain(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("GetInfoByDomain() on a public HTTPS host: %v", err)
	}
	if info == nil || info.FullName != "public-host" {
		t.Fatalf("unexpected provider info: %+v", info)
	}
}
