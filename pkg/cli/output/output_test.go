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

package output

import "testing"

func TestHumanSize(t *testing.T) {
	for _, tt := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{3145728, "3.0M"},
		{1073741824, "1.0G"},
		{1099511627776, "1.0T"},
		{1125899906842624, "1.0P"},
	} {
		if got := HumanSize(tt.in); got != tt.want {
			t.Errorf("HumanSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
