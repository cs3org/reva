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

package invoke

import "sync"

// MetaInvocations is the registry metadata key holding a node's invocation
// names (comma-separated), so catalogs can be answered without dialing.
const MetaInvocations = "invocations"

// instance is one service instance in this process, addressed by its registry
// node id. inv holds the service's own operations, or is nil.
type instance struct {
	service string
	config  map[string]any // redacted
	inv     Invokable
}

var (
	mu sync.RWMutex
	// instances maps a node id (host:port/service) to its instance.
	instances = map[string]instance{}
)

// RegisterInstance records a service instance under its node id, with its
// config (redacted here) and optional Invokable.
func RegisterInstance(id, service string, config map[string]any, inv Invokable) {
	mu.Lock()
	defer mu.Unlock()
	instances[id] = instance{service: service, config: Redact(config), inv: inv}
}

// HasInvocations reports whether this process exposes anything invokable; the
// runtime gates the control channel on it.
func HasInvocations() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(instances) > 0
}

// lookup resolves a target: a node id, or (as a local fallback) a service name
// matching a local instance.
func lookup(target string) (instance, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if inst, ok := instances[target]; ok {
		return inst, true
	}
	for _, inst := range instances {
		if inst.service == target {
			return inst, true
		}
	}
	return instance{}, false
}
