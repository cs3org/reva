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

package invoke

import (
	"regexp"
	"strings"
)

// RedactedValue is the placeholder substituted for a secret.
const RedactedValue = "<redacted>"

// sensitiveKeySubstrings are lower-cased fragments that mark a config key as
// secret-bearing (JWT secret, DB DSN, tokens, passwords, api keys, …).
var sensitiveKeySubstrings = []string{
	"secret",
	"token",
	"password",
	"passwd",
	"apikey",
	"api_key",
	"credential",
	"private_key",
	"privatekey",
	"dsn",
}

// dsnWithCredentials matches a connection string with inline credentials,
// e.g. "postgres://user:pass@host/db".
var dsnWithCredentials = regexp.MustCompile(`://[^/@\s:]+:[^/@\s]+@`)

// isSensitiveKey reports whether a config key name looks secret-bearing.
func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Redact returns a deep copy of cfg with secret-looking values masked: keys
// matched by name, string values also checked for inline DSN credentials.
// The input is never modified.
func Redact(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		// A nested table under a sensitive key is a config section, not a
		// secret: recurse instead of hiding it.
		if isSensitiveKey(k) && isScalar(v) {
			out[k] = RedactedValue
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

// isScalar reports whether v is a leaf value (not a nested table or list).
func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return Redact(t)
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = redactValue(e)
		}
		return s
	case string:
		if dsnWithCredentials.MatchString(t) {
			return RedactedValue
		}
		return t
	default:
		return v
	}
}
