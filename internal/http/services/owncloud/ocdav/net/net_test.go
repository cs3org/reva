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

package net

import "testing"

func TestParseDepth(t *testing.T) {
	tests := map[string]Depth{
		"":         DepthOne,
		"0":        DepthZero,
		"1":        DepthOne,
		"infinity": DepthInfinity,
	}

	for input, expected := range tests {
		parsed, err := ParseDepth(input)
		if err != nil {
			t.Errorf("failed to parse depth %s", input)
		}
		if parsed != expected {
			t.Errorf("parseDepth returned %s expected %s", parsed.String(), expected.String())
		}
	}

	_, err := ParseDepth("invalid")
	if err == nil {
		t.Error("parse depth didn't return an error for invalid depth: invalid")
	}
}

var result Depth

func BenchmarkParseDepth(b *testing.B) {
	inputs := []string{"", "0", "1", "infinity", "INFINITY"}
	size := len(inputs)
	for i := 0; i < b.N; i++ {
		result, _ = ParseDepth(inputs[i%size])
	}
}
