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
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// Selector picks one node, preferring ready over degraded and never offline or
// draining.
type Selector interface {
	Pick(nodes []registry.Node) (registry.Node, bool)
}

// NewSelector returns the selector configured by name: "local" (also the
// default for an empty name), "first", "random" or "roundrobin".
func NewSelector(name string) (Selector, error) {
	switch name {
	case "", "local":
		host, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("service registry: local selector: %w", err)
		}
		return LocalSelector{Host: host}, nil
	case "first":
		return FirstSelector{}, nil
	case "random":
		return RandomSelector{}, nil
	case "roundrobin":
		return &RoundRobinSelector{}, nil
	}
	return nil, fmt.Errorf("service registry: unknown selector %q, expected local, first, random or roundrobin", name)
}

// eligible drops the nodes no selector may pick: offline and draining ones.
func eligible(nodes []registry.Node) []registry.Node {
	out := make([]registry.Node, 0, len(nodes))
	for _, n := range nodes {
		switch n.Metadata()[registry.MetaState] {
		case registry.StateOffline, registry.StateDraining:
			continue
		}
		out = append(out, n)
	}
	return out
}

// selectable returns ready nodes, or degraded if none are ready.
func selectable(nodes []registry.Node) []registry.Node {
	ready := make([]registry.Node, 0, len(nodes))
	degraded := make([]registry.Node, 0, len(nodes))
	for _, n := range eligible(nodes) {
		if n.Metadata()[registry.MetaState] == registry.StateDegraded {
			degraded = append(degraded, n)
		} else {
			ready = append(ready, n)
		}
	}
	if len(ready) > 0 {
		return ready
	}
	return degraded
}

// LocalSelector picks a node registered from this host, so that a call stays on
// the machine it started on, and a random one when none here is selectable.
type LocalSelector struct{ Host string }

func (s LocalSelector) Pick(nodes []registry.Node) (registry.Node, bool) {
	c := selectable(nodes)
	if len(c) == 0 {
		return nil, false
	}
	local := make([]registry.Node, 0, len(c))
	for _, n := range c {
		if n.Metadata()[registry.MetaHost] == s.Host {
			local = append(local, n)
		}
	}
	if len(local) > 0 {
		c = local
	}
	return c[rand.Intn(len(c))], true
}

// FirstSelector returns the first selectable node.
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
