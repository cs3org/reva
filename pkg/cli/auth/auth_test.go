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

package auth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// stubProvider is a Provider whose behaviour each test dictates.
type stubProvider struct {
	name      string
	available bool
	token     *Token
	err       error
	hint      string
	calls     int
	refreshes int
	refreshed *Token
	refreshFn func(*Token) (*Token, error)
}

func (p *stubProvider) Name() string                   { return p.name }
func (p *stubProvider) Available(context.Context) bool { return p.available }
func (p *stubProvider) IdentityHint() string           { return p.hint }
func (p *stubProvider) Token(context.Context) (*Token, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	cp := *p.token
	return &cp, nil
}

func (p *stubProvider) Refresh(_ context.Context, tok *Token) (*Token, error) {
	p.refreshes++
	if p.refreshFn != nil {
		return p.refreshFn(tok)
	}
	if p.refreshed == nil {
		return nil, errors.New("cannot refresh")
	}
	cp := *p.refreshed
	return &cp, nil
}

func validToken(provider string) *Token {
	return &Token{
		Kind:     KindBearer,
		Value:    provider + "-token",
		Expiry:   time.Now().Add(time.Hour),
		Provider: provider,
	}
}

func TestTokenValid(t *testing.T) {
	tests := []struct {
		name string
		tok  *Token
		want bool
	}{
		{"nil", nil, false},
		{"empty value", &Token{Kind: KindBearer}, false},
		{"no expiry means non-expiring", &Token{Value: "x"}, true},
		{"future expiry", &Token{Value: "x", Expiry: time.Now().Add(time.Hour)}, true},
		{"past expiry", &Token{Value: "x", Expiry: time.Now().Add(-time.Hour)}, false},
		// The refresh window is what stops a token expiring mid-request.
		{"expiring within the refresh window", &Token{Value: "x", Expiry: time.Now().Add(30 * time.Second)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tok.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestChainPrefersEarlierProviders pins the documented order: an explicit token
// beats Kerberos, Kerberos beats an app token.
func TestChainPrefersEarlierProviders(t *testing.T) {
	first := &stubProvider{name: "token", available: true, token: validToken("token")}
	second := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos")}

	ch := NewChain("https://cernbox.test", []Provider{first, second})
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "token" {
		t.Errorf("provider = %q, want the first available one", tok.Provider)
	}
	if second.calls != 0 {
		t.Errorf("the second provider was called %d times, want 0", second.calls)
	}
}

func TestChainSkipsUnavailableProviders(t *testing.T) {
	unavailable := &stubProvider{name: "token", available: false}
	available := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos")}

	ch := NewChain("https://cernbox.test", []Provider{unavailable, available})
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "kerberos" {
		t.Errorf("provider = %q", tok.Provider)
	}
	if unavailable.calls != 0 {
		t.Error("an unavailable provider must not be asked for a token")
	}
}

// TestChainFallsThroughOnFailure: an available provider that fails must not
// stop the chain, or a stale Kerberos ticket would block the device flow.
func TestChainFallsThroughOnFailure(t *testing.T) {
	failing := &stubProvider{name: "kerberos", available: true, err: errors.New("ticket expired")}
	working := &stubProvider{name: "device", available: true, token: validToken("device")}

	ch := NewChain("https://cernbox.test", []Provider{failing, working})
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "device" {
		t.Errorf("provider = %q, want the chain to continue past the failure", tok.Provider)
	}
}

// TestChainReportsTheFirstRealFailure: when everything fails, the error that
// names what actually went wrong is far more useful than "no credentials".
func TestChainReportsTheFirstRealFailure(t *testing.T) {
	failing := &stubProvider{name: "kerberos", available: true, err: errors.New("ticket expired")}
	alsoFailing := &stubProvider{name: "device", available: true, err: errors.New("no browser")}

	ch := NewChain("https://cernbox.test", []Provider{failing, alsoFailing})
	_, err := ch.Token(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "ticket expired") {
		t.Errorf("got %q, want it to name the first failure", err)
	}
	if cberr.ExitCode(err) != cberr.ExitAuth {
		t.Errorf("exit code = %d, want %d", cberr.ExitCode(err), cberr.ExitAuth)
	}
}

func TestChainWithNothingAvailableGivesAdvice(t *testing.T) {
	ch := NewChain("https://cernbox.test", []Provider{
		&stubProvider{name: "kerberos", available: false},
	}, WithAdvice("run `cernbox login`, or kinit if you use Kerberos"))
	_, err := ch.Token(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"kinit", "cernbox login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should suggest %q, got %q", want, err)
		}
	}
}

func TestChainCachesInMemory(t *testing.T) {
	p := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos")}
	ch := NewChain("https://cernbox.test", []Provider{p})

	for range 5 {
		if _, err := ch.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if p.calls != 1 {
		t.Errorf("provider called %d times, want 1: a valid token should be reused", p.calls)
	}
}

func TestChainReauthenticatesWhenExpired(t *testing.T) {
	expired := &Token{Kind: KindBearer, Value: "old", Expiry: time.Now().Add(-time.Hour), Provider: "kerberos"}
	p := &stubProvider{name: "kerberos", available: true, token: expired}
	ch := NewChain("https://cernbox.test", []Provider{p})

	for range 3 {
		if _, err := ch.Token(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if p.calls != 3 {
		t.Errorf("provider called %d times, want 3: an expired token must not be reused", p.calls)
	}
}

func TestChainMethodSelection(t *testing.T) {
	kerberos := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos")}
	device := &stubProvider{name: "device", available: true, token: validToken("device")}

	ch := NewChain("https://cernbox.test", []Provider{kerberos, device}, WithMethod("device"))
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "device" {
		t.Errorf("provider = %q, want the forced method", tok.Provider)
	}
	if kerberos.calls != 0 {
		t.Error("--method device must not fall back to kerberos")
	}
}

func TestChainUnknownMethod(t *testing.T) {
	ch := NewChain("https://cernbox.test", []Provider{
		&stubProvider{name: "kerberos", available: true, token: validToken("kerberos")},
	}, WithMethod("telepathy"))

	_, err := ch.Token(context.Background())
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error", err)
	}
	if !strings.Contains(err.Error(), "kerberos") {
		t.Errorf("the error should list the available methods, got %q", err)
	}
}

func TestChainUsesDiskCache(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	p := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos"), hint: "einstein@CERN.CH"}

	first := NewChain("https://cernbox.test", []Provider{p}, WithCache(cache))
	if _, err := first.Token(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A second process, same cache: the provider must not be asked again.
	second := NewChain("https://cernbox.test", []Provider{p}, WithCache(cache))
	tok, err := second.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "kerberos" {
		t.Errorf("provider = %q", tok.Provider)
	}
	if p.calls != 1 {
		t.Errorf("provider called %d times, want 1: the disk cache was not used", p.calls)
	}
}

// TestChainCacheIsKeyedByPrincipal is the guard against the worst cache bug:
// running kinit as a service principal and silently reusing the personal
// account's token.
func TestChainCacheIsKeyedByPrincipal(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")

	personal := &stubProvider{
		name: "kerberos", available: true, hint: "gdelmont@CERN.CH",
		token: &Token{Kind: KindBearer, Value: "personal", Expiry: time.Now().Add(time.Hour), Subject: "gdelmont"},
	}
	service := &stubProvider{
		name: "kerberos", available: true, hint: "svcbox@CERN.CH",
		token: &Token{Kind: KindBearer, Value: "service", Expiry: time.Now().Add(time.Hour), Subject: "svcbox"},
	}

	personalChain := NewChain("https://cernbox.test", []Provider{personal}, WithCache(cache))
	if _, err := personalChain.Token(context.Background()); err != nil {
		t.Fatal(err)
	}

	serviceChain := NewChain("https://cernbox.test", []Provider{service}, WithCache(cache))
	tok, err := serviceChain.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "service" {
		t.Errorf("token = %q: a different principal must not reuse the cached session", tok.Value)
	}
	if service.calls != 1 {
		t.Errorf("the service principal should have authenticated, calls = %d", service.calls)
	}
}

func TestChainCacheIsKeyedByEndpoint(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	p := &stubProvider{name: "token", available: true, token: validToken("token")}

	prod := NewChain("https://cernbox.cern.ch", []Provider{p}, WithCache(cache))
	if _, err := prod.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	preprod := NewChain("https://cernbox-test.cern.ch", []Provider{p}, WithCache(cache))
	if _, err := preprod.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Errorf("provider called %d times, want 2: a token for one endpoint must not be sent to another", p.calls)
	}
}

// TestChainIgnoresCachedTokenFromExcludedProvider: "--method device" must not
// pick up a cached Kerberos session.
func TestChainIgnoresCachedTokenFromExcludedProvider(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	kerberos := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos")}
	device := &stubProvider{name: "device", available: true, token: validToken("device")}

	seed := NewChain("https://cernbox.test", []Provider{kerberos}, WithCache(cache))
	if _, err := seed.Token(context.Background()); err != nil {
		t.Fatal(err)
	}

	forced := NewChain("https://cernbox.test", []Provider{kerberos, device}, WithCache(cache), WithMethod("device"))
	tok, err := forced.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != "device" {
		t.Errorf("provider = %q, want the forced method rather than the cached session", tok.Provider)
	}
}

func TestChainRefreshesRatherThanReauthenticating(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	expired := &Token{
		Kind: KindBearer, Value: "old",
		Expiry: time.Now().Add(-time.Minute), RefreshToken: "refresh-me", Provider: "kerberos",
	}
	if err := cache.Put(Upstream, "https://cernbox.test\x00einstein@CERN.CH", expired); err != nil {
		t.Fatal(err)
	}

	p := &stubProvider{
		name: "kerberos", available: true, hint: "einstein@CERN.CH",
		token:     validToken("kerberos"),
		refreshed: &Token{Kind: KindBearer, Value: "fresh", Expiry: time.Now().Add(time.Hour)},
	}

	ch := NewChain("https://cernbox.test", []Provider{p}, WithCache(cache))
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "fresh" {
		t.Errorf("token = %q, want the refreshed one", tok.Value)
	}
	if p.refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", p.refreshes)
	}
	if p.calls != 0 {
		t.Errorf("full authentication ran %d times, want 0 when a refresh works", p.calls)
	}
}

// TestChainFallsBackWhenRefreshFails: an expired refresh token should lead to a
// silent re-authentication, not an error the user has to act on.
func TestChainFallsBackWhenRefreshFails(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	expired := &Token{
		Kind: KindBearer, Value: "old",
		Expiry: time.Now().Add(-time.Minute), RefreshToken: "too-old", Provider: "kerberos",
	}
	if err := cache.Put(Upstream, "https://cernbox.test\x00einstein@CERN.CH", expired); err != nil {
		t.Fatal(err)
	}

	p := &stubProvider{
		name: "kerberos", available: true, hint: "einstein@CERN.CH",
		token:     validToken("kerberos"),
		refreshFn: func(*Token) (*Token, error) { return nil, errors.New("refresh token expired") },
	}

	ch := NewChain("https://cernbox.test", []Provider{p}, WithCache(cache))
	tok, err := ch.Token(context.Background())
	if err != nil {
		t.Fatalf("a failed refresh should fall back to authenticating: %v", err)
	}
	if tok.Provider != "kerberos" {
		t.Errorf("provider = %q", tok.Provider)
	}
	if p.calls != 1 {
		t.Errorf("full authentication ran %d times, want 1", p.calls)
	}
}

func TestChainForget(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	p := &stubProvider{name: "kerberos", available: true, token: validToken("kerberos"), hint: "einstein@CERN.CH"}

	ch := NewChain("https://cernbox.test", []Provider{p}, WithCache(cache))
	if _, err := ch.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ch.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 {
		t.Errorf("provider called %d times, want 2: logout should have cleared the session", p.calls)
	}
}

func TestTokenHeader(t *testing.T) {
	for _, tc := range []struct {
		tok        Token
		name, want string
	}{
		{Token{Kind: KindReva, Value: "jwt"}, "x-access-token", "jwt"},
		{Token{Kind: KindBearer, Value: "at"}, "Authorization", "Bearer at"},
		{Token{Kind: KindAppToken, User: "u", Value: "p"}, "Authorization", "Basic dTpw"},
		{Token{Kind: KindKerberos, Value: "spnego"}, "Authorization", "Negotiate spnego"},
	} {
		if n, v := tc.tok.Header(); n != tc.name || v != tc.want {
			t.Errorf("%s: header %s: %s, want %s: %s", tc.tok.Kind, n, v, tc.name, tc.want)
		}
	}
}

func TestChainCachesOnlyBearerTokens(t *testing.T) {
	cache := NewCache(t.TempDir() + "/cache")
	basic := &stubProvider{name: "basic", available: true, token: &Token{Kind: KindBasic, User: "u", Value: "secret"}}
	if _, err := NewChain("k", []Provider{basic}, WithCache(cache)).Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(cache.Entries(Upstream)); n != 0 {
		t.Errorf("%d cache entries after a basic sign-in, want none: a password is not cached", n)
	}
}

func TestCacheReadsTheOldTokenFile(t *testing.T) {
	path := t.TempDir() + "/token"
	if err := os.WriteFile(path, []byte("raw-jwt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewCache(path)
	if got := c.Legacy(); got != "raw-jwt" {
		t.Fatalf("Legacy() = %q, want the token of the old file", got)
	}
	if err := c.Put(Reva, "k", &Token{Kind: KindReva, Value: "new"}); err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Get(Reva, "k"); !ok || got.Value != "new" || c.Legacy() != "" {
		t.Errorf("after Put: %v %v, legacy %q: want the new format only", got, ok, c.Legacy())
	}
}
