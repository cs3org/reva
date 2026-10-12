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

package ratelimit

import (
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	l := New()
	for i := range 3 {
		if !l.Allow("ip", 3) {
			t.Fatalf("request %d refused", i)
		}
	}
	if l.Allow("ip", 3) {
		t.Fatal("the fourth request in a minute must be refused")
	}
	if !l.Allow("other", 3) {
		t.Fatal("buckets are per key")
	}
	if !l.Allow("ip", 0) {
		t.Fatal("a limit of zero disables the bucket")
	}
}

func TestExhaustedTakesNothing(t *testing.T) {
	l := New()
	for range 10 {
		if l.Exhausted("ip", 2) {
			t.Fatal("peeking must not take tokens")
		}
	}
	l.Allow("ip", 2)
	l.Allow("ip", 2)
	if !l.Exhausted("ip", 2) {
		t.Fatal("an empty bucket must be reported")
	}
}

func TestPruneKeepsBusyBuckets(t *testing.T) {
	l := New()
	l.Allow("busy", 5)
	l.Exhausted("idle", 5)
	l.mu.Lock()
	l.prune(time.Now().Add(time.Second))
	_, busy := l.buckets["busy"]
	_, idle := l.buckets["idle"]
	l.mu.Unlock()
	if !busy || idle {
		t.Fatalf("prune kept busy=%v idle=%v", busy, idle)
	}
}
