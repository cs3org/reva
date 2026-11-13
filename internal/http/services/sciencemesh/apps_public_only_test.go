// Copyright 2018-2024 CERN
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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
)

type recordingLaunchDiscovery struct {
	*ocmd.OCMClient
	endpoint string
	err      error
}

func (c *recordingLaunchDiscovery) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	c.endpoint = endpoint
	disco, err := c.OCMClient.Discover(ctx, endpoint)
	c.err = err
	return disco, err
}

func TestWebappLaunchPublicOnlyRejectsBeforeContact(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	acceptedDone := make(chan struct{})
	var accepted atomic.Int32
	go func() {
		defer close(acceptedDone)
		for {
			conn, accErr := ln.Accept()
			if accErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-acceptedDone
		if accepted.Load() != 0 {
			t.Errorf("destination accepted %d connections", accepted.Load())
		}
	})
	port := ln.Addr().(*net.TCPAddr).Port

	tests := []struct {
		name       string
		webdav     string
		insecure   bool
		wantRefuse bool
	}{
		{
			name:       "loopback discovery",
			webdav:     fmt.Sprintf("https://127.0.0.1:%d/dav", port),
			insecure:   false,
			wantRefuse: true,
		},
		{
			name:       "loopback discovery with insecure tls",
			webdav:     fmt.Sprintf("https://127.0.0.1:%d/dav", port),
			insecure:   true,
			wantRefuse: true,
		},
		{
			name:       "private discovery",
			webdav:     "https://10.2.3.4/dav",
			wantRefuse: true,
		},
		{
			name:       "carrier grade nat discovery",
			webdav:     "https://100.64.0.1/dav",
			wantRefuse: true,
		},
		{
			name:       "link-local discovery",
			webdav:     "https://169.254.169.254/dav",
			wantRefuse: true,
		},
		{
			name:       "ipv6 link-local discovery",
			webdav:     "https://[fe80::1]/dav",
			wantRefuse: true,
		},
		{
			name:       "ipv6 loopback discovery",
			webdav:     "https://[::1]/dav",
			wantRefuse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := accepted.Load()
			share := receivedWebappShare(
				tt.webdav,
				"https://app.example/hub",
				launchSecret,
				[]string{"must-exchange-token"},
			)
			h := newTestHandlerWithConfig(t, &fakeReceivedGateway{resp: okShareResponse(share)}, &config{
				OCMMountPoint:     "/ocm",
				ProviderDomain:    "receiver.example",
				OCMClientTimeout:  1,
				OCMClientInsecure: tt.insecure,
			}, nil)
			real, ok := h.launchClient.(*ocmd.OCMClient)
			if !ok {
				t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
			}
			discovery := &recordingLaunchDiscovery{OCMClient: real}
			h.launchClient = discovery
			req, logs := newLaunchRequest(t, "/ocm/share-1")
			rec := httptest.NewRecorder()
			h.WebappLaunch(rec, req)

			if rec.Code == http.StatusOK {
				t.Fatalf("non-public launch succeeded: %s", rec.Body.String())
			}
			body := rec.Body.String()
			if discovery.endpoint != webdavHost(tt.webdav) {
				t.Fatalf("discovery input = %q, want %q", discovery.endpoint, webdavHost(tt.webdav))
			}
			if tt.wantRefuse && !errors.Is(discovery.err, client.ErrPolicyViolation) {
				t.Fatalf("dial guard did not refuse: body %s log %s", body, logs.String())
			}
			if accepted.Load() != before {
				t.Fatalf("destination accepted %d connections", accepted.Load()-before)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestWebappLaunchPublicDiscoveryPrivateToken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	acceptedDone := make(chan struct{})
	var accepted atomic.Int32
	go func() {
		defer close(acceptedDone)
		for {
			conn, accErr := ln.Accept()
			if accErr != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-acceptedDone
		if accepted.Load() != 0 {
			t.Errorf("token endpoint accepted %d connections", accepted.Load())
		}
	})
	port := ln.Addr().(*net.TCPAddr).Port
	tokenURL := fmt.Sprintf("https://127.0.0.1:%d/ocm/token", port)

	share := receivedWebappShare(
		"https://192.0.2.1/remote.php/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandlerWithConfig(t, &fakeReceivedGateway{resp: okShareResponse(share)}, &config{
		OCMMountPoint:     "/ocm",
		ProviderDomain:    "receiver.example",
		OCMClientTimeout:  1,
		OCMClientInsecure: true,
	}, nil)
	real, ok := h.launchClient.(*ocmd.OCMClient)
	if !ok {
		t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
	}
	h.launchClient = &publicOnlyScript{
		real:          real,
		tokenEndpoint: tokenURL,
	}

	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)

	// The private token endpoint is non-public, so the public-only dialer
	// refuses the exchange before any connection. The launch error is
	// redacted to a generic "launch failed"; the dialer's non-public
	// message never reaches the response.
	assertLaunchRefused(t, rec, logs.String(), http.StatusInternalServerError, "launch failed")
	if accepted.Load() != 0 {
		t.Fatalf("token endpoint accepted %d connections", accepted.Load())
	}
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestWebappLaunchPublicOnlyIgnoresProxy(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)

	share := receivedWebappShare(
		"https://10.9.8.7/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandlerWithConfig(t, &fakeReceivedGateway{resp: okShareResponse(share)}, &config{
		OCMMountPoint:     "/ocm",
		ProviderDomain:    "receiver.example",
		OCMClientTimeout:  1,
		OCMClientInsecure: true,
	}, nil)
	real, ok := h.launchClient.(*ocmd.OCMClient)
	if !ok {
		t.Fatalf("launchClient = %T, want *ocmd.OCMClient", h.launchClient)
	}
	discovery := &recordingLaunchDiscovery{OCMClient: real}
	h.launchClient = discovery
	tr := client.HTTPTransport(real.Transport())
	if tr == nil || tr.DialContext == nil || tr.Proxy != nil {
		t.Fatal("launch transport must retain its guarded dialer and nil proxy")
	}
	var addresses sync.Map
	dial := tr.DialContext
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		addresses.Store(address, true)
		return dial(ctx, network, address)
	}
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.WebappLaunch(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("proxied launch succeeded")
	}
	if hits.Load() != 0 {
		t.Fatalf("proxy received %d requests", hits.Load())
	}
	if discovery.endpoint != "10.9.8.7" || !errors.Is(discovery.err, client.ErrPolicyViolation) {
		t.Fatalf("discovery did not report a policy refusal: %s", logs.String())
	}
	if _, ok := addresses.Load("10.9.8.7:443"); !ok {
		t.Fatal("dial did not target the share host")
	}
	if _, ok := addresses.Load(proxy.Listener.Addr().String()); ok {
		t.Fatal("dial targeted the proxy")
	}
}
