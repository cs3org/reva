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
	"os"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// The names of the built-in providers, for auth.chain of a brand and for
// login --method.
const (
	MethodToken    = "token"
	MethodAppToken = "app-token"
	MethodBasic    = "basic"
	MethodKerberos = "kerberos"
	MethodDevice   = "device"
)

// TokenProvider presents a reva token that the caller supplied, through
// --token or its env. It is the escape hatch for CI and for debugging.
type TokenProvider struct {
	Value string
}

// Name implements Provider.
func (p *TokenProvider) Name() string { return MethodToken }

// Available implements Provider.
func (p *TokenProvider) Available(context.Context) bool { return p.Value != "" }

// Token implements Provider. No expiry is asserted: the runtime reads it
// from the token.
func (p *TokenProvider) Token(context.Context) (*Token, error) {
	if p.Value == "" {
		return nil, cberr.Authf("no token supplied")
	}
	return &Token{Kind: KindReva, Value: p.Value, Provider: MethodToken}, nil
}

// AppTokenProvider presents an app token (app password). This is the
// credential for batch jobs and cron, where no Kerberos ticket is
// forwarded.
type AppTokenProvider struct {
	// Username is the account the token belongs to.
	Username string
	// Secret is the app token. When empty, the file TokenFile is read.
	Secret string
	// TokenFile keeps the secret out of the environment, and so out of
	// /proc and of process listings.
	TokenFile string
}

// Name implements Provider.
func (p *AppTokenProvider) Name() string { return MethodAppToken }

// Available implements Provider.
func (p *AppTokenProvider) Available(context.Context) bool {
	user, token := p.credentials()
	return user != "" && token != ""
}

// Token implements Provider.
func (p *AppTokenProvider) Token(context.Context) (*Token, error) {
	user, token := p.credentials()
	if token == "" {
		return nil, cberr.Authf("no app token supplied: pass --app-token-file")
	}
	if user == "" {
		return nil, cberr.Authf("an app token needs a user name: pass --user")
	}
	return &Token{Kind: KindAppToken, User: user, Value: token, Provider: MethodAppToken, Subject: user}, nil
}

// IdentityHint implements IdentityHinter.
func (p *AppTokenProvider) IdentityHint() string { return p.Username }

func (p *AppTokenProvider) credentials() (user, token string) {
	token = p.Secret
	if token == "" && p.TokenFile != "" {
		if b, err := os.ReadFile(p.TokenFile); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	return p.Username, token
}

// BasicProvider presents a user name and a password. It exists for
// development instances, which have no Kerberos and no SSO.
type BasicProvider struct {
	Username string
	Password string
	// Prompt asks for what is missing: the user name when it is empty, and
	// the password. It is nil in non-interactive contexts, which makes the
	// provider unavailable rather than hanging.
	Prompt func(user string) (string, string, error)
}

// Name implements Provider.
func (p *BasicProvider) Name() string { return MethodBasic }

// Available implements Provider.
func (p *BasicProvider) Available(context.Context) bool {
	return p.Prompt != nil || p.Username != "" && p.Password != ""
}

// Token implements Provider.
func (p *BasicProvider) Token(context.Context) (*Token, error) {
	user, password := p.Username, p.Password
	if user == "" || password == "" {
		if p.Prompt == nil {
			return nil, cberr.Authf("no password for basic authentication")
		}
		var err error
		if user, password, err = p.Prompt(user); err != nil {
			return nil, cberr.Authf("reading the password: %v", err)
		}
	}
	if user == "" {
		return nil, cberr.Authf("no user name for basic authentication")
	}
	return &Token{Kind: KindBasic, User: user, Value: password, Provider: MethodBasic, Subject: user}, nil
}

// IdentityHint implements IdentityHinter.
func (p *BasicProvider) IdentityHint() string { return p.Username }
