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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

func innerUntrustedHTTPTransport(t *testing.T, rt http.RoundTripper) *http.Transport {
	t.Helper()
	tr := untrustedInnerTransport(rt)
	if tr == nil {
		t.Fatalf("untrusted transport: got %T, want an *http.Transport wrapper", rt)
	}
	return tr
}

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

// httptest servers bind loopback, which the untrusted dial guard refuses
// when the hatch is empty. Keep the https gate, redirect cap, and Proxy=nil;
// only drop Control so the TLS fixture is reachable.
func allowUntrustedLoopback(t *testing.T, h *wayfHandler) {
	t.Helper()
	tr := innerUntrustedHTTPTransport(t, h.untrustedClient.HTTPTransport())
	tr.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
}

// refuseNonPublicTestAddr is the test-only public-address check used when a
// public URL is redirected onto the httptest listener.
func refuseNonPublicTestAddr(_, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() {
		return fmt.Errorf("refusing non-public address %s", address)
	}
	return nil
}

// dialPublicHostToListener keeps the public-address dial check, then connects
// to the httptest listener so a public URL can exercise HTTPS redirects.
func dialPublicHostToListener(t *testing.T, h *wayfHandler, listenerAddr string) {
	t.Helper()
	tr := innerUntrustedHTTPTransport(t, h.untrustedClient.HTTPTransport())
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if err := refuseNonPublicTestAddr(network, address); err != nil {
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
		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"http://1.1.1.1/",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.untrustedClient.HTTPTransport().RoundTrip(req)
		if err == nil {
			t.Fatal("expected https refusal for a public http URL")
		}
		if !errors.Is(err, ocmd.ErrNonHTTPS) {
			t.Fatalf("err = %v, want ocmd.ErrNonHTTPS", err)
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
		// still runs refuseNonPublicTestAddr on it, then connects to httptest.
		publicURL := "https://203.0.113.1:" + port
		_, err = h.untrustedClient.Discover(context.Background(), publicURL)
		if err == nil {
			t.Fatal("expected redirect-cap refusal")
		}

		req, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			publicURL+"/.well-known/ocm",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{
			Transport:     h.untrustedClient.HTTPTransport(),
			Timeout:       h.untrustedClient.RequestTimeout(),
			CheckRedirect: ocmd.PublicOnlyCheckRedirect,
		}).Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			t.Fatal("expected redirect-cap refusal from the shared transport")
		}
		if !errors.Is(err, ocmd.ErrTooManyRedirects) {
			t.Fatalf("err = %v, want ocmd.ErrTooManyRedirects", err)
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
	if h.untrustedClient.RequestTimeout() != 5*time.Second {
		t.Errorf("untrusted timeout = %v, want 5s", h.untrustedClient.RequestTimeout())
	}
	if _, ok := h.untrustedClient.HTTPTransport().(*http.Transport); ok {
		t.Fatal("untrusted discover client must use the shared public-only transport")
	}
	if h.ocmClient == nil {
		t.Fatal("trusted ocmClient is nil")
	}
	if _, ok := h.ocmClient.HTTPTransport().(*http.Transport); !ok {
		t.Fatalf("trusted WAYF client transport = %T, want *http.Transport", h.ocmClient.HTTPTransport())
	}

	tr := innerUntrustedHTTPTransport(t, h.untrustedClient.HTTPTransport())
	if tr.Proxy != nil {
		t.Error("untrusted client must not use a proxy")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want at least TLS 1.2", tr.TLSClientConfig.MinVersion)
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
	secureTr := innerUntrustedHTTPTransport(t, secure.untrustedClient.HTTPTransport())
	if secureTr.TLSClientConfig == nil {
		t.Fatal("secure TLSClientConfig is nil")
	}
	if secureTr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify must be false when OCMClientInsecure is false")
	}
}

func TestWayfUntrustedTLSMinVersion(t *testing.T) {
	t.Parallel()

	h := new(wayfHandler)
	if err := h.init(&config{
		OCMClientTimeout:       5,
		OCMClientInsecure:      true,
		OCMClientResponseLimit: 1 << 20,
		ocmClientTLSMin:        tls.VersionTLS13,
	}); err != nil {
		t.Fatal(err)
	}
	if h.ocmClient == nil {
		t.Fatal("trusted ocmClient is nil")
	}
	tr := innerUntrustedHTTPTransport(t, h.untrustedClient.HTTPTransport())
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("untrusted MinVersion = %v, want TLS 1.3", tr.TLSClientConfig)
	}
	trustedTr, ok := h.ocmClient.HTTPTransport().(*http.Transport)
	if !ok {
		t.Fatalf("trusted WAYF client transport = %T, want *http.Transport", h.ocmClient.HTTPTransport())
	}
	if trustedTr.TLSClientConfig == nil || trustedTr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("trusted MinVersion = %v, want TLS 1.2", trustedTr.TLSClientConfig)
	}
}

func TestUntrustedDiscoverClientSplitsDialAndRequestTimeout(t *testing.T) {
	h := new(wayfHandler)
	if err := h.init(&config{
		OCMClientTimeout:       3,
		OCMClientDialTimeout:   1,
		OCMClientInsecure:      true,
		OCMClientResponseLimit: 1 << 20,
	}); err != nil {
		t.Fatal(err)
	}
	if h.untrustedClient == nil {
		t.Fatal("untrustedClient is nil")
	}
	if h.untrustedClient.RequestTimeout() != 3*time.Second {
		t.Errorf("untrusted request timeout = %v, want 3s", h.untrustedClient.RequestTimeout())
	}
	if h.ocmClient == nil || h.ocmClient.RequestTimeout() != 3*time.Second {
		t.Errorf("trusted directory client timeout = %v, want 3s", h.ocmClient.RequestTimeout())
	}

	tr := innerUntrustedHTTPTransport(t, h.untrustedClient.HTTPTransport())
	if tr.DialContext == nil {
		t.Fatal("untrusted DialContext is nil; dial-timeout override did not install")
	}
	wrapper := runtime.FuncForPC(reflect.ValueOf(tr.DialContext).Pointer()).Name()
	if !strings.Contains(wrapper, "overridePublicOnlyDialTimeout") {
		t.Fatalf("DialContext = %s, want overridePublicOnlyDialTimeout wrapper", wrapper)
	}

	start := time.Now()
	_, err := h.untrustedClient.Discover(context.Background(), "https://240.0.0.1")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected discover to a non-responsive public address to fail")
	}
	// Discover falls back to a second path. Each dial is capped at 1s, so a
	// working override finishes before the 3s request deadline. A silent
	// no-op keeps the 3s NewPublicOnlyClient dial/request timeout and cannot
	// pass this bound.
	if elapsed >= 3*time.Second {
		t.Fatalf("dial timeout was not applied to the untrusted transport; elapsed %v, want < 3s request deadline", elapsed)
	}
}
