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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newDiscoverHandler(t *testing.T, limit int) *wayfHandler {
	t.Helper()
	h := new(wayfHandler)
	err := h.init(&config{
		OCMClientTimeout:       5,
		OCMClientInsecure:      true,
		OCMClientResponseLimit: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func postDiscover(t *testing.T, h *wayfHandler, domain string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(DiscoverRequest{Domain: domain})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/discover", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.DiscoverProvider(rec, req)
	return rec
}

// httptest servers bind loopback, which the public-only dial guard refuses.
// Keep https-only, the redirect cap, and Proxy=nil; only drop Control so the
// TLS fixture is reachable.
func allowUntrustedLoopback(t *testing.T, h *wayfHandler) {
	t.Helper()
	tr, ok := h.untrustedClient.Transport.(*publicOnlyTransport)
	if !ok {
		t.Fatalf("untrusted transport: got %T, want *publicOnlyTransport", h.untrustedClient.Transport)
	}
	if tr.Transport == nil {
		t.Fatal("untrusted client is missing an inner *http.Transport")
	}
	tr.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
}

// dialPublicHostToListener keeps the public-address dial check, then connects
// to the httptest listener so a public URL can exercise HTTPS redirects.
func dialPublicHostToListener(t *testing.T, h *wayfHandler, listenerAddr string) {
	t.Helper()
	tr, ok := h.untrustedClient.Transport.(*publicOnlyTransport)
	if !ok {
		t.Fatalf("untrusted transport: got %T, want *publicOnlyTransport", h.untrustedClient.Transport)
	}
	if tr.Transport == nil {
		t.Fatal("untrusted client is missing an inner *http.Transport")
	}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if err := refuseNonPublicAddr(network, address, nil); err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, listenerAddr)
	}
}

func discoveryJSON(t *testing.T, invite string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"enabled":            true,
		"apiVersion":         "1.1",
		"inviteAcceptDialog": invite,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestDiscoverProviderUntrustedClient(t *testing.T) {
	t.Run("public https host succeeds with a bounded read", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/ocm" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(discoveryJSON(t, "/accept"))
		}))
		defer srv.Close()

		h := newDiscoverHandler(t, 4096)
		allowUntrustedLoopback(t, h)

		rec := postDiscover(t, h, srv.URL)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
		}

		var got DiscoverResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.InviteAcceptDialog == "" {
			t.Fatal("expected inviteAcceptDialog in the discovery response")
		}
		if !strings.HasSuffix(got.InviteAcceptDialog, "/accept") {
			t.Fatalf("inviteAcceptDialog = %q, want a URL ending in /accept", got.InviteAcceptDialog)
		}
	})

	t.Run("private metadata host is refused at dial", func(t *testing.T) {
		fetched := false
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(discoveryJSON(t, "/accept"))
		}))
		defer srv.Close()

		h := newDiscoverHandler(t, 1<<20)

		// Loopback httptest is a private address; the dial guard must refuse
		// before the handler runs.
		rec := postDiscover(t, h, srv.URL)
		if fetched {
			t.Error("untrusted /discover reached a private httptest host")
		}
		if rec.Code == http.StatusOK {
			t.Fatalf("status = %d, want a refusal for a private host; body = %s", rec.Code, rec.Body.String())
		}

		rec = postDiscover(t, h, "https://169.254.169.254")
		if rec.Code == http.StatusOK {
			t.Fatalf("status = %d, want a refusal for the metadata address", rec.Code)
		}
	})

	t.Run("http host is refused", func(t *testing.T) {
		fetched := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(discoveryJSON(t, "/accept"))
		}))
		defer srv.Close()

		h := newDiscoverHandler(t, 1<<20)
		rec := postDiscover(t, h, srv.URL)
		if fetched {
			t.Error("untrusted /discover fetched a non-https URL")
		}
		if rec.Code == http.StatusOK {
			t.Fatalf("status = %d, want a refusal for http; body = %s", rec.Code, rec.Body.String())
		}
	})

	// 1.1.1.1 is a public address, so a refusal cannot be the private-dial
	// guard. The https check runs in RoundTrip before Dial.
	t.Run("http public host is refused by https guard", func(t *testing.T) {
		h := newDiscoverHandler(t, 1<<20)
		_, err := discoverUntrusted(
			context.Background(),
			h.untrustedClient,
			"http://1.1.1.1",
			1<<20,
		)
		if err == nil {
			t.Fatal("expected https refusal for a public http URL")
		}
		if !errors.Is(err, errUntrustedNonHTTPS) {
			t.Fatalf("err = %v, want errUntrustedNonHTTPS", err)
		}
	})

	t.Run("more than 3 https redirects are refused", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/next", http.StatusFound)
		}))
		defer srv.Close()

		h := newDiscoverHandler(t, 1<<20)
		dialPublicHostToListener(t, h, srv.Listener.Addr().String())

		_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		// TEST-NET-3 is a public documentation address; the dial helper
		// still runs refuseNonPublicAddr on it, then connects to httptest.
		publicURL := "https://203.0.113.1:" + port
		_, err = discoverUntrusted(
			context.Background(),
			h.untrustedClient,
			publicURL,
			1<<20,
		)
		if err == nil {
			t.Fatal("expected redirect-cap refusal")
		}
		if !errors.Is(err, errUntrustedTooManyRedirects) {
			t.Fatalf("err = %v, want errUntrustedTooManyRedirects", err)
		}
	})

	t.Run("response over the configured limit is rejected", func(t *testing.T) {
		const limit = 64
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			payload := map[string]any{
				"enabled":            true,
				"apiVersion":         "1.1",
				"inviteAcceptDialog": "/accept",
				"provider":           strings.Repeat("a", 2048),
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Error(err)
				http.Error(w, "marshal", http.StatusInternalServerError)
				return
			}
			if len(body) <= limit {
				t.Errorf("fixture body is %d bytes, want more than limit %d", len(body), limit)
			}
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		h := newDiscoverHandler(t, limit)
		allowUntrustedLoopback(t, h)

		rec := postDiscover(t, h, srv.URL)
		if rec.Code == http.StatusOK {
			t.Fatalf("oversized discovery body must be rejected; body = %s", rec.Body.String())
		}
	})
}

