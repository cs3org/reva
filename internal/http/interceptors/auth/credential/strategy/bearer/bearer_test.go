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

package bearer

import (
	"net/http/httptest"
	"testing"
)

func TestGetCredentials(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		target     string
		wantToken  string
		wantErr    bool
	}{
		{"bearer scheme", "Bearer mytoken", "/webdav/home", "mytoken", false},
		{"lowercase scheme", "bearer mytoken", "/webdav/home", "mytoken", false},
		{"uppercase scheme", "BEARER mytoken", "/webdav/home", "mytoken", false},
		// Regression: any non-bearer Authorization header used to be returned as a bearer token.
		{"basic auth is not a bearer token", "Basic dXNlcjpwYXNz", "/webdav/home", "", true},
		{"unknown scheme is not a bearer token", "Negotiate abcdef", "/webdav/home", "", true},
		{"no header", "", "/webdav/home", "", true},
		{"query parameter fallback", "", "/webdav/home?access_token=querytoken", "querytoken", false},
		{"scheme without token", "Bearer", "/webdav/home", "", true},
		{"scheme with empty token", "Bearer ", "/webdav/home", "", true},
	}

	s := &strategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("PROPFIND", tt.target, nil)
			if tt.authHeader != "" {
				r.Header.Set("Authorization", tt.authHeader)
			}
			creds, err := s.GetCredentials(nil, r)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got credentials %+v", creds)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if creds.Type != "bearer" {
				t.Errorf("credentials type: got %q, want bearer", creds.Type)
			}
			if creds.ClientSecret != tt.wantToken {
				t.Errorf("client secret: got %q, want %q", creds.ClientSecret, tt.wantToken)
			}
		})
	}
}
