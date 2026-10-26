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

// Package auth is the credential chain of the CLI: the sign-in methods
// (Provider) that give an upstream credential, in the order of the brand,
// and the cache of the credentials. The runtime exchanges the upstream
// credential for a reva token. The cli.auth.* plugins implement Provider.
package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// refreshWindow is how long before expiry a token is treated as already
// expired. It covers clock skew and the round trip of the request the token is
// about to be used for.
const refreshWindow = 60 * time.Second

// The kinds of credential. Each one but KindReva is also a key of
// auth.types of the brand: the gateway auth type of a gRPC Authenticate.
const (
	// KindReva is a reva token, used as it is.
	KindReva = "reva"
	// KindBearer is an OIDC access token.
	KindBearer = "bearer"
	// KindBasic is a user name and a password.
	KindBasic = "basic"
	// KindAppToken is a user name and an app token.
	KindAppToken = "app-token"
	// KindKerberos is a SPNEGO token, base64.
	KindKerberos = "kerberos"
)

// Token is a credential together with what is needed to manage its lifetime.
type Token struct {
	// Kind is one of the Kind constants.
	Kind string `json:"kind"`
	// Value is the secret: the token, the password or the SPNEGO token.
	Value string `json:"value"`
	// User is the user name of a basic or an app-token credential.
	User string `json:"user,omitempty"`
	// Expiry is when the token stops being valid. A zero value means unknown,
	// which is treated as "does not expire": an app password, for instance.
	Expiry time.Time `json:"expiry,omitzero"`
	// RefreshToken renews Value without re-authenticating, when the issuer
	// supplied one.
	RefreshToken string `json:"refresh_token,omitempty"`
	// Provider names the provider that produced this token, for "status"
	// and for deciding how to refresh it.
	Provider string `json:"provider"`
	// Subject is the identity the token represents, for display.
	Subject string `json:"subject,omitempty"`
}

// Valid reports whether the token can still be used, leaving a margin for the
// request it is about to authenticate.
func (t *Token) Valid() bool {
	if t == nil || t.Value == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return time.Now().Add(refreshWindow).Before(t.Expiry)
}

// Header returns the HTTP header that carries the token.
func (t *Token) Header() (name, value string) {
	switch t.Kind {
	case KindReva:
		return "x-access-token", t.Value
	case KindBasic, KindAppToken:
		return "Authorization", "Basic " + base64.StdEncoding.EncodeToString([]byte(t.User+":"+t.Value))
	case KindKerberos:
		return "Authorization", "Negotiate " + t.Value
	}
	return "Authorization", "Bearer " + t.Value
}

// Provider obtains a token by one specific method.
type Provider interface {
	// Name identifies the provider in "status" and in login --method.
	Name() string
	// Available reports whether this provider has what it needs to be tried.
	// It must be cheap and must not touch the network: the chain calls it on
	// every provider to decide what to attempt.
	Available(ctx context.Context) bool
	// Token authenticates and returns a token.
	Token(ctx context.Context) (*Token, error)
}

// Refresher is implemented by providers that can renew a token without
// re-authenticating from scratch.
type Refresher interface {
	Refresh(ctx context.Context, tok *Token) (*Token, error)
}

// IdentityHinter is implemented by providers that can name the identity they
// would authenticate as without actually authenticating. The chain uses it to
// key the token cache, so that a user who runs kinit as a different principal
// gets a different cache entry instead of silently reusing the previous
// identity's token.
type IdentityHinter interface {
	IdentityHint() string
}

// Chain resolves a credential by trying providers in order. It is safe for
// concurrent use, which matters because the transfer engine asks for a
// credential from many goroutines at once.
type Chain struct {
	providers []Provider
	cache     *Cache
	key       string
	advice    string

	mu      sync.Mutex
	current *Token
	// forced restricts the chain to a single provider, set by --method.
	forced string
}

// ChainOption configures a Chain.
type ChainOption func(*Chain)

// WithCache attaches a token cache. Without one, every invocation
// re-authenticates, which is correct but slow in a shell loop.
func WithCache(c *Cache) ChainOption { return func(ch *Chain) { ch.cache = c } }

// WithMethod restricts the chain to the named provider. An unknown name is
// reported when the chain is first used rather than at construction, so that
// "--method" errors come out through the normal error path.
func WithMethod(name string) ChainOption {
	return func(ch *Chain) { ch.forced = strings.ToLower(strings.TrimSpace(name)) }
}

// WithAdvice sets the text that tells how to sign in, for the error when no
// provider has a credential.
func WithAdvice(s string) ChainOption { return func(ch *Chain) { ch.advice = s } }

// NewChain builds a chain. key names the server in the cache: a credential
// is valid only where it was issued.
func NewChain(key string, providers []Provider, opts ...ChainOption) *Chain {
	ch := &Chain{key: key, providers: providers}
	for _, o := range opts {
		o(ch)
	}
	return ch
}

// Providers returns the configured providers, in order.
func (ch *Chain) Providers() []Provider { return ch.providers }

