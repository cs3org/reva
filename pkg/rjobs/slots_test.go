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

package rjobs

import "testing"

func TestSlotPool(t *testing.T) {
	leader := func(job string) bool { return job == "lead" }
	p := newSlotPool(3, 1, map[string]int{"capped": 1}, leader)

	relCapped, ok := p.tryAcquire("capped")
	if !ok {
		t.Fatal("first run of a capped job should get a slot")
	}
	if _, ok := p.tryAcquire("capped"); ok {
		t.Fatal("a capped job must not exceed its cap")
	}
	relFree, ok := p.tryAcquire("free")
	if !ok {
		t.Fatal("an uncapped job should get a slot")
	}
	// two on-demand runs fill the three workers minus the periodic reserve.
	if _, ok := p.tryAcquire("free"); ok {
		t.Fatal("on-demand jobs must not take the periodic reserve")
	}
	if _, ok := p.tryAcquire("lead"); !ok {
		t.Fatal("a leader periodic job should get the reserved slot")
	}

	freed := p.released()
	relCapped()
	relCapped() // releasing twice is a no-op
	select {
	case <-freed:
	default:
		t.Fatal("a release must wake the waiting dispatchers")
	}
	if _, ok := p.tryAcquire("capped"); !ok {
		t.Fatal("a released slot should be reusable")
	}
	relFree()
	if p.onDemand != 1 || p.busy["free"] != 0 {
		t.Fatalf("unexpected accounting: on-demand %d, busy %v", p.onDemand, p.busy)
	}
}
