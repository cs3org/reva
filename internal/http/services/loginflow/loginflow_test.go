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

package loginflow

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDeviceName(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{"empty body", "", "", false},
		{"empty json", `{}`, "", false},
		{"plain name", `{"name":"my laptop"}`, "my laptop", false},
		{"trimmed", `{"name":"  pad  "}`, "pad", false},
		{"64 runes ok", `{"name":"` + strings.Repeat("a", 64) + `"}`, strings.Repeat("a", 64), false},
		{"65 runes rejected", `{"name":"` + strings.Repeat("a", 65) + `"}`, "", true},
		{"control char rejected", `{"name":"bad\u0007name"}`, "", true},
		{"malformed json", `{"name":`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body *strings.Reader
			if c.body == "" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(c.body)
			}
			r := httptest.NewRequest("POST", "/flow/x/grant", body)
			got, err := parseDeviceName(r)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got name %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestParseUserAgent(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (Linux) mirall/3.16.0 (Nextcloud)", "Nextcloud Desktop 3.16.0 on Linux"},
		{"Nextcloud-Desktop/3.16.0 (Windows)", "Nextcloud Desktop 3.16.0 on Windows"},
		{"", "Unknown client"},
		{"curl/8.0", "curl/8.0"},
	}
	for _, c := range cases {
		if got := parseUserAgent(c.ua); got != c.want {
			t.Errorf("parseUserAgent(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}

func TestSplitOrigin(t *testing.T) {
	s := &svc{c: &config{WebUIURL: "https://cernbox.cern.ch"}}
	cases := []struct {
		origin string
		want   bool
	}{
		{"https://cernbox.cern.ch", true},
		{"https://cernbox.cern.ch/", true},
		{"https://evil.example.com", false},
		{"", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/flow/x/grant", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := s.originAllowed(r); got != c.want {
			t.Errorf("originAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}
