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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenProvider(t *testing.T) {
	ctx := context.Background()
	p := &TokenProvider{Value: "abc"}
	tok, err := p.Token(ctx)
	if err != nil || !p.Available(ctx) {
		t.Fatal(err)
	}
	if tok.Kind != KindReva || tok.Value != "abc" || !tok.Expiry.IsZero() {
		t.Errorf("token = %+v", tok)
	}
	if (&TokenProvider{}).Available(ctx) {
		t.Error("should be unavailable with nothing configured")
	}
}

func TestAppTokenProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit", func(t *testing.T) {
		p := &AppTokenProvider{Username: "einstein", Secret: "app-password"}
		if !p.Available(ctx) {
			t.Fatal("should be available")
		}
		tok, err := p.Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tok.Kind != KindAppToken || tok.User != "einstein" || tok.Value != "app-password" || tok.Subject != "einstein" {
			t.Errorf("token = %+v", tok)
		}
		if p.IdentityHint() != "einstein" {
			t.Errorf("identity hint = %q", p.IdentityHint())
		}
	})

	t.Run("from file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(path, []byte("  file-password\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		tok, err := (&AppTokenProvider{Username: "einstein", TokenFile: path}).Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tok.Value != "file-password" {
			t.Errorf("secret = %q, want the surrounding whitespace trimmed", tok.Value)
		}
	})

	t.Run("missing parts are explained", func(t *testing.T) {
		if _, err := (&AppTokenProvider{Secret: "x"}).Token(ctx); err == nil || !strings.Contains(err.Error(), "--user") {
			t.Errorf("got %v, want it to name the flag", err)
		}
		if _, err := (&AppTokenProvider{Username: "einstein"}).Token(ctx); err == nil || !strings.Contains(err.Error(), "--app-token-file") {
			t.Errorf("got %v, want it to name the flag", err)
		}
		if (&AppTokenProvider{Username: "einstein"}).Available(ctx) || (&AppTokenProvider{Secret: "x"}).Available(ctx) {
			t.Error("half a credential is not usable")
		}
	})
}

func TestBasicProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit", func(t *testing.T) {
		tok, err := (&BasicProvider{Username: "einstein", Password: "relativity"}).Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tok.Kind != KindBasic || tok.User != "einstein" || tok.Value != "relativity" {
			t.Errorf("token = %+v", tok)
		}
	})

	t.Run("prompts for what is missing", func(t *testing.T) {
		p := &BasicProvider{
			Username: "einstein",
			Prompt: func(user string) (string, string, error) {
				if user != "einstein" {
					t.Errorf("prompt got user %q", user)
				}
				return user, "typed-password", nil
			},
		}
		tok, err := p.Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tok.Value != "typed-password" {
			t.Errorf("secret = %q", tok.Value)
		}
	})

	t.Run("unavailable without a password or a prompt", func(t *testing.T) {
		if (&BasicProvider{Username: "einstein"}).Available(ctx) {
			t.Error("should be unavailable with no way to obtain a password")
		}
	})

	t.Run("prompt failure is reported", func(t *testing.T) {
		p := &BasicProvider{Prompt: func(string) (string, string, error) { return "", "", errors.New("not a terminal") }}
		if _, err := p.Token(ctx); err == nil {
			t.Error("expected the prompt failure to surface")
		}
	})
}

func TestProviderNames(t *testing.T) {
	names := map[string]Provider{
		MethodToken:    &TokenProvider{},
		MethodAppToken: &AppTokenProvider{},
		MethodBasic:    &BasicProvider{},
		MethodKerberos: &KerberosProvider{},
		MethodDevice:   &DeviceProvider{},
	}
	for want, p := range names {
		if got := p.Name(); got != want {
			t.Errorf("%T.Name() = %q, want %q", p, got, want)
		}
	}
}

func TestJWTExpiry(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1767225600}`))
	token := "header." + payload + ".signature"

	got := JWTExpiry(token)
	if got.Unix() != 1767225600 {
		t.Errorf("jwtExpiry = %v, want the exp claim", got)
	}
	if got := JWTExpiry("Bearer " + token); got.Unix() != 1767225600 {
		t.Errorf("jwtExpiry should tolerate the Bearer prefix, got %v", got)
	}

	// A token that does not parse has no expiry.
	for _, bad := range []string{"", "not-a-jwt", "a.b", "a.!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".c"} {
		if got := JWTExpiry(bad); !got.IsZero() {
			t.Errorf("JWTExpiry(%q) = %v, want zero", bad, got)
		}
	}
}
