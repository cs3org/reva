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

// Package ratelimit holds a per-key token bucket, for limits per source IP or
// per credential.
package ratelimit

import (
	"sync"
	"time"
)

// maxBuckets bounds the memory taken by keys an attacker chooses, such as
// source IPs: past it, the buckets that have refilled are dropped.
const maxBuckets = 100000

// Limiter is a per-key token bucket.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   float64
	last     time.Time
	capacity float64
	refill   float64 // tokens per second
}

// New returns an empty limiter.
func New() *Limiter {
	return &Limiter{buckets: make(map[string]*bucket)}
}

// Allow takes one token from the bucket named key, sized for perMin requests
// per minute. It returns false when the bucket is empty. perMin <= 0 disables
// the limit.
func (l *Limiter) Allow(key string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.refilled(key, perMin)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// AllowN takes n tokens from each bucket named in keys, sized for perMin
// tokens per minute, or from none of them when one holds fewer than n. When it
// refuses, it also returns how long until they all hold n. perMin <= 0
// disables the limit.
func (l *Limiter) AllowN(keys []string, perMin, n int) (bool, time.Duration) {
	if perMin <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var wait time.Duration
	buckets := make([]*bucket, 0, len(keys))
	for _, k := range keys {
		b := l.refilled(k, perMin)
		if missing := float64(n) - b.tokens; missing > 0 {
			wait = max(wait, time.Duration(missing/b.refill*float64(time.Second)))
		}
		buckets = append(buckets, b)
	}
	if wait > 0 {
		return false, wait
	}
	for _, b := range buckets {
		b.tokens -= float64(n)
	}
	return true, 0
}

// Exhausted reports whether the bucket named key is empty, without taking a
// token.
func (l *Limiter) Exhausted(key string, perMin int) bool {
	if perMin <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refilled(key, perMin).tokens < 1
}

// refilled returns the bucket named key, topped up for the time elapsed. It
// must be called with l.mu held.
func (l *Limiter) refilled(key string, perMin int) *bucket {
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxBuckets {
			l.prune(now)
		}
		b = &bucket{
			tokens:   float64(perMin),
			last:     now,
			capacity: float64(perMin),
			refill:   float64(perMin) / 60.0,
		}
		l.buckets[key] = b
	}

	b.tokens += now.Sub(b.last).Seconds() * b.refill
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now
	return b
}

func (l *Limiter) prune(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*b.refill >= b.capacity {
			delete(l.buckets, k)
		}
	}
}
