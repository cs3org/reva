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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

func TestOpenInAppPublicOnlyRejectsBeforeContact(t *testing.T) {
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
		wantScheme bool
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
			name:       "http loopback is scheme rejection",
			webdav:     fmt.Sprintf("http://127.0.0.1:%d/dav", port),
			wantScheme: true,
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
			h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
			h.clientTimeout = time.Second
			h.clientInsecure = tt.insecure
			req, logs := newLaunchRequest(t, "/ocm/share-1")
			rec := httptest.NewRecorder()
			h.OpenInApp(rec, req)

			if rec.Code == http.StatusOK {
				t.Fatalf("non-public launch succeeded: %s", rec.Body.String())
			}
			body := rec.Body.String()
			// Discover replaces a refused dial with a generic discovery error.
			// The dial guard is visible in the request log, before any accept.
			if tt.wantRefuse {
				if !strings.Contains(logs.String(), "non-public") && !strings.Contains(logs.String(), "refusing") {
					t.Fatalf("dial guard did not refuse: body %s log %s", body, logs.String())
				}
			}
			if tt.wantScheme {
				if strings.Contains(body, "refusing") || strings.Contains(body, "non-public") ||
					strings.Contains(logs.String(), "refusing") || strings.Contains(logs.String(), "non-public") {
					t.Fatal("scheme rejection was reported as address rejection")
				}
				if !strings.Contains(body, "https") {
					t.Fatalf("body %s", body)
				}
			}
			if accepted.Load() != before {
				t.Fatalf("destination accepted %d connections", accepted.Load()-before)
			}
			assertNotLeaked(t, body, logs.String(), launchSecret, launchToken)
		})
	}
}

func TestOpenInAppPublicDiscoveryPrivateToken(t *testing.T) {
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
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = true
	var gotTimeout time.Duration
	var gotInsecure bool
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		gotTimeout = timeout
		gotInsecure = insecure
		return &publicOnlyScript{
			real:          ocmd.NewPublicOnlyClient(timeout, insecure),
			tokenEndpoint: tokenURL,
		}
	}

	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("private token hop succeeded: %s", rec.Body.String())
	}
	// The private token endpoint is non-public, so the public-only dialer
	// refuses the exchange before any connection. The launch error is
	// redacted to a generic "launch failed" body; the dialer's non-public
	// message never reaches the response.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "launch failed") {
		t.Fatalf("body %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "app_url") || strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("launch payload leaked: %s", rec.Body.String())
	}
	if accepted.Load() != 0 {
		t.Fatalf("token endpoint accepted %d connections", accepted.Load())
	}
	if gotTimeout != time.Second || !gotInsecure {
		t.Fatalf("timeout %s insecure %v", gotTimeout, gotInsecure)
	}
	assertNotLeaked(t, rec.Body.String(), logs.String(), launchSecret, launchToken)
}

func TestOpenInAppPublicOnlyScriptRefusesPrivateDiscovery(t *testing.T) {
	share := receivedWebappShare(
		"https://10.9.8.7/dav",
		"https://app.example/hub",
		launchSecret,
		[]string{"must-exchange-token"},
	)
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = false
	script := &publicOnlyScript{
		real:          ocmd.NewPublicOnlyClient(time.Second, false),
		tokenEndpoint: "https://token.example/ocm/token",
	}
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		if timeout != time.Second || insecure {
			t.Errorf("timeout %s insecure %v", timeout, insecure)
		}
		return script
	}
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("private discovery succeeded")
	}
	if script.discoverCalls != 1 || script.exchangeCalls != 0 {
		t.Fatalf("discover %d exchange %d", script.discoverCalls, script.exchangeCalls)
	}
	if !strings.Contains(logs.String(), "non-public") && !strings.Contains(logs.String(), "refusing") {
		t.Fatalf("body %s log %s", rec.Body.String(), logs.String())
	}
}

func TestOpenInAppPublicOnlyIgnoresProxy(t *testing.T) {
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
	h := newTestHandler(t, &fakeReceivedGateway{resp: okShareResponse(share)}, nil)
	h.clientTimeout = time.Second
	h.clientInsecure = true
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	rec := httptest.NewRecorder()
	h.OpenInApp(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("proxied launch succeeded")
	}
	if hits.Load() != 0 {
		t.Fatalf("proxy received %d requests", hits.Load())
	}
	// A configured proxy would dial the proxy address. Proxy=nil dials the target.
	// The request URL omits the default port, so ":443" is the dial address.
	if !strings.Contains(logs.String(), "10.9.8.7:443") {
		t.Fatalf("dial did not target the share host: %s", logs.String())
	}
	if strings.Contains(logs.String(), proxy.Listener.Addr().String()) {
		t.Fatalf("dial targeted the proxy: %s", logs.String())
	}
}
