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

package sync

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cache is a barebones cache implementation.
type Cache struct {
	// capacity and length have to be the first words
	// in order to be 64-aligned on 32-bit architectures.
	capacity, length uint64 // access atomically
	entries  sync.Map
	pool     sync.Pool
}

// CacheEntry represents an entry on the cache. You can type assert on V.
type CacheEntry struct {
	V          interface{}
	expiration time.Time
}

// NewCache returns a new instance of Cache.
func NewCache(capacity int) Cache {
	return Cache{
		capacity: uint64(capacity),
		pool: sync.Pool{New: func() interface{} {
			return new(CacheEntry)
		}},
	}
}

// Load loads an entry by given key
func (c *Cache) Load(key string) *CacheEntry {
	if mapEntry, ok := c.entries.Load(key); ok {
		entry := mapEntry.(*CacheEntry)
		if c.expired(entry) {
			c.entries.Delete(key)
			return nil
		}
		return entry
	}
	return nil
}

// Store adds an entry for given key and value
func (c *Cache) Store(key string, val interface{}, expiration time.Time) {
	if c.length > c.capacity {
		c.evict()
	}

	poolEntry := c.pool.Get() //nolint: ifshort
	if mapEntry, loaded := c.entries.LoadOrStore(key, poolEntry); loaded {
		entry := mapEntry.(*CacheEntry)
		entry.V = val
		entry.expiration = expiration

		c.pool.Put(poolEntry)
	} else {
		entry := poolEntry.(*CacheEntry)
		entry.V = val
		entry.expiration = expiration

		atomic.AddUint64(&c.length, 1)
	}
}

// Delete removes an entry by given key
func (c *Cache) Delete(key string) bool {
	_, loaded := c.entries.LoadAndDelete(key)

	if loaded {
		atomic.AddUint64(&c.length, ^uint64(0))
	}

	return loaded
}

// evict frees memory from the cache by removing entries that exceeded the cache TTL.
func (c *Cache) evict() {
	c.entries.Range(func(key, mapEntry interface{}) bool {
		entry := mapEntry.(*CacheEntry)
		if c.expired(entry) {
			c.Delete(key.(string))
		}
		return true
	})
}

// expired checks if an entry is expired
func (c *Cache) expired(e *CacheEntry) bool {
	return e.expiration.Before(time.Now())
}
