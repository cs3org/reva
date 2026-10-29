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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// DeviceProvider signs in with the OAuth device authorization grant (RFC 8628):
// the user completes the login in a browser while the CLI polls.
type DeviceProvider struct {
	OIDC       *OIDCConfig
	HTTPClient *http.Client
	// Interactive says that a user can complete the flow. A script must not
	// wait for a login that nobody performs.
	Interactive bool
	// Prompt shows the verification URL and the user code.
	Prompt func(verificationURI, userCode string, expiresIn time.Duration)
	// MaxWait bounds the polling. Zero means the lifetime of the device code.
	MaxWait time.Duration
}

const (
	defaultPollInterval = 5 * time.Second
	minPollInterval     = 10 * time.Millisecond
)

// Name implements Provider.
func (p *DeviceProvider) Name() string { return MethodDevice }

// Available implements Provider.
func (p *DeviceProvider) Available(context.Context) bool {
	return p.Interactive && p.OIDC != nil && p.OIDC.Issuer != "" && p.OIDC.ClientID != ""
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	// Interval is a pointer: RFC 8628 says that an absent interval is five
	// seconds, and that zero means no rate limit.
	Interval *int64 `json:"interval"`

	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Token implements Provider.
func (p *DeviceProvider) Token(ctx context.Context) (*Token, error) {
	hc := p.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}

	meta, err := Discover(ctx, hc, p.OIDC.Issuer)
	if err != nil {
		return nil, err
	}
	if meta.DeviceAuthorizationEndpoint == "" {
		return nil, cberr.Authf("this server does not support signing in with a browser code: " +
			"use another sign-in method, or an app token")
	}

	auth, err := p.startDeviceAuth(ctx, hc, meta)
	if err != nil {
		return nil, err
	}

	verificationURI := auth.VerificationURIComplete
	if verificationURI == "" {
		verificationURI = auth.VerificationURI
	}
	if p.Prompt != nil {
		p.Prompt(verificationURI, auth.UserCode, time.Duration(auth.ExpiresIn)*time.Second)
	}

	return p.poll(ctx, hc, meta, auth)
}

// Refresh implements Refresher. It needs no terminal, so a cron job with a
// cached refresh token renews silently.
func (p *DeviceProvider) Refresh(ctx context.Context, tok *Token) (*Token, error) {
	if tok == nil || tok.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token to renew with")
	}
	if p.OIDC == nil || p.OIDC.Issuer == "" {
		return nil, fmt.Errorf("no OIDC issuer configured")
	}

	hc := p.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	meta, err := Discover(ctx, hc, p.OIDC.Issuer)
	if err != nil {
		return nil, err
	}

	tr, err := PostForm(ctx, hc, meta.TokenEndpoint, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
		"client_id":     {p.OIDC.ClientID},
	})
	if err != nil {
		return nil, err
	}

	out := tr.Token(MethodDevice)
	if out.RefreshToken == "" {
		// Not every issuer rotates the refresh token.
		out.RefreshToken = tok.RefreshToken
	}
	out.Subject = tok.Subject
	return out, nil
}

func (p *DeviceProvider) startDeviceAuth(ctx context.Context, hc *http.Client, meta *ProviderMetadata) (*deviceAuthResponse, error) {
	form := url.Values{
		"client_id": {p.OIDC.ClientID},
		"scope":     {strings.Join(p.OIDC.scopes(), " ")},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		meta.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("starting the device login: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var auth deviceAuthResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("the device endpoint returned %s with an unparseable body", resp.Status)
	}
	if auth.Error != "" {
		msg := auth.Error
		if auth.ErrorDescription != "" {
			msg += ": " + auth.ErrorDescription
		}
		return nil, cberr.Authf("starting the device login: %s", msg)
	}
	if auth.DeviceCode == "" || auth.UserCode == "" {
		return nil, cberr.Authf("the device endpoint returned an incomplete response")
	}
	return &auth, nil
}

func (p *DeviceProvider) poll(ctx context.Context, hc *http.Client, meta *ProviderMetadata, auth *deviceAuthResponse) (*Token, error) {
	interval := defaultPollInterval
	if auth.Interval != nil {
		interval = time.Duration(*auth.Interval) * time.Second
	}
	// A tight loop gets the client rate-limited, even if the server says zero.
	interval = max(interval, minPollInterval)

	deadline := time.Now().Add(time.Duration(auth.ExpiresIn) * time.Second)
	if auth.ExpiresIn <= 0 {
		deadline = time.Now().Add(10 * time.Minute)
	}
	if p.MaxWait > 0 {
		if earlier := time.Now().Add(p.MaxWait); earlier.Before(deadline) {
			deadline = earlier
		}
	}

	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {auth.DeviceCode},
		"client_id":   {p.OIDC.ClientID},
	}

	// The first poll is immediate: the login may be done already.
	first := true
	for {
		if !first {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
		first = false

		if time.Now().After(deadline) {
			return nil, cberr.Authf("the login was not completed in time")
		}

		tr, err := PostForm(ctx, hc, meta.TokenEndpoint, form)
		if err == nil {
			return tr.Token(MethodDevice), nil
		}
		if tr == nil {
			return nil, err
		}
		switch tr.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return nil, cberr.Authf("the login was refused")
		case "expired_token":
			return nil, cberr.Authf("the login code expired before it was used")
		default:
			return nil, cberr.Authf("device login failed: %s", OAuthErrorMessage(tr))
		}
	}
}
