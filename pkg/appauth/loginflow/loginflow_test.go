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

import "testing"

func TestClientDescription(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 (Linux) mirall/3.16.0 (Nextcloud)", "Nextcloud Sync Client v3.16.0 (on Linux)"},
		{"Nextcloud-Desktop/3.16.0 (Windows)", "Nextcloud Sync Client v3.16.0 (on Windows)"},
		{"mirall/3.16.0", "Nextcloud Sync Client v3.16.0"},
		{"", "Unknown client"},
		{"curl/8.0", "curl/8.0"},
	}
	for _, c := range cases {
		if got := ClientDescription(c.ua); got != c.want {
			t.Errorf("ClientDescription(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}
