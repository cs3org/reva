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
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Schedule is a parsed periodic schedule. It only needs to answer one
// question: given the previous fire time, when is the next one due.
type Schedule struct {
	interval time.Duration
}

// well-known aliases. These cover the cadence of maintenance-style jobs
// without pulling in a full cron parser. Cron expressions can be added behind
// the same ParseSchedule entry point later if minute-of-hour precision is
// needed.
var aliases = map[string]time.Duration{
	"@hourly": time.Hour,
	"@daily":  24 * time.Hour,
	"@weekly": 7 * 24 * time.Hour,
}

// ParseSchedule parses a schedule spec. The supported grammar is:
//
//	@every <duration>   e.g. "@every 5m", "@every 1h30m"
//	@hourly             every hour
//	@daily              every 24 hours
//	@weekly             every 7 days
//
// <duration> is anything accepted by time.ParseDuration.
func ParseSchedule(spec string) (Schedule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Schedule{}, errors.New("rjobs: empty schedule")
	}

	if d, ok := aliases[spec]; ok {
		return Schedule{interval: d}, nil
	}

	if rest, ok := strings.CutPrefix(spec, "@every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return Schedule{}, errors.Wrapf(err, "rjobs: invalid duration in schedule %q", spec)
		}
		if d <= 0 {
			return Schedule{}, errors.Errorf("rjobs: schedule interval must be positive, got %q", spec)
		}
		return Schedule{interval: d}, nil
	}

	return Schedule{}, errors.Errorf("rjobs: unsupported schedule %q", spec)
}

// Interval returns the configured interval.
func (s Schedule) Interval() time.Duration {
	return s.interval
}

// Next returns the next fire time after prev.
func (s Schedule) Next(prev time.Time) time.Time {
	return prev.Add(s.interval)
}
