// Copyright 2018-2022 CERN
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

package rhttp

import "testing"

func TestURLHasPrefix(t *testing.T) {
	tests := map[string]struct {
		url      string
		prefix   string
		expected bool
	}{
		"root": {
			url:      "/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root": {
			url:      "/api/v0",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_slash_end": {
			url:      "/api/v0/",
			prefix:   "/",
			expected: true,
		},
		"suburl_root_no_slash": {
			url:      "/api/v0",
			prefix:   "",
			expected: true,
		},
		"no_common_prefix": {
			url:      "/api/v0/project",
			prefix:   "/api/v0/p",
			expected: false,
		},
		"long_url_prefix": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0",
			expected: true,
		},
		"prefix_end_slash": {
			url:      "/api/v0/project/test",
			prefix:   "/api/v0/",
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			res := urlHasPrefix(test.url, test.prefix)
			if res != test.expected {
				t.Fatalf("%s got an unexpected result: %+v instead of %+v", t.Name(), res, test.expected)
			}
		})
	}
}
