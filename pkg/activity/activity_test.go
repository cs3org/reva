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

package activity

import "testing"

func TestCounterEnterAndSnapshot(t *testing.T) {
	c := New()

	if s := c.Snapshot(); s.InFlight != 0 || s.Total != 0 || !s.LastRequest.IsZero() {
		t.Fatalf("fresh counter should be zero, got %+v", s)
	}

	done1 := c.Enter("A")
	done2 := c.Enter("A")
	if s := c.Snapshot(); s.InFlight != 2 || s.Total != 2 {
		t.Fatalf("expected in-flight=2 total=2, got %+v", s)
	}
	if c.Snapshot().LastRequest.IsZero() {
		t.Fatal("last request should be set after Enter")
	}

	done1()
	if s := c.Snapshot(); s.InFlight != 1 || s.Total != 2 {
		t.Fatalf("after one completion expected in-flight=1 total=2, got %+v", s)
	}
	done2()
	if s := c.Snapshot(); s.InFlight != 0 || s.Total != 2 {
		t.Fatalf("after all completions expected in-flight=0 total=2, got %+v", s)
	}
}

// TestPerMethodBreakdown checks the aggregate and per-method counts track
// independently, and that an empty method counts toward the aggregate only.
func TestPerMethodBreakdown(t *testing.T) {
	c := New()
	c.Enter("GetUser")()
	c.Enter("GetUser")()
	inflight := c.Enter("GetUserGroups") // leave one in flight
	c.Enter("")()                        // aggregate-only (HTTP-style)

	s := c.Snapshot()
	if s.Total != 4 || s.InFlight != 1 {
		t.Fatalf("aggregate wrong: %+v", s)
	}
	if len(s.Methods) != 2 {
		t.Fatalf("expected 2 methods (empty excluded), got %+v", s.Methods)
	}
	if m := s.Methods["GetUser"]; m.Total != 2 || m.InFlight != 0 {
		t.Fatalf("GetUser wrong: %+v", m)
	}
	if m := s.Methods["GetUserGroups"]; m.Total != 1 || m.InFlight != 1 {
		t.Fatalf("GetUserGroups wrong: %+v", m)
	}
	inflight()
	if m := c.Snapshot().Methods["GetUserGroups"]; m.InFlight != 0 {
		t.Fatalf("GetUserGroups should be idle after completion: %+v", m)
	}
}

// TestNilCounter checks the nil-safe contract used for services with no counter
// wired (e.g. serverless).
func TestNilCounter(t *testing.T) {
	var c *Counter
	done := c.Enter("X")
	done()
	if s := c.Snapshot(); s.InFlight != 0 || s.Total != 0 || !s.LastRequest.IsZero() || s.Methods != nil {
		t.Fatalf("nil counter should read zero, got %+v", s)
	}
}
