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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func TestNewEndpointNormalisation(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://cernbox.cern.ch", "https://cernbox.cern.ch"},
		{"cernbox.cern.ch", "https://cernbox.cern.ch"},
		{"https://cernbox.cern.ch/", "https://cernbox.cern.ch"},
		{"http://localhost:8080", "http://localhost:8080"},
	}
	for _, tt := range tests {
		c, err := New(tt.in)
		if err != nil {
			t.Fatalf("New(%q): %v", tt.in, err)
		}
		if got := c.Endpoint(); got != tt.want {
			t.Errorf("New(%q).Endpoint() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewRejectsBadEndpoint(t *testing.T) {
	for _, in := range []string{"", "https://"} {
		if _, err := New(in); err == nil {
			t.Errorf("New(%q) should fail", in)
		} else if cberr.KindOf(err) != cberr.KindUsage {
			t.Errorf("New(%q) should be a usage error, got kind %v", in, cberr.KindOf(err))
		}
	}
}

func TestCredentialIsAttached(t *testing.T) {
	f := newFakeServer(t)
	c := f.client()

	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	req := f.lastRequest(http.MethodGet)
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the credential from the source", got)
	}
	if ua := req.Header.Get("User-Agent"); !strings.Contains(ua, "reva-cli") {
		t.Errorf("User-Agent = %q, want it to identify the CLI", ua)
	}
}

func TestCustomUserAgent(t *testing.T) {
	f := newFakeServer(t)
	c := f.client(WithUserAgent("cernbox-cli/1.2.3 (rev-abc)"))
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.lastRequest(http.MethodGet).Header.Get("User-Agent"); got != "cernbox-cli/1.2.3 (rev-abc)" {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestMissingCredentialsIsAnAuthError(t *testing.T) {
	f := newFakeServer(t)
	c, err := New(f.ts.URL, WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Me(context.Background())
	if err == nil {
		t.Fatal("expected an error with no credential source")
	}
	if cberr.ExitCode(err) != cberr.ExitAuth {
		t.Errorf("exit code = %d, want %d", cberr.ExitCode(err), cberr.ExitAuth)
	}
}

func TestCredentialErrorPropagates(t *testing.T) {
	f := newFakeServer(t)
	want := cberr.Authf("no Kerberos ticket: run kinit")
	c, err := New(f.ts.URL, WithMaxRetries(0), WithCredentials(CredentialFunc(
		func(context.Context) (Credential, error) { return Credential{}, want },
	)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Me(context.Background()); !errors.Is(err, cberr.ErrAuth) {
		t.Errorf("got %v, want the credential source's auth error", err)
	}
}

func TestRetryOnTransientStatus(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "backend down", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, defaultMe)
	})

	c := f.client(WithMaxRetries(3))
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me after retries: %v", err)
	}
	if me.Username != "einstein" {
		t.Errorf("username = %q", me.Username)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3", got)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "nope", http.StatusNotFound)
	})

	c := f.client(WithMaxRetries(3))
	if _, err := c.Me(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d attempts, want exactly 1 for a 404", got)
	}
}

func TestNoRetryOnAuthFailure(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "expired", http.StatusUnauthorized)
	})

	c := f.client(WithMaxRetries(3))
	_, err := c.Me(context.Background())
	if cberr.ExitCode(err) != cberr.ExitAuth {
		t.Errorf("exit code = %d, want %d", cberr.ExitCode(err), cberr.ExitAuth)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server saw %d attempts, want 1: retrying will not refresh a credential", got)
	}
}

type renewingSource struct {
	mu      sync.Mutex
	token   string
	refused []string
}

func (s *renewingSource) Credential(context.Context) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Credential{Header: "x-access-token", Value: s.token}, nil
}

func (s *renewingSource) Renew(_ context.Context, refused Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refused = append(s.refused, refused.Value)
	s.token = "fresh"
	return nil
}

