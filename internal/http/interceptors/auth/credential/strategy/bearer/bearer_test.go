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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetCredentials(t *testing.T) {
	strat, err := New(nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		setHeaders func(*http.Request)
		want       string
		wantErr    bool
	}{
		{
			name:   "header precedence over form body",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch",
			body:   "access_token=body-token&file=x",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer header-token")
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			want: "header-token",
		},
		{
			name:   "form body",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch",
			body:   "access_token=body-token&file=x",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			want: "body-token",
		},
		{
			name:   "form body beats query",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch?access_token=query-token",
			body:   "access_token=body-token&file=x",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			want: "body-token",
		},
		{
			name:   "query parameter",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch?access_token=query-token",
			body:   "",
			want:   "query-token",
		},
		{
			name:   "wrong content type ignores body",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch?access_token=query-token",
			body:   `{"access_token":"body-token"}`,
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/json")
			},
			want: "query-token",
		},
		{
			name:   "GET ignores form body",
			method: http.MethodGet,
			target: "/sciencemesh/webapp-launch?access_token=query-token",
			body:   "access_token=body-token",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			want: "query-token",
		},
		{
			name:   "PUT ignores form body",
			method: http.MethodPut,
			target: "/sciencemesh/webapp-launch?access_token=query-token",
			body:   "access_token=body-token",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			want: "query-token",
		},
		{
			name:   "no credentials",
			method: http.MethodPost,
			target: "/sciencemesh/webapp-launch",
			body:   "file=x",
			setHeaders: func(r *http.Request) {
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.setHeaders != nil {
				tt.setHeaders(req)
			}

			creds, err := strat.GetCredentials(nil, req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("GetCredentials() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetCredentials() error: %v", err)
			}
			if creds.ClientSecret != tt.want {
				t.Fatalf("ClientSecret = %q, want %q", creds.ClientSecret, tt.want)
			}
			if creds.Type != "bearer" {
				t.Fatalf("Type = %q, want bearer", creds.Type)
			}
		})
	}
}

func TestGetCredentialsFormAlreadyParsed(t *testing.T) {
	strat, err := New(nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/sciencemesh/webapp-launch",
		strings.NewReader("access_token=preparsed-token&file=x"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}

	creds, err := strat.GetCredentials(nil, req)
	if err != nil {
		t.Fatalf("GetCredentials() error: %v", err)
	}
	if creds.ClientSecret != "preparsed-token" {
		t.Fatalf("ClientSecret = %q, want preparsed-token", creds.ClientSecret)
	}
}
