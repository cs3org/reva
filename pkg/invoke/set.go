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

// Handler is a unary invocation implementation, made with Typed.
type Handler struct {
	args []ArgSpec
	fn   func(ctx context.Context, raw map[string]any) (Result, error)
}

// StreamHandler is a streaming invocation implementation, made with
// TypedStream.
type StreamHandler struct {
	args []ArgSpec
	fn   func(ctx context.Context, raw map[string]any, emit StreamEmit) error
}

// Typed makes a Handler from fn, which takes its arguments as the struct A.
// Each argument is a field with an `arg:"name"` tag (`arg:"name,required"` if
// it is required) and a `help` tag. The field type is a string, bool, integer
// or float kind, time.Duration, time.Time, or an encoding.TextUnmarshaler; a
// slice of one of these takes comma-separated values, and a pointer to one is
// nil when the argument is absent. A time is RFC3339, or a duration that means
// that long ago. Use struct{} for no arguments.
func Typed[A any](fn func(ctx context.Context, args A) (Result, error)) Handler {
	fields := argFields[A]()
	return Handler{
		args: specsOf(fields),
		fn: func(ctx context.Context, raw map[string]any) (Result, error) {
			a, err := bind[A](fields, raw)
			if err != nil {
				return nil, err
			}
			return fn(ctx, a)
		},
	}
}

// TypedStream is the streaming counterpart of Typed.
func TypedStream[A any](fn func(ctx context.Context, args A, emit StreamEmit) error) StreamHandler {
	fields := argFields[A]()
	return StreamHandler{
		args: specsOf(fields),
		fn: func(ctx context.Context, raw map[string]any, emit StreamEmit) error {
			a, err := bind[A](fields, raw)
			if err != nil {
				return err
			}
			return fn(ctx, a, emit)
		},
	}
}

// Set is a service's collection of named invocations. It implements Invokable:
// a service embeds a *Set and declares its methods instead of hand-writing the
// catalog, routing and required-arg checks. Build it once at construction; it
// is read-only afterward.
type Set struct {
	order   []string
	methods map[string]*registered
}

type registered struct {
	spec   InvocationSpec
	fn     *Handler
	stream *StreamHandler
}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{methods: map[string]*registered{}} }

// Builder configures one invocation: chain Mutating/Dangerous and finish with
// Handle.
type Builder struct{ m *registered }

// Add begins registering an invocation (KindReadonly by default).
func (s *Set) Add(name, description string) *Builder {
	m := &registered{spec: InvocationSpec{Name: name, Description: description, Kind: KindReadonly}}
	if _, dup := s.methods[name]; !dup {
		s.order = append(s.order, name)
	}
	s.methods[name] = m
	return &Builder{m}
}

// Mutating marks the invocation as changing state.
func (b *Builder) Mutating() *Builder { b.m.spec.Kind = KindMutating; return b }

// Dangerous marks the invocation as warranting a confirmation prompt.
func (b *Builder) Dangerous() *Builder { b.m.spec.Kind = KindDangerous; return b }

// Shared marks the invocation as acting on state that all instances share, so
// one instance per service runs it.
func (b *Builder) Shared() *Builder { b.m.spec.Scope = ScopeShared; return b }

// Stream marks the invocation as producing a stream of results (implied by
// HandleStream).
func (b *Builder) Stream() *Builder { b.m.spec.Streaming = true; return b }

// Handle sets the unary implementation, made with Typed.
func (b *Builder) Handle(h Handler) {
	b.m.fn = &h
	b.m.spec.Args = h.args
}

// HandleStream sets the streaming implementation, made with TypedStream, and
// marks the invocation streaming. An invocation may have both a Handle and a
// HandleStream.
func (b *Builder) HandleStream(h StreamHandler) {
	b.m.stream = &h
	b.m.spec.Args = h.args
	b.m.spec.Streaming = true
}

// Invocations implements Invokable: the catalog, in registration order.
func (s *Set) Invocations() []InvocationSpec {
	out := make([]InvocationSpec, 0, len(s.order))
	for _, name := range s.order {
		m := s.methods[name]
		spec := m.spec
		spec.StreamOnly = m.fn == nil && m.stream != nil
		out = append(out, spec)
	}
	return out
}

// Invoke implements Invokable.
func (s *Set) Invoke(ctx context.Context, name string, args map[string]any) (Result, error) {
	m, ok := s.methods[name]
	if !ok || m.fn == nil {
		return nil, fmt.Errorf("invoke: no method %q", name)
	}
	return m.fn.fn(ctx, args)
}

// InvokeStream implements StreamInvokable.
func (s *Set) InvokeStream(ctx context.Context, name string, args map[string]any, emit StreamEmit) error {
	m, ok := s.methods[name]
	if !ok || m.stream == nil {
		return fmt.Errorf("invoke: no streaming method %q", name)
	}
	return m.stream.fn(ctx, args, emit)
}

// DriverPrefix is put in front of the invocations that a service takes from
// its driver, so that they cannot hide the invocations of the service.
const DriverPrefix = "driver."

// MergeDriver adds the invocations of driver under DriverPrefix, if driver is
// Invokable. Each one keeps its spec and goes to the driver with its own
// name. A name that s has already is an error, so the service does not start.
func (s *Set) MergeDriver(driver any) error {
	inv, ok := driver.(Invokable)
	if !ok {
		return nil
	}
	sv, canStream := driver.(StreamInvokable)
	for _, spec := range inv.Invocations() {
		name := DriverPrefix + spec.Name
		if _, dup := s.methods[name]; dup {
			return fmt.Errorf("invoke: driver invocation %q is already registered", name)
		}
		if spec.Streaming && !canStream {
			return fmt.Errorf("invoke: driver invocation %q is streaming but the driver has no InvokeStream", name)
		}
		m := &registered{spec: spec}
		m.spec.Name = name
		if !spec.StreamOnly {
			m.fn = &Handler{fn: func(ctx context.Context, raw map[string]any) (Result, error) {
				return inv.Invoke(ctx, spec.Name, raw)
			}}
		}
		if spec.Streaming {
			m.stream = &StreamHandler{fn: func(ctx context.Context, raw map[string]any, emit StreamEmit) error {
				return sv.InvokeStream(ctx, spec.Name, raw, emit)
			}}
		}
		s.order = append(s.order, name)
		s.methods[name] = m
	}
	return nil
}
