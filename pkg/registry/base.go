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

import (
	"fmt"
	"maps"
	"sync"
)

// BaseRegistry is the shared registry: it owns the node cache and implements
// Add/Remove/GetService/ListServices, the liveness state machine and the watch
// loop. A backend supplies only a Driver. Writes go cache-first, then to the
// driver.
type BaseRegistry struct {
	driver     Driver
	thresholds Thresholds

	mu       sync.RWMutex
	services map[string]map[string]Node // service name -> node ID -> node

	stop chan struct{}
}

// NewBase wraps driver in a BaseRegistry and starts its watch and liveness
// loops.
func NewBase(driver Driver, thresholds Thresholds) *BaseRegistry {
	b := &BaseRegistry{
		driver:     driver,
		thresholds: thresholds,
		services:   map[string]map[string]Node{},
		stop:       make(chan struct{}),
	}
	go b.watchLoop()
	if thresholds.Enabled() {
		go b.livenessLoop()
	}
	return b
}

func (b *BaseRegistry) Add(svc Service) error {
	for _, n := range svc.Nodes() {
		b.cachePut(svc.Name(), n)
		if err := b.driver.Add(svc.Name(), n); err != nil {
			return err
		}
	}
	return nil
}

// Remove de-registers the given nodes, or all nodes of the service if none are
// given.
func (b *BaseRegistry) Remove(svc Service) error {
	nodes := svc.Nodes()
	if len(nodes) == 0 {
		for _, id := range b.nodeIDs(svc.Name()) {
			b.cacheDelete(svc.Name(), id)
			if err := b.driver.Remove(svc.Name(), id); err != nil {
				return err
			}
		}
		return nil
	}
	for _, n := range nodes {
		b.cacheDelete(svc.Name(), n.ID())
		if err := b.driver.Remove(svc.Name(), n.ID()); err != nil {
			return err
		}
	}
	return nil
}

func (b *BaseRegistry) GetService(name string) (Service, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	nodes, ok := b.services[name]
	if !ok || len(nodes) == 0 {
		return nil, fmt.Errorf("service %v not found", name)
	}
	return serviceFromCache(name, nodes), nil
}

func (b *BaseRegistry) ListServices() ([]Service, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Service, 0, len(b.services))
	for name, nodes := range b.services {
		if len(nodes) == 0 {
			continue
		}
		out = append(out, serviceFromCache(name, nodes))
	}
	return out, nil
}

func (b *BaseRegistry) Watch() (<-chan Event, error) {
	return b.driver.Watch()
}

func (b *BaseRegistry) Close() {
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
	b.driver.Close()
}

// watchLoop applies remote changes to the cache, re-establishing the watch when
// the channel closes.
func (b *BaseRegistry) watchLoop() {
	for {
		select {
		case <-b.stop:
			return
		default:
		}
		ch, err := b.driver.Watch()
		if err != nil {
			if !b.sleep() {
				return
			}
			continue
		}
		if !b.consume(ch) {
			return
		}
		if !b.sleep() {
			return
		}
	}
}

func (b *BaseRegistry) consume(ch <-chan Event) bool {
	for {
		select {
		case <-b.stop:
			return false
		case ev, ok := <-ch:
			if !ok {
				return true
			}
			switch ev.Type {
			case EventAdd:
				if ev.Node != nil {
					b.cachePut(ev.Service, ev.Node)
				}
			case EventRemove:
				if ev.Node != nil {
					b.cacheDelete(ev.Service, ev.Node.ID())
				}
			}
		}
	}
}

func (b *BaseRegistry) cachePut(service string, n Node) {
	b.mu.Lock()
	defer b.mu.Unlock()
	nodes, ok := b.services[service]
	if !ok {
		nodes = map[string]Node{}
		b.services[service] = nodes
	}
	nodes[n.ID()] = NewNode(n.ID(), n.Address(), copyMeta(n.Metadata()))
}

func (b *BaseRegistry) cacheDelete(service, nodeID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	nodes, ok := b.services[service]
	if !ok {
		return
	}
	delete(nodes, nodeID)
	if len(nodes) == 0 {
		delete(b.services, service)
	}
}

func (b *BaseRegistry) nodeIDs(service string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	nodes := b.services[service]
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	return ids
}

func serviceFromCache(name string, nodes map[string]Node) Service {
	ns := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		ns = append(ns, n)
	}
	return NewService(name, ns)
}

func copyMeta(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}
