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

package appauth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetCredentials(t *testing.T) {
	pw := "Im8DoiYG4SPameem_8zKJmERWW0zdRKuVEDOR5nyUZM"
	cases := []struct {
		name         string
		basic        []string
		bearer       string
		presentation string // empty when no credentials are claimed
		clientID     string
	}{
		{"basic auth", []string{"einstein", pw}, "", "basic", "einstein"},
		{"bearer", nil, pw, "bearer", ""},
		{"regular password", []string{"einstein", "relativity"}, "", "", ""},
		{"JWT", nil, "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln", "", ""},
		{"too short", nil, pw[:42], "", ""},
		{"not base64url", nil, strings.Replace(pw, "_", "+", 1), "", ""},
	}
	s, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/remote.php/webdav", nil)
		if c.basic != nil {
			r.SetBasicAuth(c.basic[0], c.basic[1])
		}
		if c.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+c.bearer)
		}
		creds, _ := s.GetCredentials(nil, r)
		switch {
		case c.presentation == "" && creds != nil:
			t.Errorf("%s: claimed %+v", c.name, creds)
		case c.presentation != "" && (creds == nil || creds.Type != "appauth" || creds.Presentation != c.presentation || creds.ClientID != c.clientID):
			t.Errorf("%s: credentials %+v", c.name, creds)
		}
	}
}
