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
	"strconv"
)

// Args are an invocation's arguments, with typed accessors over their string
// wire form. A missing key yields the zero value.
type Args map[string]any

// String returns the argument as a string, or "" if absent.
func (a Args) String(key string) string {
	switch v := a[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// Bool parses the argument as a boolean ("true"/"1"/…), false if absent.
func (a Args) Bool(key string) bool {
	switch v := a[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}

// Int parses the argument as an int, 0 if absent or unparseable.
func (a Args) Int(key string) int {
	switch v := a[key].(type) {
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// Has reports whether an argument was supplied.
func (a Args) Has(key string) bool { _, ok := a[key]; return ok }

// Handler runs one invocation.
type Handler func(ctx context.Context, args Args) (Result, error)

// Set is a service's collection of named invocations. It implements Invokable,
// so a service exposes its operations by building a Set (embedding *Set, or
// returning it from a method) instead of hand-writing Invocations plus an Invoke
// switch: the Set does the name→handler routing, builds the catalog, and
// validates required arguments. Build it once at construction; it is read-only
// afterward.
type Set struct {
	order   []string
	methods map[string]*registered
}

type registered struct {
	spec InvocationSpec
	fn   Handler
}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{methods: map[string]*registered{}} }

// Builder configures one invocation: chain Arg/OptArg/Mutating/Dangerous and
// finish with Handle.
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

// Arg declares a required argument, enforced before the handler runs.
func (b *Builder) Arg(name, description string) *Builder {
	b.m.spec.Args = append(b.m.spec.Args, ArgSpec{Name: name, Description: description, Required: true})
	return b
}

// OptArg declares an optional argument.
func (b *Builder) OptArg(name, description string) *Builder {
	b.m.spec.Args = append(b.m.spec.Args, ArgSpec{Name: name, Description: description})
	return b
}

// Mutating marks the invocation as changing state.
func (b *Builder) Mutating() *Builder { b.m.spec.Kind = KindMutating; return b }

// Dangerous marks the invocation as changing state in a way that warrants a
// confirmation prompt.
func (b *Builder) Dangerous() *Builder { b.m.spec.Kind = KindDangerous; return b }

// Handle sets the implementation, completing the registration.
func (b *Builder) Handle(fn Handler) { b.m.fn = fn }

// Invocations implements Invokable: the catalog, in registration order.
func (s *Set) Invocations() []InvocationSpec {
	out := make([]InvocationSpec, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, s.methods[name].spec)
	}
	return out
}

// Invoke implements Invokable.
func (s *Set) Invoke(ctx context.Context, name string, args map[string]any) (Result, error) {
	m, ok := s.methods[name]
	if !ok || m.fn == nil {
		return nil, fmt.Errorf("invoke: no method %q", name)
	}
	for _, a := range m.spec.Args {
		if a.Required {
			if _, ok := args[a.Name]; !ok {
				return nil, fmt.Errorf("invoke: %q requires argument %q", name, a.Name)
			}
		}
	}
	return m.fn(ctx, Args(args))
}
