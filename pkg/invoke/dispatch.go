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

import (
	"context"
	"fmt"
)

// Invocations returns the invocations a target exposes on this node: the
// shared defaults plus the target's own. ok is false for an unknown target.
func Invocations(target string) ([]InvocationSpec, bool) {
	inst, ok := lookup(target)
	if !ok {
		return nil, false
	}
	specs := defaultSpecs()
	if inst.inv != nil {
		specs = append(specs, inst.inv.Invocations()...)
	}
	return specs, true
}

// Invoke runs a named invocation on a target: built-in defaults are handled by
// the framework, anything else is delegated to the target's Invokable.
func Invoke(ctx context.Context, target, name string, args map[string]any) (Result, error) {
	inst, ok := lookup(target)
	if !ok {
		return nil, fmt.Errorf("invoke: unknown target %q on this node", target)
	}
	if d, ok := lookupDefault(name); ok {
		if d.fn == nil {
			return nil, fmt.Errorf("invoke: %q is streaming-only (use InvokeStream)", name)
		}
		return d.fn(ctx, inst, Args(args))
	}
	if inst.inv == nil {
		return nil, fmt.Errorf("invoke: %q exposes no invocation %q on this node", target, name)
	}
	return inst.inv.Invoke(ctx, name, args)
}

// InvocationNames returns just the names a target exposes, for the
// MetaInvocations registry metadata.
func InvocationNames(target string) []string {
	specs, _ := Invocations(target)
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}
