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

package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		trust      bool
		want       string
		wantErr    bool
	}{
		{
			name:       "peer address when there is no header",
			remoteAddr: "192.0.2.10:54321",
			want:       "192.0.2.10",
		},
		{
			name:       "a spoofed header is ignored while we do not trust it",
			remoteAddr: "192.0.2.10:54321",
			forwarded:  "198.51.100.7",
			want:       "192.0.2.10",
		},
		{
			name:       "the header is read once a proxy is declared",
			remoteAddr: "10.0.0.1:443",
			forwarded:  "198.51.100.7",
			trust:      true,
			want:       "198.51.100.7",
		},
		{
			name:       "chain: the last hop is the one our proxy saw",
			remoteAddr: "10.0.0.1:443",
			forwarded:  "1.2.3.4, 198.51.100.7",
			trust:      true,
			want:       "198.51.100.7",
		},
		{
			name:       "chain with spaces",
			remoteAddr: "10.0.0.1:443",
			forwarded:  "  1.2.3.4 ,  198.51.100.7  ",
			trust:      true,
			want:       "198.51.100.7",
		},
		{
			name:       "empty header falls back to the peer",
			remoteAddr: "192.0.2.10:54321",
			forwarded:  "",
			trust:      true,
			want:       "192.0.2.10",
		},
		{
			name:       "header holding only a comma falls back to the peer",
			remoteAddr: "192.0.2.10:54321",
			forwarded:  ",",
			trust:      true,
			want:       "192.0.2.10",
		},
		{
			name:       "ipv6 peer",
			remoteAddr: "[2001:db8::1]:443",
			want:       "2001:db8::1",
		},
		{
			name:       "peer without a port",
			remoteAddr: "192.0.2.10",
			want:       "192.0.2.10",
		},
		{
			name:       "unparseable peer",
			remoteAddr: "not-an-address",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/ocm/shares", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			got, err := GetClientIP(r, tt.trust)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
