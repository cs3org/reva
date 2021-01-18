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

package crypto

import (
	"io"
	"strings"
	"testing"
)

func TestChecksums(t *testing.T) {
	tests := map[string]struct {
		xsFunc     func(r io.Reader) (string, error)
		input      string
		expectedXS string
	}{
		"adler32_hello": {ComputeAdler32XS, "Hello World!", "1c49043e"},
		"sha1_hello":    {ComputeSHA1XS, "Hello World!", "2ef7bde608ce5404e97d5f042f95f89f1c232871"},
		"md5_hello":     {ComputeMD5XS, "Hello World!", "ed076287532e86365e841e92bfc50d8c"},
	}

	for name := range tests {
		var tc = tests[name]
		t.Run(name, func(t *testing.T) {
			actual, err := tc.xsFunc(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("%v returned an unexpected error: %v", t.Name(), err)
			}

			if actual != tc.expectedXS {
				t.Fatalf("%v returned wrong checksum:\n\tAct: %v\n\tExp: %v", t.Name(), actual, tc.expectedXS)
			}
		})
	}
}
