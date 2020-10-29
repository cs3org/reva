// Copyright 2018-2020 CERN
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

package registry

import "fmt"

// Registry represents a simple id->entry map.
type Registry struct {
	Entries map[string]interface{}
}

// Register registers a new entry.
func (r *Registry) Register(id string, entry interface{}) {
	r.Entries[id] = entry
}

// EntriesByID returns all entries matching the provided IDs. If an entry with a certain ID doesn't exist, an error is returned.
func (r *Registry) EntriesByID(ids []string) ([]interface{}, error) {
	var entries []interface{}
	for _, id := range ids {
		if entry, ok := r.Entries[id]; ok {
			entries = append(entries, entry)
		} else {
			return nil, fmt.Errorf("no entry with ID '%v' registered", id)
		}
	}

	return entries, nil
}

func NewRegistry() *Registry {
	return &Registry{
		Entries: make(map[string]interface{}),
	}
}
