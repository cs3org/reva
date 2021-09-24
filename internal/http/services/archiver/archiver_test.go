// Copyright 2018-2021 CERN
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

package archiver

import "testing"

func TestGetDeepestCommonDir(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "no paths",
			paths:    []string{},
			expected: "",
		},
		{
			name:     "one path",
			paths:    []string{"/aa/bb/cc"},
			expected: "/aa/bb/cc",
		},
		{
			name:     "root as common parent",
			paths:    []string{"/aa/bb/bb", "/bb/cc"},
			expected: "/",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc", "/aa/bb/dd"},
			expected: "/aa/bb",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc", "/aa/bb/dd", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "common parent",
			paths:    []string{"/aa/bb/cc/", "/aa/bb/dd/", "/aa/test/"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa", "/aa/bb/dd", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa/", "/aa/bb/dd/", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/aa/bb/dd", "/aa/", "/aa/test"},
			expected: "/aa",
		},
		{
			name:     "one is common parent",
			paths:    []string{"/reva/einstein/test", "/reva/einstein"},
			expected: "/reva/einstein",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			res := getDeepestCommonDir(tt.paths)
			if res != tt.expected {
				t.Errorf("getDeepestCommondDir() failed: paths=%+v expected=%s got=%s", tt.paths, tt.expected, res)
			}

		})

	}
}
