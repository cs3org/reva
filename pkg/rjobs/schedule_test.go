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

package rjobs

import (
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		spec    string
		want    time.Duration
		wantErr bool
	}{
		{spec: "@every 5m", want: 5 * time.Minute},
		{spec: "@every 1h30m", want: 90 * time.Minute},
		{spec: "@every 30s", want: 30 * time.Second},
		{spec: "@hourly", want: time.Hour},
		{spec: "@daily", want: 24 * time.Hour},
		{spec: "@weekly", want: 7 * 24 * time.Hour},
		{spec: "  @hourly  ", want: time.Hour},
		{spec: "", wantErr: true},
		{spec: "@every", wantErr: true},
		{spec: "@every 0s", wantErr: true},
		{spec: "@every -5m", wantErr: true},
		{spec: "@every notaduration", wantErr: true},
		{spec: "@yearly", wantErr: true},
		{spec: "*/5 * * * *", wantErr: true},
	}

	for _, tt := range tests {
		s, err := ParseSchedule(tt.spec)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseSchedule(%q): expected error, got none", tt.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSchedule(%q): unexpected error: %v", tt.spec, err)
			continue
		}
		if s.Interval() != tt.want {
			t.Errorf("ParseSchedule(%q): interval = %v, want %v", tt.spec, s.Interval(), tt.want)
		}
	}
}

func TestScheduleNext(t *testing.T) {
	s, err := ParseSchedule("@every 10m")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if got := s.Next(now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Errorf("Next() = %v, want %v", got, now.Add(10*time.Minute))
	}
}
