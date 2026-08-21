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

package loginflow

import (
	"sync"
	"time"
)

// limiter is a per-key token bucket. Keys are source IPs or token hashes. The
// anonymous Init and poll endpoints use it.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   float64
	last     time.Time
	capacity float64
	refill   float64 // tokens per second
}

func newLimiter() *limiter {
	return &limiter{buckets: make(map[string]*bucket)}
}

// allow takes one token from the bucket named key, sized for perMin requests per
// minute. It returns false when the bucket is empty. perMin <= 0 disables the
// limit.
func (l *limiter) allow(key string, perMin int) bool {
	if perMin <= 0 {
		return true
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
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

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
