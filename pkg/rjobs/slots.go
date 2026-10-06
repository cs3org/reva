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

import (
	"errors"
	"sync"
)

// ErrNoSlot is returned by Store.Claim when every job it could fetch for is at
// its concurrency cap. The dispatcher then waits for a slot to be released.
var ErrNoSlot = errors.New("rjobs: no free slot for any job")

// Slots hands out the worker slots of a runner, per job. Store.Claim acquires a
// slot before fetching a run of a job and releases it if nothing came; a slot
// that came with a run is kept until the run ends.
type Slots interface {
	// TryAcquire takes a slot for job, if the job is below its cap.
	TryAcquire(job string) (release func(), ok bool)
}

// slotPool keeps the per-job concurrency caps of a runner. On-demand jobs,
// capped or not, never take the slots reserved for leader periodic jobs, so a
// busy on-demand job cannot starve the schedule.
type slotPool struct {
	mu          sync.Mutex
	caps        map[string]int
	busy        map[string]int
	onDemand    int // on-demand runs holding a slot
	onDemandMax int
	isLeader    func(job string) bool
	// freed is closed, and replaced, whenever a slot is released.
	freed chan struct{}
}

func newSlotPool(workers, reserve int, caps map[string]int, isLeader func(string) bool) *slotPool {
	return &slotPool{
		caps: caps,
		busy: make(map[string]int),
		// always leave room for one on-demand run.
		onDemandMax: max(workers-reserve, 1),
		isLeader:    isLeader,
		freed:       make(chan struct{}),
	}
}

func (p *slotPool) tryAcquire(job string) (func(), bool) {
	leader := p.isLeader(job)

	p.mu.Lock()
	defer p.mu.Unlock()
	if c := p.caps[job]; c > 0 && p.busy[job] >= c {
		return nil, false
	}
	if !leader && p.onDemand >= p.onDemandMax {
		return nil, false
	}
	p.busy[job]++
	if !leader {
		p.onDemand++
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.busy[job]--
			if !leader {
				p.onDemand--
			}
			close(p.freed)
			p.freed = make(chan struct{})
		})
	}, true
}

// released returns a channel closed on the next release of a slot. Take it
// before claiming, so a release in between is not missed.
func (p *slotPool) released() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.freed
}

// claimSlots is the Slots handed to one Claim call. It remembers the slot that
// came with the claimed run, so the dispatcher can release it when the run ends.
type claimSlots struct {
	pool *slotPool
	held func()
}

func (c *claimSlots) TryAcquire(job string) (func(), bool) {
	release, ok := c.pool.tryAcquire(job)
	if !ok {
		return nil, false
	}
	c.held = release
	return func() {
		c.held = nil
		release()
	}, true
}

// take returns the slot kept by the claimed run, if any.
func (c *claimSlots) take() func() {
	held := c.held
	c.held = nil
	if held == nil {
		return func() {}
	}
	return held
}
