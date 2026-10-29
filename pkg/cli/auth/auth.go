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
// (Provider), in the order of the brand, and the cache of the credentials.
// The cli.auth.* plugins implement Provider.
package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// refreshWindow is how long before expiry a token counts as expired. It covers
// clock skew and the round trip of the request.
const refreshWindow = 60 * time.Second

// The kinds of credential. Each one but KindReva is a key of auth.types of the
// brand.
const (
	KindReva     = "reva"      // a reva token, used as it is
	KindBearer   = "bearer"    // an OIDC access token
	KindBasic    = "basic"     // a user name and a password
	KindAppToken = "app-token" // a user name and an app token
	KindKerberos = "kerberos"  // a SPNEGO token, base64
)

// Token is a credential together with what is needed to manage its lifetime.
type Token struct {
	Kind string `json:"kind"`
	// Value is the secret: the token, the password or the SPNEGO token.
	Value string `json:"value"`
	// User is the user name of a basic or an app-token credential.
	User string `json:"user,omitempty"`
	// Expiry is when the token stops being valid. Zero means it does not
	// expire, like an app password.
	Expiry       time.Time `json:"expiry,omitzero"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	// Provider names the provider that made the token, to refresh it.
	Provider string `json:"provider"`
	// Subject is the identity of the token, for display.
	Subject string `json:"subject,omitempty"`
}

// Valid reports whether the token can still be used.
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
	// Available must be cheap and must not touch the network: the chain calls
	// it on every provider.
	Available(ctx context.Context) bool
	Token(ctx context.Context) (*Token, error)
}

// Refresher is implemented by providers that can renew a token without
// signing in again.
type Refresher interface {
	Refresh(ctx context.Context, tok *Token) (*Token, error)
}

// IdentityHinter is implemented by providers that know their identity without
// signing in. The chain puts it in the cache key, so that kinit as another
// principal does not reuse the token of the first one.
type IdentityHinter interface {
	IdentityHint() string
}

// Chain resolves a credential by trying providers in order. It is safe for
// concurrent use: the transfer engine asks for a credential from many
// goroutines.
type Chain struct {
	providers []Provider
	cache     *Cache
	key       string

	mu      sync.Mutex
	current *Token
	forced  string
}

// ChainOption configures a Chain.
type ChainOption func(*Chain)

// WithCache attaches a token cache.
func WithCache(c *Cache) ChainOption { return func(ch *Chain) { ch.cache = c } }

// WithMethod restricts the chain to the named provider. An unknown name is an
// error at the first use of the chain, not here.
func WithMethod(name string) ChainOption {
	return func(ch *Chain) { ch.forced = strings.ToLower(strings.TrimSpace(name)) }
}

// NewChain builds a chain. key names the server in the cache: a credential
// is valid only where it was issued.
func NewChain(key string, providers []Provider, opts ...ChainOption) *Chain {
	ch := &Chain{key: key, providers: providers}
	for _, o := range opts {
		o(ch)
	}
	return ch
}

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

	if tok := ch.fromCache(candidates); tok != nil {
		ch.current = tok
		return tok, nil
	}

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
		// The error of a provider that failed is more useful than "no
		// credentials".
		return nil, cberr.Authf("%v", firstErr)
	}
	return nil, ch.noCredentialsError(attempted)
}

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

func (ch *Chain) fromCache(candidates []Provider) *Token {
	if ch.cache == nil {
		return nil
	}
	tok, ok := ch.cache.Get(Upstream, ch.cacheKey(candidates))
	if !ok || !tok.Valid() {
		return nil
	}
	// "--method device" must not reuse a Kerberos session.
	if ch.forced != "" && tok.Provider != ch.forced {
		return nil
	}
	return tok
}

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
			// An expired refresh token is not an error: sign in again.
			return nil
		}
		refreshed.Provider = p.Name()
		return refreshed
	}
	return nil
}

// store keeps only bearer tokens: the other kinds are secrets that the provider
// has anyway, or SPNEGO tokens that the server takes only once.
func (ch *Chain) store(tok *Token) {
	if tok.Kind != KindBearer {
		return
	}
	ch.current = tok
	if ch.cache != nil {
		// A cache write failure must not fail the command: the token is good.
		_ = ch.cache.Put(Upstream, ch.cacheKey(ch.providers), tok)
	}
}

// cacheKey names the cache entry of the server and of the identity, so that a
// user with two principals does not get the session of the other one.
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
	if len(attempted) == 0 {
		return cberr.Authf("no usable credentials found")
	}
	return cberr.Authf("no usable credentials found (tried: %s)", strings.Join(attempted, ", "))
}

// Forget drops the token and the cached tokens of this server.
func (ch *Chain) Forget() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.current = nil
	if ch.cache == nil {
		return nil
	}
	return ch.cache.DeletePrefix(Upstream, ch.key+"\x00")
}

// Invalidate drops the token and its cache entry, after the server refused it.
func (ch *Chain) Invalidate(tok *Token) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.current == tok {
		ch.current = nil
	}
	if ch.cache == nil || tok == nil || tok.Kind != KindBearer {
		return
	}
	// The refresh token can still work, so the entry stays, as expired.
	key := ch.cacheKey(ch.providers)
	if c, ok := ch.cache.Get(Upstream, key); ok && c.RefreshToken != "" {
		c.Expiry = time.Unix(1, 0)
		_ = ch.cache.Put(Upstream, key, c)
		return
	}
	_ = ch.cache.Delete(Upstream, key)
}