func TestUnauthorizedRenewsTheCredentialOnce(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-access-token") != "fresh" {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"id":"1","onPremisesSamAccountName":"alice"}`)
	})
	src := &renewingSource{token: "stale"}
	c := f.client(WithCredentials(src))
	u, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if u.Username != "alice" || len(src.refused) != 1 || src.refused[0] != "stale" {
		t.Errorf("user %q, refused %v: want alice after one renewal of the stale token", u.Username, src.refused)
	}

	f2 := newFakeServer(t)
	var calls atomic.Int32
	f2.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "no", http.StatusUnauthorized)
	})
	src = &renewingSource{token: "stale"}
	if _, err := f2.client(WithCredentials(src)).Me(context.Background()); cberr.ExitCode(err) != cberr.ExitAuth {
		t.Errorf("err = %v, want an auth error", err)
	}
	if calls.Load() != 2 {
		t.Errorf("server saw %d attempts, want 2: one renewal, then the answer", calls.Load())
	}
}

func TestRetryGivesUpAndReportsTheStatus(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "still down", http.StatusBadGateway)
	})

	c := f.client(WithMaxRetries(1))
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	var ce *cberr.Error
	if !errors.As(err, &ce) || ce.Status != http.StatusBadGateway {
		t.Errorf("got %v, want a classified error carrying 502", err)
	}
}

func TestServerMessageIsSurfaced(t *testing.T) {
	t.Run("DAV XML", func(t *testing.T) {
		f := newFakeServer(t)
		f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<?xml version="1.0"?><d:error xmlns:d="DAV:" xmlns:s="http://sabredav.org/ns">`+
				`<s:message>sharing is disabled for this space</s:message></d:error>`)
		})
		_, err := f.client().Me(context.Background())
		if err == nil || !strings.Contains(err.Error(), "sharing is disabled for this space") {
			t.Errorf("got %v, want the server's message", err)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		f := newFakeServer(t)
		f.on(http.MethodGet, graphV1+"/me", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"conflict","message":"a share already exists here"}}`)
		})
		_, err := f.client().Me(context.Background())
		if err == nil || !strings.Contains(err.Error(), "a share already exists here") {
			t.Errorf("got %v, want the server's message", err)
		}
	})
}

func TestContextCancellationIsNotRetried(t *testing.T) {
	f := newFakeServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := f.client(WithMaxRetries(3))
	if _, err := c.Me(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
	if n := f.countRequests(http.MethodGet); n != 0 {
		t.Errorf("a cancelled context produced %d requests, want 0", n)
	}
}

func TestURL(t *testing.T) {
	tests := []struct {
		name, base, in, want string
	}{
		{"absolute path", "https://cernbox.cern.ch", "/graph/v1.0/me", "https://cernbox.cern.ch/graph/v1.0/me"},
		{"no leading slash", "https://cernbox.cern.ch", "graph/v1.0/me", "https://cernbox.cern.ch/graph/v1.0/me"},
		{"base path", "https://example.org/cernbox", "/graph/v1.0/me", "https://example.org/cernbox/graph/v1.0/me"},
		// A second escaping would send "%2520" for a space, and the server would
		// create a file named "my%20notes.txt".
		{"already escaped", "https://cernbox.cern.ch",
			"/remote.php/dav/files/einstein/my%20notes%20%231.txt",
			"https://cernbox.cern.ch/remote.php/dav/files/einstein/my%20notes%20%231.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.base)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.URL(tt.in); got != tt.want {
				t.Errorf("URL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExtractHelpers(t *testing.T) {
	if got := extractTag(`<a><s:message>hi there</s:message></a>`, "s:message"); got != "hi there" {
		t.Errorf("extractTag = %q", got)
	}
	if got := extractTag(`<a></a>`, "s:message"); got != "" {
		t.Errorf("extractTag on a missing tag = %q, want empty", got)
	}
	if got := extractJSONField(`{"message":"boom","x":1}`, "message"); got != "boom" {
		t.Errorf("extractJSONField = %q", got)
	}
	if got := extractJSONField(`{"message":42}`, "message"); got != "" {
		t.Errorf("extractJSONField on a non-string = %q, want empty", got)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	for attempt := 1; attempt < 20; attempt++ {
		if d := backoff(attempt); d > 5_000_000_000 {
			t.Fatalf("backoff(%d) = %v, want it capped at 5s", attempt, d)
		}
	}
}
