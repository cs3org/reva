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

import "context"

// APIInvocation is the invocation an HTTP service instance exposes to return
// the OpenAPI document of the routes it serves, so that one document can
// describe a whole fleet.
const APIInvocation = "openapi"

// apiSpec is shared: every instance of a service serves the same routes, so
// one of them answers for all.
var apiSpec = InvocationSpec{
	Name:        APIInvocation,
	Description: "Return the OpenAPI document of the HTTP routes the service serves.",
	Kind:        KindReadonly,
	Scope:       ScopeShared,
}

// DescribeAPI gives the instance registered under id the APIInvocation,
// answered by describe. Only an instance that serves HTTP routes has one, which
// is why it is not a default every instance exposes.
func DescribeAPI(id string, describe func(context.Context) (Result, error)) {
	mu.Lock()
	defer mu.Unlock()
	inst, ok := instances[id]
	if !ok {
		return
	}
	inst.api = describe
	instances[id] = inst
}
