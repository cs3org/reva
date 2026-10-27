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

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/cli/auth"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/spf13/pflag"
)

// revaHTTP signs in an app token of alice, as the auth interceptor of reva
// does: the reva token comes back in x-access-token.
func revaHTTP(t *testing.T, exp time.Time) (*httptest.Server, *atomic.Int32) {
	var signIns atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); ok {
			if u != "alice" || p != "app-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			signIns.Add(1)
			w.Header().Set("x-access-token", token(t, "alice", exp))
			return
		}
		if r.Header.Get("x-access-token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &signIns
}

func newTokenRuntime(t *testing.T, args ...string) *Runtime {
	t.Helper()
	r := NewRuntime(nil)
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	r.AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if err := r.Load(fs); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTokenExchangeAndCache(t *testing.T) {
	dir := setup(t)
	t.Setenv("KRB5CCNAME", "FILE:"+filepath.Join(dir, "none"))
	ts, signIns := revaHTTP(t, time.Now().Add(time.Hour))
	secret := filepath.Join(dir, "app-token")
	writeFile(t, secret, "app-secret\n")
	args := []string{"--endpoint", ts.URL, "--user", "alice", "--app-token-file", secret}

	got, err := newTokenRuntime(t, args...).Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if TokenExpiry(got).IsZero() || signIns.Load() != 1 {
		t.Fatalf("token %q after %d sign-ins, want a reva token after one", got, signIns.Load())
	}

	// The cache layer of the reva token is keyed by the endpoint and the
	// gateway: another gateway signs in again.
	c := auth.NewCache(filepath.Join(dir, ".config", "reva", "token"))
	key := "endpoint=" + ts.URL + " gateway="
	if tok, ok := c.Get(auth.Reva, key); !ok || tok.Value != got || tok.Provider != auth.MethodAppToken {
		t.Fatalf("cache entry %v, %v: want the reva token from app-token", tok, ok)
	}
	r := newTokenRuntime(t, "--endpoint", ts.URL)
	if again, err := r.Token(context.Background()); err != nil || again != got {
		t.Errorf("second run: %q, %v: want the cached token", again, err)
	}
	if s := r.SignedIn(); s == nil || s.Provider != auth.MethodAppToken || s.Subject != "alice" {
		t.Errorf("SignedIn() = %+v", s)
	}
	// The gateway of the same deployment can use it, another endpoint not.
	if again, err := newTokenRuntime(t, "--endpoint", ts.URL, "--gateway", "gw:9142").Token(context.Background()); err != nil || again != got {
		t.Errorf("the endpoint with a gateway: %q, %v: want the cached token", again, err)
	}
	if _, err := newTokenRuntime(t, "--endpoint", "https://other.example.org").Token(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "no login token") {
		t.Errorf("another endpoint: %v, want no login token", err)
	}

	// A refused token makes the next call sign in again.
	r = newTokenRuntime(t, args...)
	if _, err := r.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.renew(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if signIns.Load() != 2 {
		t.Errorf("%d sign-ins, want 2: the cached token of alice, then one more after the refusal", signIns.Load())
	}
}

func TestTokenExpiredSignsInAgain(t *testing.T) {
	dir := setup(t)
	ts, signIns := revaHTTP(t, time.Now().Add(time.Hour))
	c := auth.NewCache(filepath.Join(dir, ".config", "reva", "token"))
	old := token(t, "alice", time.Now().Add(-time.Minute))
	if err := c.Put(auth.Reva, "endpoint="+ts.URL+" gateway=", &auth.Token{Kind: auth.KindReva, Value: old, Expiry: TokenExpiry(old)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVA_TOKEN", "")
	secret := filepath.Join(dir, "app-token")
	writeFile(t, secret, "app-secret")
	got, err := newTokenRuntime(t, "--endpoint", ts.URL, "--user", "alice", "--app-token-file", secret).Token(context.Background())
	if err != nil || got == old || signIns.Load() != 1 {
		t.Errorf("token %q, %v after %d sign-ins: want a new token", got, err, signIns.Load())
	}
}

func TestTokenOfAnOlderLogin(t *testing.T) {
	dir := setup(t)
	old := token(t, "alice", time.Now().Add(time.Hour))
	if err := os.MkdirAll(filepath.Join(dir, ".config", "reva"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".config", "reva", "token"), old)
	got, err := newTokenRuntime(t, "--gateway", "gw:9142").Token(context.Background())
	if err != nil || got != old {
		t.Errorf("token %q, %v: want the raw token of the old file", got, err)
	}
}

type ssoStub struct{}

func (ssoStub) Name() string                   { return "test-sso" }
func (ssoStub) Available(context.Context) bool { return true }
func (ssoStub) Token(context.Context) (*auth.Token, error) {
	return &auth.Token{Kind: auth.KindBearer, Value: "oidc-access-token", Expiry: time.Now().Add(time.Hour)}, nil
}

func TestAuthPluginInTheChain(t *testing.T) {
	setup(t)
	RegisterAuth("test-sso", func(*Runtime) AuthProvider { return ssoStub{} })
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("x-access-token", token(t, "alice", time.Now().Add(time.Hour)))
	}))
	defer ts.Close()
	t.Setenv("KRB5CCNAME", "FILE:"+filepath.Join(t.TempDir(), "none"))
	r := NewRuntime(brand.MustParse([]byte("[program]\nname = \"acme\"\n[auth]\nchain = [\"kerberos\", \"test-sso\"]\n")))
	r.httpEndpoint = ts.URL
	if _, err := r.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer oidc-access-token" {
		t.Errorf("Authorization = %q, want the bearer token of the plugin", got)
	}
}
