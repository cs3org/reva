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

package ocgraph

import (
	"context"
	"sync"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/service"
)

// testResolver is a process-wide service.Clients whose Gateway is swappable so
// tests can inject a mock gateway. Production code resolves the gateway through
// service.Gateway(ctx) (the global resolver), so tests install one resolver
// once and swap the returned client per test.
type testResolver struct {
	service.Clients
	mu               sync.Mutex
	gw               gateway.GatewayAPIClient
	gatewayFailAfter int
	gatewayErr       error
}

func (r *testResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gatewayFailAfter > 0 {
		r.gatewayFailAfter--
		if r.gatewayFailAfter == 0 {
			return nil, r.gatewayErr
		}
	}
	return r.gw, nil
}

var (
	globalTestResolver     = &testResolver{}
	globalTestResolverOnce sync.Once
)

// stampGateway installs the swappable test resolver (once) and points it at gw,
// so a subsequent service.Gateway(ctx) returns gw.
func stampGateway(gw gateway.GatewayAPIClient) {
	globalTestResolverOnce.Do(func() {
		service.SetGlobal(globalTestResolver)
	})
	globalTestResolver.mu.Lock()
	globalTestResolver.gw = gw
	globalTestResolver.gatewayFailAfter = 0
	globalTestResolver.gatewayErr = nil
	globalTestResolver.mu.Unlock()
}

// stampGatewayErrorAfter fails the nth Gateway call (1-based). Earlier calls
// still return the stamped client. stampGateway clears the countdown.
func stampGatewayErrorAfter(n int, err error) {
	globalTestResolverOnce.Do(func() {
		service.SetGlobal(globalTestResolver)
	})
	globalTestResolver.mu.Lock()
	globalTestResolver.gatewayFailAfter = n
	globalTestResolver.gatewayErr = err
	globalTestResolver.mu.Unlock()
}
