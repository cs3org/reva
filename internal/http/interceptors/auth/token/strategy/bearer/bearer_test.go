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

package header

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetTokenFromAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		want       string
	}{
		{"bearer scheme", "Bearer mytoken", "mytoken"},
		{"lowercase scheme", "bearer mytoken", "mytoken"},
		{"uppercase scheme", "BEARER mytoken", "mytoken"},
		// Regression: any non-bearer Authorization header used to be returned as the token.
		{"basic auth is not a bearer token", "Basic dXNlcjpwYXNz", ""},
		{"unknown scheme is not a bearer token", "Negotiate abcdef", ""},
		{"no header", "", ""},
		{"scheme without token", "Bearer", ""},
		{"scheme with empty token", "Bearer ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("PROPFIND", "/webdav/home", nil)
			if tt.authHeader != "" {
				r.Header.Set("Authorization", tt.authHeader)
			}
			if got := (b{}).GetToken(r); got != tt.want {
				t.Errorf("GetToken = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetTokenFallbacks(t *testing.T) {
	t.Run("form body parameter", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/webdav/home", strings.NewReader("access-token=formtoken"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if got := (b{}).GetToken(r); got != "formtoken" {
			t.Errorf("GetToken = %q, want formtoken", got)
		}
	})
	t.Run("query parameter", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/webdav/home?access_token=querytoken", nil)
		if got := (b{}).GetToken(r); got != "querytoken" {
			t.Errorf("GetToken = %q, want querytoken", got)
		}
	})
}
