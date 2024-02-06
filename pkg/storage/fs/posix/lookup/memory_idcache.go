// Copyright 2018-2021 CERN
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

package lookup

import "context"

type MemoryIDCache struct {
	cache map[string]map[string]string
}

// NewMemoryIDCache returns a new MemoryIDCache
func NewMemoryIDCache() *MemoryIDCache {
	return &MemoryIDCache{
		cache: make(map[string]map[string]string),
	}
}

// Add adds a new entry to the cache
func (c *MemoryIDCache) Set(_ context.Context, spaceID, nodeID, val string) error {
	spaceCache := c.cache[spaceID]
	if spaceCache == nil {
		spaceCache = make(map[string]string)
		c.cache[spaceID] = spaceCache
	}
	spaceCache[nodeID] = val
	return nil
}

// Get returns the value for a given key
func (c *MemoryIDCache) Get(_ context.Context, spaceID, nodeID string) (string, bool) {
	spaceCache, ok := c.cache[spaceID]
	if !ok {
		return "", false
	}
	val, ok := spaceCache[nodeID]
	return val, ok
}
