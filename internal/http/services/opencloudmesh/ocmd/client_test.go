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

package ocmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestExchangeTokenSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "jwt-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	tok, exp, err := c.ExchangeToken(context.Background(), srv.URL, "code123", "client1")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "jwt-tok" {
		t.Errorf("access_token: got %q, want jwt-tok", tok)
	}
	if exp != 3600 {
		t.Errorf("expires_in: got %d, want 3600", exp)
	}
}

func TestExchangeTokenInvalidGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "bad-code", "")
	if err == nil {
		t.Fatal("expected error for invalid_grant")
	}
	if _, ok := err.(errtypes.InvalidCredentials); !ok {
		t.Errorf("expected InvalidCredentials, got %T: %v", err, err)
	}
}

func TestExchangeTokenUnsupportedGrantType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for unsupported_grant_type")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if _, ok := err.(errtypes.PermissionDenied); !ok {
		t.Errorf("expected PermissionDenied, got %T: %v", err, err)
	}
}

func TestExchangeTokenUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if _, ok := err.(errtypes.PermissionDenied); !ok {
		t.Errorf("expected PermissionDenied, got %T: %v", err, err)
	}
}

func TestExchangeTokenServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenMissingAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type": "Bearer",
			"expires_in": 3600,
		})
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
	if _, ok := err.(errtypes.InternalError); !ok {
		t.Errorf("expected InternalError, got %T: %v", err, err)
	}
}

func TestExchangeTokenMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(10*time.Second, true)
	_, _, err := c.ExchangeToken(context.Background(), srv.URL, "code", "")
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}
