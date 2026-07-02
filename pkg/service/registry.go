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

package service

import (
	"sync"

	"github.com/cs3org/reva/v3/pkg/registry"
)

var (
	globalRegistryMu sync.RWMutex
	globalRegistry   registry.Registry
)

// SetGlobalRegistry installs the process-wide service registry handle. The
// first non-nil registry wins; later calls are ignored.
func SetGlobalRegistry(r registry.Registry) {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	if globalRegistry == nil {
		globalRegistry = r
	}
}

// GlobalRegistry returns the process-wide registry, or nil if none was set.
func GlobalRegistry() registry.Registry {
	globalRegistryMu.RLock()
	defer globalRegistryMu.RUnlock()
	return globalRegistry
}
