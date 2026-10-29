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
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cs3org/reva/v3"
	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli/auth"
	"github.com/cs3org/reva/v3/pkg/cli/kerberos"
	"golang.org/x/term"
)

// The cache is keyed by the endpoint and the gateway, because a reva token is
// valid only where the JWT secret is shared.

// LoginOptions select how Login signs in.
type LoginOptions struct {
	// Method is a provider of the chain. Empty means the chain of the brand,
	// in its order. A name that is no provider is a gateway auth type, for a
	// user name and a secret over gRPC (for example machine).
	Method      string
	SPN, CCache string
	In          io.Reader
	Out         io.Writer
	Terminal    bool
}

// SignIn is the result of a sign-in.
type SignIn struct {
	Provider string    `json:"provider"`
	Subject  string    `json:"subject,omitempty"`
	Expires  time.Time `json:"expires,omitzero"`
}

func (r *Runtime) serverKey() string {
	return "endpoint=" + r.httpEndpoint + " gateway=" + r.gatewayAddr
}

// cachedToken returns the cached reva token of the same endpoint and
// gateway. It also accepts one where only one of the two is known on one
// side, so that a login over HTTPS serves a command that sets the gateway.
func (r *Runtime) cachedToken(c *auth.Cache) (*auth.Token, bool) {
	if t, ok := c.Get(auth.Reva, r.serverKey()); ok && t.Valid() {
		return t, true
	}
	match := func(a, b string) int {
		switch {
		case a == b && a != "":
			return 1
		case a == "" || b == "":
			return 0
		}
		return -1
	}
	for key, t := range c.Entries(auth.Reva) {
		ep, gw, _ := strings.Cut(strings.TrimPrefix(key, "endpoint="), " gateway=")
		e, g := match(ep, r.httpEndpoint), match(gw, r.gatewayAddr)
		if e >= 0 && g >= 0 && e+g > 0 && t.Valid() {
			return t, true
		}
	}
	return nil, false
}

func (r *Runtime) tokenCacheFile() *auth.Cache {
	p, _ := state.TokenPath()
	return auth.NewCache(p)
}

// loginToken returns the token of --token, else the cached one, else it
// signs in with the chain. It asks nothing unless a provider can (the device
// flow on a terminal).
func (r *Runtime) loginToken(ctx context.Context) (string, error) {
	if r.tokenValue != "" {
		return r.tokenValue, nil
	}
	tokMu.Lock()
	defer tokMu.Unlock()
	if r.revaTok.Valid() {
		return r.revaTok.Value, nil
	}
	c := r.tokenCacheFile()
	// An app token that is set names its user: a cached token of another
	// user does not do.
	if t, ok := r.cachedToken(c); ok && (!r.hasAppToken() || t.Subject == r.user) {
		r.revaTok = t
		return t.Value, nil
	}
	if !r.hasAppToken() {
		if old := c.Legacy(); old != "" && usable(old) {
			r.revaTok = &auth.Token{Kind: auth.KindReva, Value: old, Expiry: TokenExpiry(old)}
			return old, nil
		}
	}
	t, err := r.signIn(ctx, LoginOptions{}, false)
	if err != nil {
		return "", err
	}
	return t.Value, nil
}

func (r *Runtime) hasAppToken() bool { return r.appToken != "" || r.appTokenFile != "" }

// Login signs in with opts, and stores the reva token for this server.
func (r *Runtime) Login(ctx context.Context, opts LoginOptions) (*SignIn, error) {
	if r.tokenValue != "" {
		return nil, errors.New("--token is set: there is nothing to sign in to")
	}
	tokMu.Lock()
	defer tokMu.Unlock()
	t, err := r.signIn(ctx, opts, true)
	if err != nil {
		return nil, err
	}
	return &SignIn{Provider: t.Provider, Subject: t.Subject, Expires: t.Expiry}, nil
}

// SignedIn returns the stored sign-in for this server, nil if there is none.
func (r *Runtime) SignedIn() *SignIn {
	if t, ok := r.cachedToken(r.tokenCacheFile()); ok {
		return &SignIn{Provider: t.Provider, Subject: t.Subject, Expires: t.Expiry}
	}
	return nil
}

// Logout forgets the reva token and the upstream credentials of every
// server, and the admin token.
func (r *Runtime) Logout() error {
	r.revaTok = nil
	return state.ClearTokens()
}

// renew drops the reva token that the server refused, so that the next
// call signs in again. A token of --token cannot be renewed.
func (r *Runtime) renew(ctx context.Context, refused string) error {
	if r.tokenValue != "" {
		return errors.New("the server refused the token of --token")
	}
	tokMu.Lock()
	defer tokMu.Unlock()
	if r.revaTok != nil && r.revaTok.Value != refused && r.revaTok.Valid() {
		return nil
	}
	r.revaTok = nil
	c := r.tokenCacheFile()
	for key, t := range c.Entries(auth.Reva) {
		if t.Value == refused {
			_ = c.Delete(auth.Reva, key)
		}
	}
	if c.Legacy() == refused {
		_ = c.Clear()
	}
	_, err := r.signIn(ctx, LoginOptions{}, false)
	return err
}

