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

import (
	"strings"
)

var (
	// character used to separate tags in lists
	_tagsep = ","
	// maximum number of tags
	_maxtags = 100
)

// Tags is a helper struct for merging, deleting and deduplicating the tags while preserving the order
type Tags struct {
	sep     string
	maxtags int

	t       []string
	exists  map[string]bool
	numtags int
}

// FromList creates a Tags struct from a list of tags
func FromList(s string) *Tags {
	t := &Tags{sep: _tagsep, maxtags: _maxtags, exists: make(map[string]bool)}
	t.t = t.addTags(s)
	return t
}

// AddList appends a list of new tags and returns true if at least one was appended
func (t *Tags) AddList(s string) bool {
	tags := t.addTags(s)
	t.t = append(tags, t.t...)
	return len(tags) > 0
}

// RemoveList removes a list of tags and returns true if at least one was removed
func (t *Tags) RemoveList(s string) bool {
	var removed bool
	for _, tag := range strings.Split(s, t.sep) {
		if !t.exists[tag] {
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

// AsList returns the tags converted to a list
func (t *Tags) AsList() string {
	return strings.Join(t.t, t.sep)
}

// AsSlice returns the tags as slice of strings
func (t *Tags) AsSlice() []string {
	return t.t
}

// adds the tags and returns a list of added tags
func (t *Tags) addTags(s string) []string {
	added := make([]string, 0)
	for _, tag := range strings.Split(s, t.sep) {
		if tag == "" {
			// ignore empty tags
			continue
		}

		if t.exists[tag] {
			// tag is already existing
			continue
		}

		if t.numtags >= t.maxtags {
			// max number of tags reached. We return silently without warning anyone
			break
		}

		added = append(added, tag)
		t.exists[tag] = true
		t.numtags++
	}

	return added
}
