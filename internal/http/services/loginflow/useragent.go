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

import "strings"

// parseUserAgent turns a raw User-Agent into a short human label for the
// confirmation page and the app password. It recognises the NextCloud desktop
// client (which sends a "mirall/<version>" token) and falls back to the raw
// string for anything else.
func parseUserAgent(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return "Unknown client"
	}

	os := ""
	if l := strings.Index(ua, "("); l >= 0 {
		if r := strings.Index(ua[l+1:], ")"); r >= 0 {
			os = strings.TrimSpace(ua[l+1 : l+1+r])
		}
	}

	if v, ok := uaToken(ua, "mirall/"); ok {
		label := "Nextcloud Desktop " + v
		if os != "" {
			label += " on " + os
		}
		return label
	}
	if v, ok := uaToken(ua, "Nextcloud-Desktop/"); ok {
		label := "Nextcloud Desktop " + v
		if os != "" {
			label += " on " + os
		}
		return label
	}

	return ua
}

// uaToken returns the whitespace-delimited value that follows prefix in s.
func uaToken(s, prefix string) (string, bool) {
	_, rest, ok := strings.Cut(s, prefix)
	if !ok {
		return "", false
	}
	if f := strings.Fields(rest); len(f) > 0 {
		return f[0], true
	}
	return "", false
}
