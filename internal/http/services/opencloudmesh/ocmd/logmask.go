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
	"slices"
	"strings"
)

const secretLogPlaceholder = "***"

func newShareRequestSecrets(r *NewShareRequest) []string {
	if r == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var secrets []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		secrets = append(secrets, s)
	}
	add(r.Code)
	for _, p := range r.Protocols {
		switch v := p.(type) {
		case *WebDAV:
			add(v.SharedSecret)
		case *Webapp:
			add(v.SharedSecret)
		}
	}
	slices.SortFunc(secrets, func(a, b string) int {
		return len(b) - len(a)
	})
	return secrets
}

func maskNewShareSecrets(r *NewShareRequest, s string) string {
	return maskSecrets(newShareRequestSecrets(r), s)
}

func maskSecrets(secrets []string, s string) string {
	for _, secret := range secrets {
		s = strings.ReplaceAll(s, secret, secretLogPlaceholder)
		escaped, err := json.Marshal(secret)
		if err != nil || len(escaped) < 2 {
			continue
		}
		inner := string(escaped[1 : len(escaped)-1])
		if inner != secret {
			s = strings.ReplaceAll(s, inner, secretLogPlaceholder)
		}
	}
	return s
}