// signIn runs with tokMu held.
func (r *Runtime) signIn(ctx context.Context, opts LoginOptions, login bool) (*auth.Token, error) {
	if opts.Method != "" && !slices.Contains(AuthProviders(), opts.Method) {
		return r.gatewayLogin(ctx, opts)
	}
	providers := r.providers(opts, login)
	c := r.tokenCacheFile()
	chainOpts := []auth.ChainOption{auth.WithCache(c)}
	if opts.Method != "" {
		chainOpts = append(chainOpts, auth.WithMethod(opts.Method))
	}
	ch := auth.NewChain(r.serverKey(), providers, chainOpts...)
	up, err := ch.Token(ctx)
	if err != nil {
		if !login && !slices.ContainsFunc(providers, func(p auth.Provider) bool { return p.Available(ctx) }) {
			return nil, fmt.Errorf("no login token: run `%s login` first", r.Name())
		}
		return nil, err
	}
	if opts.Method == auth.MethodKerberos && r.user != "" && up.User != r.user {
		return nil, fmt.Errorf("the Kerberos ticket is for %s, not %s: run `kinit %s`, or leave out --user", up.Subject, r.user, r.user)
	}
	tok, err := r.exchange(ctx, up)
	if err != nil {
		ch.Invalidate(up)
		return nil, err
	}
	t := &auth.Token{Kind: auth.KindReva, Value: tok, Expiry: TokenExpiry(tok), Provider: up.Provider, Subject: cmp.Or(up.Subject, up.User)}
	r.revaTok = t
	if up.Kind != auth.KindReva || login {
		if err := c.Put(auth.Reva, r.serverKey(), t); err != nil {
			r.Debugf("token cache: %v", err)
		}
	}
	return t, nil
}

// providers returns the providers of the chain. A login gets every provider,
// so that --method can name one that the brand leaves out. Without a login,
// basic is left out: it would ask for a password on every command.
func (r *Runtime) providers(opts LoginOptions, login bool) []auth.Provider {
	terminal := opts.Terminal || term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
	order := slices.Clone(r.Brand().Auth.Chain)
	if r.hasAppToken() {
		order = append([]string{auth.MethodAppToken}, order...)
	}
	if login {
		for _, n := range AuthProviders() {
			if !slices.Contains(order, n) {
				order = append(order, n)
			}
		}
	}
	out := make([]auth.Provider, 0, len(order))
	for _, name := range slices.Compact(order) {
		if p := r.provider(name, opts, login, terminal); p != nil && !slices.ContainsFunc(out, func(q auth.Provider) bool { return q.Name() == name }) {
			out = append(out, p)
		}
	}
	return out
}

func (r *Runtime) provider(name string, opts LoginOptions, login, terminal bool) auth.Provider {
	switch name {
	case auth.MethodToken:
		return &auth.TokenProvider{Value: r.tokenValue}
	case auth.MethodAppToken:
		return &auth.AppTokenProvider{Username: r.user, Secret: r.appToken, TokenFile: r.appTokenFile}
	case auth.MethodBasic:
		if !login && opts.Method != auth.MethodBasic {
			return nil
		}
		p := &auth.BasicProvider{Username: r.user, Password: r.password}
		if login {
			p.Prompt = func(user string) (string, string, error) { return readCredentials(opts.In, opts.Out, user) }
		}
		return p
	case auth.MethodKerberos:
		return &auth.KerberosProvider{
			Ticket: kerberos.Provider{CCache: opts.CCache},
			SPN: func() (string, error) {
				if v := cmp.Or(opts.SPN, r.kerberosSPN); v != "" {
					return v, nil
				}
				return r.defaultSPN()
			},
		}
	case auth.MethodDevice:
		o := r.Brand().Auth.OIDC
		return &auth.DeviceProvider{
			OIDC:        &auth.OIDCConfig{Issuer: o.Issuer, ClientID: o.ClientID, Scopes: o.Scopes},
			HTTPClient:  r.HTTPClient().GetNativeHTTP(),
			Interactive: terminal,
			Prompt: func(uri, code string, expires time.Duration) {
				fmt.Fprintf(os.Stderr, "\nTo sign in, open:\n\n    %s\n\nand enter the code:  %s\n\n", uri, code)
				if expires > 0 {
					fmt.Fprintf(os.Stderr, "The code expires in %s. Waiting...\n", expires.Round(time.Second))
				}
			},
		}
	}
	for _, p := range reva.GetPlugins(AuthNamespace) {
		if strings.TrimPrefix(string(p.ID), AuthNamespace+".") != name {
			continue
		}
		if newFn, ok := p.New.(func(*Runtime) AuthProvider); ok {
			return newFn(r)
		}
	}
	return nil
}

