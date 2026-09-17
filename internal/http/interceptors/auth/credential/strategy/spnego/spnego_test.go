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

package spnego

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newStrategy(t *testing.T, m map[string]any) *strategy {
	t.Helper()
	s, err := New(m)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.(*strategy)
}

func requestWith(header string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if header != "" {
		r.Header.Set("Authorization", header)
	}
	return r
}

func TestGetCredentials(t *testing.T) {
	s := newStrategy(t, nil)
	w := httptest.NewRecorder()

	creds, err := s.GetCredentials(w, requestWith("Negotiate YIIChAYGKwYBBQUCoIICeDCC"))
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	if creds.Type != "kerberos" {
		t.Errorf("Type = %q, want kerberos", creds.Type)
	}
	// The token stays base64: it arrives that way and the auth manager decodes
	// it, so decoding here would only mean encoding it again for the CS3 call.
	if creds.ClientSecret != "YIIChAYGKwYBBQUCoIICeDCC" {
		t.Errorf("ClientSecret = %q, want the token verbatim", creds.ClientSecret)
	}
	// The ticket is the only verified statement of identity, so nothing the
	// client says about who it is may be carried alongside it.
	if creds.ClientID != "" {
		t.Errorf("ClientID = %q, want it empty", creds.ClientID)
	}
}

// TestGetCredentialsSchemeIsCaseInsensitive: RFC 7235 makes the scheme
// case-insensitive, and clients do vary.
func TestGetCredentialsSchemeIsCaseInsensitive(t *testing.T) {
	s := newStrategy(t, nil)
	for _, scheme := range []string{"Negotiate", "negotiate", "NEGOTIATE", "NeGoTiAtE"} {
		creds, err := s.GetCredentials(httptest.NewRecorder(), requestWith(scheme+" token"))
		if err != nil {
			t.Errorf("scheme %q was rejected: %v", scheme, err)
			continue
		}
		if creds.ClientSecret != "token" {
			t.Errorf("scheme %q: secret = %q", scheme, creds.ClientSecret)
		}
	}
}

// TestGetCredentialsIgnoresOtherSchemes matters because this strategy sits in a
// chain: claiming a Basic or Bearer header would stop the strategy that can
// actually handle it from seeing it.
func TestGetCredentialsIgnoresOtherSchemes(t *testing.T) {
	s := newStrategy(t, nil)
	for _, header := range []string{
		"Basic dXNlcjpwYXNz",
		"Bearer eyJhbGciOiJIUzI1NiJ9",
		"Digest username=x",
	} {
		if _, err := s.GetCredentials(httptest.NewRecorder(), requestWith(header)); err == nil {
			t.Errorf("header %q should not be claimed by the SPNEGO strategy", header)
		}
	}
}

func TestGetCredentialsRejectsEmptyOrMalformed(t *testing.T) {
	s := newStrategy(t, nil)
	for _, header := range []string{
		"",
		"Negotiate",
		"Negotiate ",
		"Negotiate   ",
	} {
		if _, err := s.GetCredentials(httptest.NewRecorder(), requestWith(header)); err == nil {
			t.Errorf("header %q should be rejected", header)
		}
	}
}

func TestAuthTypeIsConfigurable(t *testing.T) {
	s := newStrategy(t, map[string]any{"auth_type": "krb5"})
	creds, err := s.GetCredentials(httptest.NewRecorder(), requestWith("Negotiate token"))
	if err != nil {
		t.Fatal(err)
	}
	if creds.Type != "krb5" {
		t.Errorf("Type = %q, want the configured krb5", creds.Type)
	}
}

// TestAddWWWAuthenticate: RFC 4559 defines the challenge as the bare scheme. A
// client that sees an unexpected parameter may decline to negotiate, so the
// realm must not be appended even though the interface offers one.
func TestAddWWWAuthenticate(t *testing.T) {
	s := newStrategy(t, nil)
	w := httptest.NewRecorder()

	s.AddWWWAuthenticate(w, requestWith(""), "cernbox.cern.ch")

	got := w.Header().Values("WWW-Authenticate")
	if len(got) != 1 {
		t.Fatalf("got %d challenges, want 1: %v", len(got), got)
	}
	if got[0] != "Negotiate" {
		t.Errorf("challenge = %q, want a bare Negotiate", got[0])
	}
}

// TestAddWWWAuthenticateAppends: the middleware invites every strategy in the
// chain to add a challenge, and they have to coexist.
func TestAddWWWAuthenticateAppends(t *testing.T) {
	s := newStrategy(t, nil)
	w := httptest.NewRecorder()
	w.Header().Add("WWW-Authenticate", `Basic realm="cernbox"`)

	s.AddWWWAuthenticate(w, requestWith(""), "")

	got := w.Header().Values("WWW-Authenticate")
	if len(got) != 2 {
		t.Fatalf("got %v, want the existing challenge kept alongside Negotiate", got)
	}
}
