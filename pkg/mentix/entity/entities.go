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

package entity

// Collection is an interface for entity collections.
type Collection interface {
	// Entities returns a vector of entities within the collection.
	Entities() []Entity
}

// GetIDs gets a list of entity IDs.
func GetIDs(collection Collection) []string {
	entities := collection.Entities()
	ids := make([]string, 0, len(entities))
	for _, entity := range entities {
		ids = append(ids, entity.GetID())
	}
	return ids
}

// GetNames gets a list of entity names.
func GetNames(collection Collection) []string {
	entities := collection.Entities()
	names := make([]string, 0, len(entities))
	for _, entity := range entities {
		names = append(names, entity.GetName())
	}
	return names
}
