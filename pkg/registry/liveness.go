// Copyright 2018-2024 CERN
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

package registry

import "time"

// Thresholds configure the liveness state machine; a zero value disables the
// transition.
type Thresholds struct {
	DegradedAfter time.Duration
	OfflineAfter  time.Duration
	ReapAfter     time.Duration
}

func (t Thresholds) Enabled() bool {
	return t.DegradedAfter > 0 || t.OfflineAfter > 0 || t.ReapAfter > 0
}

// livenessLoop derives node state from last_seen: ready -> degraded -> offline
// -> reaped, recovering on a fresh beat.
func (b *BaseRegistry) livenessLoop() {
	ticker := time.NewTicker(b.sweepInterval())
	defer ticker.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			b.sweep(time.Now())
		}
	}
}

func (b *BaseRegistry) sweepInterval() time.Duration {
	interval := b.thresholds.OfflineAfter
	if b.thresholds.DegradedAfter > 0 && b.thresholds.DegradedAfter < interval {
		interval = b.thresholds.DegradedAfter
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	interval /= 3
	if interval <= 0 {
		interval = time.Second
	}
	return interval
}

func (b *BaseRegistry) sweep(now time.Time) {
	services, _ := b.ListServices()
	t := b.thresholds
	for _, svc := range services {
		for _, n := range svc.Nodes() {
			state := n.Metadata()[MetaState]
			if state == StateDraining {
				continue
			}
			seen, ok := lastSeen(n.Metadata())
			if !ok {
				continue
			}
			quiet := now.Sub(seen)
			switch {
			case t.ReapAfter > 0 && quiet > t.OfflineAfter+t.ReapAfter:
				_ = b.Remove(NewService(svc.Name(), []Node{n}))
			case t.OfflineAfter > 0 && quiet > t.OfflineAfter:
				b.transition(svc.Name(), n, StateOffline)
			case t.DegradedAfter > 0 && quiet > t.DegradedAfter:
				b.transition(svc.Name(), n, StateDegraded)
			default:
				if state != StateReady {
					b.transition(svc.Name(), n, StateReady)
				}
			}
		}
	}
}

func (b *BaseRegistry) transition(service string, n Node, state string) {
	if n.Metadata()[MetaState] == state {
		return
	}
	meta := copyMeta(n.Metadata())
	meta[MetaState] = state
	_ = b.Add(NewService(service, []Node{NewNode(n.ID(), n.Address(), meta)}))
}

func lastSeen(meta map[string]string) (time.Time, bool) {
	v, ok := meta[MetaLastSeen]
	if !ok || v == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func (b *BaseRegistry) sleep() bool {
	select {
	case <-b.stop:
		return false
	case <-time.After(time.Second):
		return true
	}
}
