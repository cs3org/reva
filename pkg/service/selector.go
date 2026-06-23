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

package service

import (
	"math/rand"
	"sync/atomic"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// Selector picks one node, preferring ready over degraded and never offline or
// draining.
type Selector interface {
	Pick(nodes []registry.Node) (registry.Node, bool)
}

// selectable returns ready nodes, or degraded if none are ready.
func selectable(nodes []registry.Node) []registry.Node {
	ready := make([]registry.Node, 0, len(nodes))
	degraded := make([]registry.Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Metadata()[registry.MetaState] {
		case registry.StateOffline, registry.StateDraining:
			continue
		case registry.StateDegraded:
			degraded = append(degraded, n)
		default:
			ready = append(ready, n)
		}
	}
	if len(ready) > 0 {
		return ready
	}
	return degraded
}

// FirstSelector returns the first selectable node (the default).
type FirstSelector struct{}

func (FirstSelector) Pick(nodes []registry.Node) (registry.Node, bool) {
	c := selectable(nodes)
	if len(c) == 0 {
		return nil, false
	}
	return c[0], true
}

type RoundRobinSelector struct{ n uint64 }

func (s *RoundRobinSelector) Pick(nodes []registry.Node) (registry.Node, bool) {
	c := selectable(nodes)
	if len(c) == 0 {
		return nil, false
	}
	i := atomic.AddUint64(&s.n, 1) - 1
	return c[int(i%uint64(len(c)))], true
}

type RandomSelector struct{}

func (RandomSelector) Pick(nodes []registry.Node) (registry.Node, bool) {
	c := selectable(nodes)
	if len(c) == 0 {
		return nil, false
	}
	return c[rand.Intn(len(c))], true
}
