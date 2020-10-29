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

type RegistryEntry interface {
}

type Registry struct {
	Entries map[string]RegistryEntry
}

func (r *Registry) Register(id string, entry RegistryEntry) {
	r.Entries[id] = entry
}

func (r *Registry) EntriesByID(ids []string) ([]RegistryEntry, error) {
	var entries []RegistryEntry
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
		Entries: make(map[string]RegistryEntry),
	}
}
