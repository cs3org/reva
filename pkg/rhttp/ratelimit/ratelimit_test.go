// Copyright 2018-2025 CERN
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

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHandler(t *testing.T, c Config) http.Handler {
	t.Helper()
	l, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func do(h http.Handler, remote, xff string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/discover", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBurstThenReject(t *testing.T) {
	h := newHandler(t, Config{RequestsPerMinute: 1, Burst: 2})

	for i := 0; i < 2; i++ {
		if got := do(h, "203.0.113.7:1000", "").Code; got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, got)
		}
	}
	rec := do(h, "203.0.113.7:2000", "") // same IP, different port
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if got := do(h, "203.0.113.8:1000", "").Code; got != http.StatusOK {
		t.Fatalf("other client: got %d, want 200", got)
	}
}

func TestForwardedFor(t *testing.T) {
	h := newHandler(t, Config{RequestsPerMinute: 1, Burst: 1, TrustedProxyCIDRs: []string{"10.0.0.0/8"}})

	// Behind a trusted proxy the forwarded client is the bucket key.
	if got := do(h, "10.0.0.1:1", "198.51.100.1").Code; got != http.StatusOK {
		t.Fatalf("got %d, want 200", got)
	}
	if got := do(h, "10.0.0.1:1", "198.51.100.1").Code; got != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", got)
	}
	if got := do(h, "10.0.0.1:1", "198.51.100.2").Code; got != http.StatusOK {
		t.Fatalf("got %d, want 200", got)
	}
	// A client-supplied prefix to the left of the proxy's entry is ignored.
	if got := do(h, "10.0.0.1:1", "192.0.2.99, 198.51.100.1").Code; got != http.StatusTooManyRequests {
		t.Fatalf("spoofed prefix: got %d, want 429", got)
	}
	// An untrusted peer cannot choose its own key via the header.
	if got := do(h, "203.0.113.9:1", "198.51.100.50").Code; got != http.StatusOK {
		t.Fatalf("got %d, want 200", got)
	}
	if got := do(h, "203.0.113.9:1", "198.51.100.51").Code; got != http.StatusTooManyRequests {
		t.Fatalf("header spoofing by untrusted peer: got %d, want 429", got)
	}
}

func TestIPv6SharesPrefixBucket(t *testing.T) {
	h := newHandler(t, Config{RequestsPerMinute: 1, Burst: 1})

	if got := do(h, "[2001:db8:1:2::1]:1", "").Code; got != http.StatusOK {
		t.Fatalf("got %d, want 200", got)
	}
	if got := do(h, "[2001:db8:1:2::ffff]:1", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("same /64: got %d, want 429", got)
	}
}

func TestOverflowBucketBoundsMemory(t *testing.T) {
	l, err := New(Config{RequestsPerMinute: 1, Burst: 1, MaxClients: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	for _, ip := range []string{"192.0.2.1:1", "192.0.2.2:1", "192.0.2.3:1", "192.0.2.4:1"} {
		do(h, ip, "")
	}
	l.mu.Lock()
	n := len(l.clients)
	l.mu.Unlock()
	if n > 2 {
		t.Fatalf("tracked %d clients, want at most 2", n)
	}
}

func TestInvalidConfig(t *testing.T) {
	for name, c := range map[string]Config{
		"zero rate":  {RequestsPerMinute: 0, Burst: 1},
		"zero burst": {RequestsPerMinute: 1, Burst: 0},
		"bad CIDR":   {RequestsPerMinute: 1, Burst: 1, TrustedProxyCIDRs: []string{"10.0.0.0/8", "not-a-cidr"}},
	} {
		if l, err := New(c); err == nil {
			l.Close()
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNilLimiterIsPassthrough(t *testing.T) {
	var l *Limiter
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	if got := do(h, "203.0.113.7:1", "").Code; got != http.StatusTeapot {
		t.Fatalf("got %d, want handler's 418", got)
	}
	l.Close() // must not panic
}