func TestUntrustedDiscoverClientHardening(t *testing.T) {
	h := newDiscoverHandler(t, 1<<20)
	if h.untrustedClient == nil {
		t.Fatal("untrustedClient is nil")
	}
	if h.untrustedClient.Timeout != 5*time.Second {
		t.Errorf("untrusted timeout = %v, want 5s", h.untrustedClient.Timeout)
	}

	tr, ok := h.untrustedClient.Transport.(*publicOnlyTransport)
	if !ok {
		t.Fatalf("transport: got %T, want *publicOnlyTransport", h.untrustedClient.Transport)
	}
	if tr.Proxy != nil {
		t.Error("untrusted client must not use a proxy")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want at least TLS 1.2", tr.TLSClientConfig.MinVersion)
	}
	if h.untrustedClient.CheckRedirect == nil {
		t.Error("CheckRedirect is nil")
	}

	// newDiscoverHandler sets OCMClientInsecure so the TLS fixture works.
	// Construct the secure path separately to assert verification stays on.
	secure := new(wayfHandler)
	if err := secure.init(&config{
		OCMClientTimeout:       5,
		OCMClientInsecure:      false,
		OCMClientResponseLimit: 1 << 20,
	}); err != nil {
		t.Fatal(err)
	}
	if secure.untrustedClient == nil {
		t.Fatal("secure untrustedClient is nil")
	}
	if secure.untrustedClient.CheckRedirect == nil {
		t.Error("secure CheckRedirect is nil")
	}
	secureTr, ok := secure.untrustedClient.Transport.(*publicOnlyTransport)
	if !ok {
		t.Fatalf("secure transport: got %T, want *publicOnlyTransport", secure.untrustedClient.Transport)
	}
	if secureTr.TLSClientConfig == nil {
		t.Fatal("secure TLSClientConfig is nil")
	}
	if secureTr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify must be false when OCMClientInsecure is false")
	}
}
