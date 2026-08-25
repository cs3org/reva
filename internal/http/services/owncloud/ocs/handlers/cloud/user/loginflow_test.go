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

package user

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDeviceName(t *testing.T) {
	bell := "\a"
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
		{"control char rejected", `{"name":"bad` + bell + `name"}`, "", true},
		{"pipe rejected", `{"name":"a|b"}`, "", true},
		{"malformed json", `{"name":`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login-flow/x/grant", strings.NewReader(c.body))
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

func TestParseLabel(t *testing.T) {
	cases := []struct {
		label           string
		wantName        string
		wantDescription string
		wantCID         string
	}{
		{"my laptop|Nextcloud Sync Client v3.16.0 (on Linux)|abc-123", "my laptop", "Nextcloud Sync Client v3.16.0 (on Linux)", "abc-123"},
		{"|Nextcloud Sync Client v3.16.0|abc-123", "", "Nextcloud Sync Client v3.16.0", "abc-123"},
		{"whole", "", "whole", ""},
	}
	for _, c := range cases {
		name, description, cid := parseLabel(c.label)
		if name != c.wantName || description != c.wantDescription || cid != c.wantCID {
			t.Errorf("parseLabel(%q) = (%q,%q,%q), want (%q,%q,%q)", c.label, name, description, cid, c.wantName, c.wantDescription, c.wantCID)
		}
	}
}
