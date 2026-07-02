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

// Package invoke is the process-wide invocation framework: services opt into
// the Invokable capability, and every instance exposes the shared default
// invocations (config, logs). The control channel is its gRPC surface.
package invoke

import "context"

// InvocationKind classifies an invocation for audit and gating.
type InvocationKind string

const (
	// KindReadonly returns a redacted, serializable snapshot and changes nothing.
	KindReadonly InvocationKind = "readonly"
	// KindMutating changes state in a bounded, expected way.
	KindMutating InvocationKind = "mutating"
	// KindDangerous changes state in a way that warrants extra confirmation.
	KindDangerous InvocationKind = "dangerous"
)

// ArgSpec describes one argument an invocation accepts.
type ArgSpec struct {
	Name        string
	Description string
	Required    bool
}

// InvocationSpec is a service's self-description of one invocation: its name,
// arguments and kind. It is what ListInvocations exposes.
type InvocationSpec struct {
	Name        string
	Description string
	Args        []ArgSpec
	Kind        InvocationKind
}

// Result is an invocation's serializable, already-redacted return value.
type Result map[string]any

// Invokable is the single capability a service implements to expose operations
// to the Admin API — both read-only inspection (dump a cache, the lock table,
// the registry node table) and mutations. There is no separate inspection
// interface: a read is simply an invocation with Kind == KindReadonly. The
// invocation runs inside the service, which owns the state; the Admin API is
// only the entry point and orchestrator. It is opt-in — a service that is not
// Invokable exposes nothing, with zero coupling and no reflection into private
// fields.
type Invokable interface {
	// Invocations self-describes the operations the service exposes.
	Invocations() []InvocationSpec
	// Invoke runs the named invocation with the given arguments.
	Invoke(ctx context.Context, name string, args map[string]any) (Result, error)
}
