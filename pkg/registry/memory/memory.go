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

// Package memory is the default, in-process registry backend. It is a no-op
// Driver: there is no shared store, so all logic lives in registry.BaseRegistry.
package memory

import "github.com/cs3org/reva/v3/pkg/registry"

func init() {
	registry.Register("memory", func(m map[string]any) (registry.Driver, error) {
		return &driver{events: make(chan registry.Event)}, nil
	})
}

type driver struct {
	events chan registry.Event // never written; closed on Close
}

func (d *driver) Add(string, registry.Node) error       { return nil }
func (d *driver) Remove(string, string) error           { return nil }
func (d *driver) Watch() (<-chan registry.Event, error) { return d.events, nil }

func (d *driver) Close() {
	select {
	case <-d.events:
	default:
		close(d.events)
	}
}

// New returns an in-memory registry.Registry, for callers that build the
// backend directly.
func New(m map[string]any) registry.Registry {
	return registry.NewBase(&driver{events: make(chan registry.Event)}, registry.Thresholds{})
}
