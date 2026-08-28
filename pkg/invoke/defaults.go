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

// defaultInvocation is a built-in invocation every service instance exposes,
// implemented by the framework itself. A default shadows a service-defined
// invocation of the same name.
type defaultInvocation struct {
	spec   InvocationSpec
	fn     func(ctx context.Context, inst instance, args Args) (Result, error)
	stream func(ctx context.Context, inst instance, args Args, emit StreamEmit) error
}

// defaults are the built-in invocations, in registration order. Each registers
// itself from init() in its own file (config.go, logs.go): adding one is a new
// file, not a dispatch edit.
var defaults []defaultInvocation

// registerDefault records a built-in invocation.
func registerDefault(d defaultInvocation) { defaults = append(defaults, d) }

// defaultSpecs are the catalog entries of the built-in invocations.
func defaultSpecs() []InvocationSpec {
	specs := make([]InvocationSpec, 0, len(defaults))
	for _, d := range defaults {
		specs = append(specs, d.spec)
	}
	return specs
}

// lookupDefault finds a built-in invocation by name.
func lookupDefault(name string) (defaultInvocation, bool) {
	for _, d := range defaults {
		if d.spec.Name == name {
			return d, true
		}
	}
	return defaultInvocation{}, false
}
