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
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskNewShareSecrets(t *testing.T) {
	t.Parallel()

	webdavSecret := "webdav-top-secret"
	webappSecret := "webapp-top-secret"
	codeSecret := "auth-code-nonce"
	jsonEscSecret := `quote"slash\`

	reqWebDAV := func(secret string) *NewShareRequest {
		return &NewShareRequest{
			ShareWith: "user@remote.example",
			Name:      "notes.txt",
			Protocols: Protocols{
				&WebDAV{
					SharedSecret: secret,
					Permissions:  []string{"read"},
					URI:          "https://example.com/dav",
				},
			},
		}
	}

	t.Run("webdav secret", func(t *testing.T) {
		body, err := reqWebDAV(webdavSecret).toJSON()
		if err != nil {
			t.Fatal(err)
		}
		masked := maskNewShareSecrets(reqWebDAV(webdavSecret), string(body))
		if strings.Contains(masked, webdavSecret) {
			t.Fatalf("masked payload still contains webdav secret: %s", masked)
		}
		if !strings.Contains(masked, secretLogPlaceholder) {
			t.Fatalf("masked payload missing placeholder: %s", masked)
		}
	})

	t.Run("webapp secret", func(t *testing.T) {
		req := &NewShareRequest{
			ShareWith: "user@remote.example",
			Name:      "notes.txt",
			Protocols: Protocols{
				&Webapp{
					URI:          "https://example.com/app",
					SharedSecret: webappSecret,
					Permissions:  []string{"read"},
					Requirements: []string{"must-exchange-token"},
					Targets:      []string{"blank"},
				},
			},
		}
		body, err := req.toJSON()
		if err != nil {
			t.Fatal(err)
		}
		masked := maskNewShareSecrets(req, string(body))
		if strings.Contains(masked, webappSecret) {
			t.Fatalf("masked payload still contains webapp secret: %s", masked)
		}
	})

	t.Run("legacy options block", func(t *testing.T) {
		raw := `{"name":"ocm10format","options":{"sharedSecret":"legacy-options-secret"}}`
		var protocols Protocols
		if err := json.Unmarshal([]byte(raw), &protocols); err != nil {
			t.Fatal(err)
		}
		req := &NewShareRequest{
			ShareWith: "user@remote.example",
			Name:      "notes.txt",
			Protocols: protocols,
		}
		legacyJSON := `{"protocol":{"name":"ocm10format","options":{"sharedSecret":"legacy-options-secret"}}}`
		masked := maskNewShareSecrets(req, legacyJSON)
		if strings.Contains(masked, "legacy-options-secret") {
			t.Fatalf("masked legacy options still contain secret: %s", masked)
		}
	})

	t.Run("code", func(t *testing.T) {
		req := reqWebDAV(webdavSecret)
		req.Code = codeSecret
		body, err := req.toJSON()
		if err != nil {
			t.Fatal(err)
		}
		masked := maskNewShareSecrets(req, string(body))
		if strings.Contains(masked, codeSecret) {
			t.Fatalf("masked payload still contains code: %s", masked)
		}
	})

	t.Run("json escaped secret", func(t *testing.T) {
		req := reqWebDAV(jsonEscSecret)
		body, err := req.toJSON()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `\`) {
			t.Fatalf("fixture body should contain escaped characters: %s", body)
		}
		masked := maskNewShareSecrets(req, string(body))
		if strings.Contains(masked, jsonEscSecret) {
			t.Fatalf("masked payload still contains raw secret: %s", masked)
		}
		escaped, err := json.Marshal(jsonEscSecret)
		if err != nil {
			t.Fatal(err)
		}
		inner := string(escaped[1 : len(escaped)-1])
		if strings.Contains(masked, inner) {
			t.Fatalf("masked payload still contains json-escaped secret: %s", masked)
		}
	})

	t.Run("empty secret is a no-op", func(t *testing.T) {
		req := reqWebDAV("")
		body, err := req.toJSON()
		if err != nil {
			t.Fatal(err)
		}
		masked := maskNewShareSecrets(req, string(body))
		if masked != string(body) {
			t.Fatalf("empty secret changed payload: %q vs %q", masked, body)
		}
	})

	t.Run("overlapping secrets", func(t *testing.T) {
		long := "abcdefgh"
		short := "abc"
		req := &NewShareRequest{
			ShareWith: "user@remote.example",
			Name:      "notes.txt",
			Code:      short,
			Protocols: Protocols{
				&WebDAV{
					SharedSecret: long,
					Permissions:  []string{"read"},
					URI:          "https://example.com/dav",
				},
			},
		}
		text := "prefix " + long + " middle " + short + " suffix"
		masked := maskNewShareSecrets(req, text)
		if strings.Contains(masked, long) || strings.Contains(masked, short) {
			t.Fatalf("overlapping secrets not fully masked: %s", masked)
		}
		if strings.Count(masked, secretLogPlaceholder) < 2 {
			t.Fatalf("expected both secrets masked separately: %s", masked)
		}
	})
}
