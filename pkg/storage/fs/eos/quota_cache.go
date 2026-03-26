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

package eos

import (
	"sync"
	"time"

	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
)

type quotaCacheEntry struct {
	info       eosclient.QuotaInfo
	fetchedAt  time.Time
	refreshing bool
}

type quotaCache struct {
	mu      sync.RWMutex
	entries map[string]*quotaCacheEntry
	ttl     time.Duration
}

func newQuotaCache(ttl time.Duration) *quotaCache {
	return &quotaCache{
		entries: make(map[string]*quotaCacheEntry),
		ttl:     ttl,
	}
}

// get returns a copy of the cache entry and whether it was found.
func (c *quotaCache) get(key string) (quotaCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok {
		return quotaCacheEntry{}, false
	}
	return *e, true
}

// set stores (or updates) a cache entry, clearing the refreshing flag.
func (c *quotaCache) set(key string, info *eosclient.QuotaInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &quotaCacheEntry{
		info:      *info,
		fetchedAt: time.Now(),
	}
}

// tryMarkRefreshing atomically marks the entry as being refreshed in the background.
// Returns true if it succeeded (entry exists and wasn't already marked as refreshing).
func (c *quotaCache) tryMarkRefreshing(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || e.refreshing {
		return false
	}
	e.refreshing = true
	return true
}

// clearRefreshing clears the refreshing flag, e.g. after a background refresh fails.
func (c *quotaCache) clearRefreshing(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		e.refreshing = false
	}
}
