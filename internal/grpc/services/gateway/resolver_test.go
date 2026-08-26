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

package gateway

import (
	"context"
	"sync"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	storageregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
)

// testResolver is a process-wide service.Clients whose peers are swappable, so
// tests can inject fakes. The gateway resolves its peers through the package
// level accessors (the global resolver), so tests install one resolver once and
// swap the clients it hands back per test.
type testResolver struct {
	revaservice.Clients
	mu       sync.Mutex
	registry storageregistry.RegistryAPIClient
	shares   collaboration.CollaborationAPIClient
}

func (r *testResolver) StorageRegistry(context.Context) (storageregistry.RegistryAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registry, nil
}

func (r *testResolver) UserShareProvider(context.Context) (collaboration.CollaborationAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shares, nil
}

var (
	globalTestResolver     = &testResolver{}
	globalTestResolverOnce sync.Once
)

// stampGatewayPeers installs the swappable test resolver (once) and points it at
// the given peers, so a subsequent service.StorageRegistry(ctx) or
// service.UserShareProvider(ctx) returns them.
func stampGatewayPeers(reg storageregistry.RegistryAPIClient, shares collaboration.CollaborationAPIClient) {
	globalTestResolverOnce.Do(func() {
		revaservice.SetGlobal(globalTestResolver)
	})
	globalTestResolver.mu.Lock()
	globalTestResolver.registry = reg
	globalTestResolver.shares = shares
	globalTestResolver.mu.Unlock()
}
