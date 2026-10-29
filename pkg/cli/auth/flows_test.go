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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/cli/kerberos"
)

// fakeIDP is an OIDC provider with a discovery document, a token endpoint and
// a device flow.
type fakeIDP struct {
	ts *httptest.Server

	noDeviceEndpoint  bool
	devicePendingFor  int32
	noNewRefreshToken bool

	gotGrantType   string
	gotRefreshWith string

	devicePolls atomic.Int32
	issued      atomic.Int32
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		meta := map[string]string{
			"issuer":         idp.ts.URL,
			"token_endpoint": idp.ts.URL + "/token",
		}
		if !idp.noDeviceEndpoint {
			meta["device_authorization_endpoint"] = idp.ts.URL + "/device"
		}
		json.NewEncoder(w).Encode(meta)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		idp.gotGrantType = r.Form.Get("grant_type")

		switch idp.gotGrantType {
		case "refresh_token":
			idp.gotRefreshWith = r.Form.Get("refresh_token")
			if idp.gotRefreshWith != "refresh-me" {
				writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
				return
			}
		case "urn:ietf:params:oauth:grant-type:device_code":
			if idp.devicePolls.Add(1) <= idp.devicePendingFor {
				writeOAuthError(w, http.StatusBadRequest, "authorization_pending", "keep waiting")
				return
			}
		default:
			writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", idp.gotGrantType)
			return
		}

		n := idp.issued.Add(1)
		refreshed := "refresh-me"
		if idp.noNewRefreshToken {
			refreshed = ""
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("access-token-%d", n),
			"token_type":    "Bearer",
			"expires_in":    1200,
			"refresh_token": refreshed,
		})
	})

	mux.HandleFunc("/device", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "device-code-1",
			"user_code":        "WDJB-MJHT",
			"verification_uri": idp.ts.URL + "/activate",
			"expires_in":       600,
			"interval":         0,
		})
	})

	idp.ts = httptest.NewServer(mux)
	t.Cleanup(idp.ts.Close)
	return idp
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
}

func ssoConfig(idp *fakeIDP) *OIDCConfig {
	return &OIDCConfig{
		Issuer:   idp.ts.URL,
		ClientID: "cernbox-cli",
	}
}

func TestKerberosProvider(t *testing.T) {
	p := &KerberosProvider{
		Ticket: kerberos.Provider{CCache: filepath.Join(t.TempDir(), "none")},
		SPN:    func() (string, error) { return "HTTP/box.example.org", nil },
	}
	if p.Available(context.Background()) {
		t.Error("available without a credential cache")
	}
	if _, err := p.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "kinit") {
		t.Errorf("Token without a ticket: %v, want the advice to run kinit", err)
	}
}

func TestKerberosProviderToken(t *testing.T) {
	p := &KerberosProvider{
		SPN:       func() (string, error) { return "HTTP/box.example.org", nil },
		Negotiate: func(spn string) (string, error) { return "token-for-" + spn, nil },
	}
	p.once.Do(func() {
		p.ticket = &kerberos.Ticket{Principal: "einstein@CERN.CH", Realm: "CERN.CH", CachePath: "/tmp/stub"}
	})
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Kind != KindKerberos || tok.Value != "token-for-HTTP/box.example.org" || tok.User != "einstein" || p.IdentityHint() != "einstein@CERN.CH" {
		t.Errorf("token %+v, hint %q", tok, p.IdentityHint())
	}
}

func TestDeviceFlow(t *testing.T) {
	idp := newFakeIDP(t)
	var promptedURI, promptedCode string

	p := &DeviceProvider{
		OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client(), Interactive: true,
		Prompt: func(uri, code string, _ time.Duration) {
			promptedURI, promptedCode = uri, code
		},
	}
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Provider != MethodDevice {
		t.Errorf("provider = %q", tok.Provider)
	}
	if promptedCode != "WDJB-MJHT" {
		t.Errorf("the user code was not shown: %q", promptedCode)
	}
	if !strings.Contains(promptedURI, "/activate") {
		t.Errorf("the verification URL was not shown: %q", promptedURI)
	}
}

func TestDeviceFlowPolls(t *testing.T) {
	idp := newFakeIDP(t)
	idp.devicePendingFor = 2

	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client(), Interactive: true}
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value == "" {
		t.Error("no token after polling")
	}
	if got := idp.devicePolls.Load(); got != 3 {
		t.Errorf("polled %d times, want 3 (two pending, then success)", got)
	}
}

func TestDeviceFlowUnavailableWithoutATTY(t *testing.T) {
	idp := newFakeIDP(t)
	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client(), Interactive: false}
	if p.Available(context.Background()) {
		t.Error("the device flow should be unavailable in a non-interactive context")
	}
}

func TestDeviceFlowWithoutServerSupport(t *testing.T) {
	idp := newFakeIDP(t)
	idp.noDeviceEndpoint = true

	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client(), Interactive: true}
	_, err := p.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not support signing in with a browser code") {
		t.Errorf("got %v, want a clear explanation and an alternative", err)
	}
}

func TestDeviceFlowTimesOut(t *testing.T) {
	idp := newFakeIDP(t)
	idp.devicePendingFor = 1000

	p := &DeviceProvider{
		OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client(), Interactive: true,
		MaxWait: 10 * time.Millisecond,
	}
	_, err := p.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not completed in time") {
		t.Errorf("got %v, want a timeout", err)
	}
}

func TestDeviceRefresh(t *testing.T) {
	idp := newFakeIDP(t)
	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client()}

	got, err := p.Refresh(context.Background(), &Token{
		RefreshToken: "refresh-me", Subject: "gdelmont", Provider: MethodDevice,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idp.gotGrantType != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", idp.gotGrantType)
	}
	if idp.gotRefreshWith != "refresh-me" {
		t.Errorf("the issuer was sent %q", idp.gotRefreshWith)
	}
	if got.Value == "" {
		t.Error("no access token came back")
	}
	if got.Subject != "gdelmont" {
		t.Errorf("Subject = %q, want it carried over", got.Subject)
	}
}

// Not every issuer rotates the refresh token.
func TestDeviceRefreshKeepsTheRefreshToken(t *testing.T) {
	idp := newFakeIDP(t)
	idp.noNewRefreshToken = true
	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client()}

	got, err := p.Refresh(context.Background(), &Token{RefreshToken: "refresh-me"})
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshToken != "refresh-me" {
		t.Errorf("RefreshToken = %q, want the original kept", got.RefreshToken)
	}
}

func TestDeviceRefreshRejected(t *testing.T) {
	idp := newFakeIDP(t)
	p := &DeviceProvider{OIDC: ssoConfig(idp), HTTPClient: idp.ts.Client()}

	if _, err := p.Refresh(context.Background(), &Token{RefreshToken: "stale"}); err == nil {
		t.Fatal("a refresh token the issuer rejects should be an error")
	}
}

func TestDeviceRefreshWithoutAToken(t *testing.T) {
	p := &DeviceProvider{}
	if _, err := p.Refresh(context.Background(), &Token{}); err == nil {
		t.Error("refreshing without a refresh token should fail")
	}
	if _, err := p.Refresh(context.Background(), nil); err == nil {
		t.Error("refreshing a nil token should fail")
	}
}

// The chain refreshes only a provider that implements Refresher.
func TestDeviceProviderIsARefresher(t *testing.T) {
	var p any = &DeviceProvider{}
	if _, ok := p.(Refresher); !ok {
		t.Fatal("DeviceProvider must implement Refresher or the chain cannot renew a session")
	}
}
