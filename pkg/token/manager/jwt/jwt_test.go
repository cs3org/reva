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

package jwt

import (
	"testing"
	"time"
)

func TestGetNextWeekend(t *testing.T) {
	tests := []struct {
		now      time.Time
		expected time.Time
	}{
		{
			now:      time.Date(2022, time.December, 12, 0, 0, 0, 0, time.UTC), // mon
			expected: time.Date(2022, time.December, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			now:      time.Date(2022, time.December, 13, 0, 0, 0, 0, time.UTC), // tue
			expected: time.Date(2022, time.December, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			now:      time.Date(2022, time.December, 16, 0, 0, 0, 0, time.UTC), // fri
			expected: time.Date(2022, time.December, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			now:      time.Date(2022, time.December, 17, 0, 0, 0, 0, time.UTC), // saturday
			expected: time.Date(2022, time.December, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			now:      time.Date(2022, time.December, 18, 0, 0, 0, 0, time.UTC), // sunday
			expected: time.Date(2022, time.December, 25, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		res := getNextWeekend(tt.now)
		if !res.Equal(tt.expected) {
			t.Fatalf("expected and returned result differs: expected=%s res=%s", tt.expected.String(), res.String())
		}
	}
}

func TestSetTi(t *testing.T) {
	tests := []struct {
		t              time.Time
		hour, min, sec int
		expected       time.Time
	}{
		{
			t:        time.Date(2022, time.December, 12, 0, 0, 0, 0, time.UTC),
			hour:     23,
			min:      34,
			sec:      10,
			expected: time.Date(2022, time.December, 12, 23, 34, 10, 0, time.UTC),
		},
				{
			t:        time.Date(2022, time.December, 12, 20, 19, 10, 0, time.UTC),
			hour:     23,
			min:      34,
			sec:      10,
			expected: time.Date(2022, time.December, 12, 23, 34, 10, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		res := setTime(tt.t, tt.hour, tt.min, tt.sec)
		if !res.Equal(tt.expected) {
			t.Fatalf("expected and returned result differs: expected=%s res=%s", tt.expected.String(), res.String())
		}
	}
}