// Token returns a usable token, obtaining or refreshing one if needed.
func (ch *Chain) Token(ctx context.Context) (*Token, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.current.Valid() {
		return ch.current, nil
	}

	candidates, err := ch.candidates()
	if err != nil {
		return nil, err
	}

	// A cached token from a previous invocation avoids re-authenticating on
	// every command in a shell loop.
	if tok := ch.fromCache(ctx, candidates); tok != nil {
		ch.current = tok
		return tok, nil
	}

	// An expired token with a refresh token is cheaper to renew than to
	// replace, and it avoids a round trip to the issuer.
	if refreshed := ch.refresh(ctx, candidates); refreshed != nil {
		ch.store(refreshed)
		return refreshed, nil
	}

	var attempted []string
	var firstErr error
	for _, p := range candidates {
		if !p.Available(ctx) {
			continue
		}
		attempted = append(attempted, p.Name())
		tok, err := p.Token(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", p.Name(), err)
			}
			continue
		}
		tok.Provider = p.Name()
		ch.store(tok)
		return tok, nil
	}

	if firstErr != nil {
		// At least one provider was available and failed. That error is far
		// more useful than a generic "no credentials", because it names the
		// thing that went wrong.
		return nil, cberr.Authf("%v", firstErr)
	}
	return nil, ch.noCredentialsError(attempted)
}

// candidates returns the providers to try, honouring --method.
func (ch *Chain) candidates() ([]Provider, error) {
	if ch.forced == "" {
		return ch.providers, nil
	}
	for _, p := range ch.providers {
		if p.Name() == ch.forced {
			return []Provider{p}, nil
		}
	}
	names := make([]string, 0, len(ch.providers))
	for _, p := range ch.providers {
		names = append(names, p.Name())
	}
	return nil, cberr.Usagef("unknown sign-in method %q: use one of %s",
		ch.forced, strings.Join(names, ", "))
}

func (ch *Chain) fromCache(ctx context.Context, candidates []Provider) *Token {
	if ch.cache == nil {
		return nil
	}
	tok, ok := ch.cache.Get(Upstream, ch.cacheKey(candidates))
	if !ok || !tok.Valid() {
		return nil
	}
	// A cached token from a provider that --method excluded must not be used,
	// or "--method device" would silently reuse a Kerberos session.
	if ch.forced != "" && tok.Provider != ch.forced {
		return nil
	}
	return tok
}

// refresh tries to renew an expired cached token rather than authenticating
// again.
func (ch *Chain) refresh(ctx context.Context, candidates []Provider) *Token {
	if ch.cache == nil {
		return nil
	}
	tok, ok := ch.cache.Get(Upstream, ch.cacheKey(candidates))
	if !ok || tok.RefreshToken == "" {
		return nil
	}
	for _, p := range candidates {
		if p.Name() != tok.Provider {
			continue
		}
		r, ok := p.(Refresher)
		if !ok {
			return nil
		}
		refreshed, err := r.Refresh(ctx, tok)
		if err != nil {
			// A refresh token that the issuer has expired is not an error
			// worth reporting: fall through and authenticate normally, which
			// with Kerberos is silent anyway.
			return nil
		}
		refreshed.Provider = p.Name()
		return refreshed
	}
	return nil
}

// store keeps only bearer tokens: the other kinds are secrets that the
// provider has anyway, or SPNEGO tokens that the server takes only once.
func (ch *Chain) store(tok *Token) {
	if tok.Kind != KindBearer {
		return
	}
	ch.current = tok
	if ch.cache != nil {
		// A cache write failure must not fail the command: the token in hand
		// is still good, the next invocation will just re-authenticate.
		_ = ch.cache.Put(Upstream, ch.cacheKey(ch.providers), tok)
	}
}

// cacheKey identifies the cache entry for this endpoint and identity.
//
// The identity component is what stops a user with both a personal and a
// service principal from picking up the wrong session after kinit.
func (ch *Chain) cacheKey(candidates []Provider) string {
	hint := ""
	for _, p := range candidates {
		h, ok := p.(IdentityHinter)
		if !ok {
			continue
		}
		if v := h.IdentityHint(); v != "" {
			hint = v
			break
		}
	}
	return ch.key + "\x00" + hint
}

func (ch *Chain) noCredentialsError(attempted []string) error {
	var advice strings.Builder
	advice.WriteString("no usable credentials found")
	if len(attempted) > 0 {
		advice.WriteString(" (tried: " + strings.Join(attempted, ", ") + ")")
	}
	if ch.advice != "" {
		advice.WriteString(": " + ch.advice)
	}
	return cberr.Authf("%s", advice.String())
}

// Forget clears the in-memory and cached tokens for this server.
func (ch *Chain) Forget() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.current = nil
	if ch.cache == nil {
		return nil
	}
	return ch.cache.DeletePrefix(Upstream, ch.key+"\x00")
}

// Invalidate drops the token in memory and its cache entry, after the
// server refused it.
func (ch *Chain) Invalidate(tok *Token) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.current == tok {
		ch.current = nil
	}
	if ch.cache == nil || tok == nil || tok.Kind != KindBearer {
		return
	}
	// A refresh token can still work when the server refused the access
	// token, so the entry stays, as expired.
	key := ch.cacheKey(ch.providers)
	if c, ok := ch.cache.Get(Upstream, key); ok && c.RefreshToken != "" {
		c.Expiry = time.Unix(1, 0)
		_ = ch.cache.Put(Upstream, key, c)
		return
	}
	_ = ch.cache.Delete(Upstream, key)
}

// ErrNoProvider is returned when a chain is built with no providers at all.
var ErrNoProvider = errors.New("no authentication providers configured")
