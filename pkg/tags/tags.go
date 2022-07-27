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

package tags

import "strings"

// character to separate tags - do we need this configurable?
var _tagsep = ","

// Tags is a helper struct for merging, deleting and deduplicating the tags while preserving the order
type Tags struct {
	t      []string
	sep    string
	exists map[string]bool
}

// FromString creates a Tags struct from a string
func FromString(s string) *Tags {
	t := &Tags{sep: _tagsep, exists: make(map[string]bool)}

	tags := strings.Split(s, t.sep)
	for _, tag := range tags {
		t.t = append(t.t, tag)
		t.exists[tag] = true
	}
	return t
}

// AddString appends the the new tags and returns true if at least one was appended
func (t *Tags) AddString(s string) bool {
	var tags []string
	for _, tag := range strings.Split(s, t.sep) {
		if !t.exists[tag] {
			tags = append(tags, tag)
			t.exists[tag] = true
		}
	}

	t.t = append(tags, t.t...)
	return len(tags) != 0
}

// RemoveString removes the the tags and returns true if at least one was removed
func (t *Tags) RemoveString(s string) bool {
	var removed bool
	for _, tag := range strings.Split(s, t.sep) {
		if !t.exists[tag] {
			// should this be reported?
			continue
		}

		for i, tt := range t.t {
			if tt == tag {
				t.t = append(t.t[:i], t.t[i+1:]...)
				break
			}
		}

		delete(t.exists, tag)
		removed = true
	}
	return removed
}

// AsString returns the tags converted to a string
func (t *Tags) AsString() string {
	return strings.Join(t.t, t.sep)
}