func (r *Runtime) exchange(ctx context.Context, up *auth.Token) (string, error) {
	if up.Kind == auth.KindReva {
		return up.Value, nil
	}
	if r.httpEndpoint != "" {
		t, err := r.exchangeHTTP(ctx, up)
		var ne net.Error
		if err == nil || r.gatewayAddr == "" || !errors.As(err, &ne) {
			return t, err
		}
		r.Debugf("sign-in over https: %v: trying the gateway", err)
	}
	if r.gatewayAddr == "" {
		return "", r.notSet("an endpoint or a gateway", "endpoint", "--endpoint", "endpoint https://<host>")
	}
	return r.Authenticate(ctx, r.AuthType(up.Kind), up.User, up.Value)
}

// exchangeHTTP sends the credential to an authenticated path. The auth
// interceptor of reva writes the reva token in the x-access-token header of
// the answer.
func (r *Runtime) exchangeHTTP(ctx context.Context, up *auth.Token) (string, error) {
	u, err := url.JoinPath(r.httpEndpoint, cmp.Or(r.Brand().Auth.Kerberos.Path, "/ocs/v1.php/cloud/user"))
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	name, value := up.Header()
	req.Header.Set(name, value)
	req.Header.Set("OCS-APIREQUEST", "true")
	res, err := r.HTTPClient().GetNativeHTTP().Do(req)
	if err != nil {
		return "", err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	res.Body.Close()
	tok := res.Header.Get(appctx.TokenHeader)
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return "", fmt.Errorf("sign-in with %s: %s refused the credential", up.Provider, r.httpEndpoint)
	case res.StatusCode >= 300:
		return "", fmt.Errorf("sign-in with %s: %s answered %s", up.Provider, u, res.Status)
	case tok == "":
		return "", fmt.Errorf("sign-in with %s: %s took the credential but sent no reva token (x-access-token)", up.Provider, r.httpEndpoint)
	}
	return tok, nil
}

// gatewayLogin signs in with a gateway auth type that is no provider of the
// chain, for example machine with an API key.
func (r *Runtime) gatewayLogin(ctx context.Context, opts LoginOptions) (*auth.Token, error) {
	user, secret := r.user, r.password
	if secret == "" {
		var err error
		if user, secret, err = readCredentials(opts.In, opts.Out, user); err != nil {
			return nil, err
		}
	}
	tok, err := r.Authenticate(ctx, opts.Method, user, secret)
	if err != nil {
		return nil, err
	}
	t := &auth.Token{Kind: auth.KindReva, Value: tok, Expiry: TokenExpiry(tok), Provider: opts.Method, Subject: user}
	r.revaTok = t
	if err := r.tokenCacheFile().Put(auth.Reva, r.serverKey(), t); err != nil {
		return nil, err
	}
	return t, nil
}

// defaultSPN is HTTP/<host> of the endpoint, else of the gateway.
func (r *Runtime) defaultSPN() (string, error) {
	addr := r.gatewayAddr
	if u, err := url.Parse(r.httpEndpoint); err == nil && u.Host != "" {
		addr = u.Host
	}
	// A gRPC target can name a resolver, as in dns:///host:port.
	if _, rest, ok := strings.Cut(addr, ":///"); ok {
		addr = rest
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "", errors.New("no service principal for Kerberos: pass --spn HTTP/<host>, or set a gateway or an endpoint")
	}
	return "HTTP/" + host, nil
}

// readCredentials asks for the missing user name and for the secret. The
// secret is never a flag, so it does not show in ps or in the shell history.
// Without a terminal it reads lines from in.
func readCredentials(in io.Reader, out io.Writer, username string) (string, string, error) {
	if in == nil {
		in, out = os.Stdin, os.Stderr
	}
	r := bufio.NewReader(in)
	f, isFile := in.(*os.File)
	tty := isFile && term.IsTerminal(int(f.Fd()))
	readLine := func(prompt string) (string, error) {
		if tty {
			fmt.Fprint(out, prompt)
		}
		line, err := r.ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return "", fmt.Errorf("reading %s: %w", strings.TrimSuffix(prompt, ": "), err)
		}
		return strings.TrimSpace(line), nil
	}
	var err error
	if username == "" {
		if username, err = readLine("username: "); err != nil {
			return "", "", err
		}
	}
	if !tty {
		secret, err := readLine("password: ")
		return username, secret, err
	}
	fmt.Fprint(out, "password: ")
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(out)
	return username, strings.TrimSpace(string(b)), err
}
