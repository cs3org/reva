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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OIDCConfig describes the OIDC client of the CLI.
type OIDCConfig struct {
	Issuer   string
	ClientID string
	// Scopes defaults to openid, profile and offline_access, which gives a
	// refresh token.
	Scopes []string
}

func (c *OIDCConfig) scopes() []string {
	if len(c.Scopes) > 0 {
		return c.Scopes
	}
	return []string{"openid", "profile", "offline_access"}
}

// ProviderMetadata is the subset of the OIDC discovery document the CLI uses.
type ProviderMetadata struct {
	Issuer                      string `json:"issuer"`
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

// Discover fetches the OIDC discovery document.
func Discover(ctx context.Context, hc *http.Client, issuer string) (*ProviderMetadata, error) {
	u := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting the identity provider at %s: %w", issuer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the identity provider at %s returned %s for its discovery document", issuer, resp.Status)
	}
	var meta ProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("parsing the discovery document from %s: %w", issuer, err)
	}
	if meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("the identity provider at %s advertises no token endpoint", issuer)
	}
	return &meta, nil
}

// TokenResponse is the OAuth token endpoint response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`

	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Token returns the token of the answer.
func (r *TokenResponse) Token(provider string) *Token {
	tok := &Token{
		Kind:         KindBearer,
		Value:        r.AccessToken,
		RefreshToken: r.RefreshToken,
		Provider:     provider,
	}
	switch {
	case r.ExpiresIn > 0:
		tok.Expiry = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	default:
		// The issuer left out expires_in.
		tok.Expiry = JWTExpiry(r.AccessToken)
	}
	return tok
}

// PostForm sends a form to a token endpoint and decodes the response. It
// decodes the body also for an error status: OAuth errors come as JSON.
func PostForm(ctx context.Context, hc *http.Client, endpoint string, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting the token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("the token endpoint returned %s with an unparseable body", resp.Status)
	}
	if tr.Error != "" {
		return &tr, fmt.Errorf("%s", OAuthErrorMessage(&tr))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &tr, fmt.Errorf("the token endpoint returned %s", resp.Status)
	}
	if tr.AccessToken == "" {
		return &tr, fmt.Errorf("the token endpoint returned no access token")
	}
	return &tr, nil
}

// OAuthErrorMessage returns the error of an OAuth answer.
func OAuthErrorMessage(tr *TokenResponse) string {
	if tr.ErrorDescription != "" {
		return tr.Error + ": " + tr.ErrorDescription
	}
	return tr.Error
}

// JWTExpiry reads the exp claim of a JWT without verifying it. The server
// verifies. A token that does not parse gets no expiry: the client uses it until
// the server answers 401.
func JWTExpiry(raw string) time.Time {
	raw = strings.TrimPrefix(raw, "Bearer ")
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
