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
	"maps"
	"strconv"
	"time"
)

// Args are an invocation's arguments. The declared ones are already parsed to
// their types: read them with ArgDef.Get.
type Args map[string]any

// argValue lists the types an argument can have.
type argValue interface {
	string | bool | int | time.Duration | time.Time
}

// ArgDef declares one argument of type T, and reads it in the handler.
type ArgDef[T argValue] struct{ spec ArgSpec }

// Arg declares a required argument of type T.
func Arg[T argValue](name, description string) *ArgDef[T] {
	return &ArgDef[T]{ArgSpec{Name: name, Description: description, Required: true}}
}

// OptArg declares an optional argument of type T.
func OptArg[T argValue](name, description string) *ArgDef[T] {
	return &ArgDef[T]{ArgSpec{Name: name, Description: description}}
}

// Get returns the argument, or the zero value of T if it is absent.
func (d *ArgDef[T]) Get(a Args) T {
	v, _ := a[d.spec.Name].(T)
	return v
}

// Has reports whether the argument was supplied.
func (d *ArgDef[T]) Has(a Args) bool {
	_, ok := a[d.spec.Name]
	return ok
}

func (d *ArgDef[T]) argSpec() ArgSpec { return d.spec }

// parse converts a wire value, which is a string, to T.
func (d *ArgDef[T]) parse(raw any) (any, error) {
	if v, ok := raw.(T); ok {
		return v, nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("argument %q: want %T, got %T", d.spec.Name, *new(T), raw)
	}
	var v T
	var err error
	switch p := any(&v).(type) {
	case *bool:
		*p, err = strconv.ParseBool(s)
	case *int:
		*p, err = strconv.Atoi(s)
	case *time.Duration:
		*p, err = time.ParseDuration(s)
	case *time.Time:
		if *p, err = parseTime(s); err != nil {
			return nil, fmt.Errorf("argument %q: %w", d.spec.Name, err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("argument %q: %q is not a valid %T", d.spec.Name, s, v)
	}
	return v, nil
}

// parseTime accepts an RFC3339 time, or a duration that means that long ago.
func parseTime(s string) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not an RFC3339 time or a duration (e.g. 5m)", s)
}

// argDecl is an ArgDef of any type.
type argDecl interface {
	argSpec() ArgSpec
	parse(raw any) (any, error)
}

// specsOf returns the catalog entries of the declared arguments.
func specsOf(decls []argDecl) []ArgSpec {
	specs := make([]ArgSpec, 0, len(decls))
	for _, d := range decls {
		specs = append(specs, d.argSpec())
	}
	return specs
}

// parseArgs checks that the required arguments are present and parses the
// declared ones to their types. Undeclared arguments stay as they are.
func parseArgs(name string, decls []argDecl, raw map[string]any) (Args, error) {
	a := make(Args, len(raw))
	maps.Copy(a, raw)
	for _, d := range decls {
		spec := d.argSpec()
		v, ok := raw[spec.Name]
		if !ok {
			if spec.Required {
				return nil, fmt.Errorf("invoke: %q requires argument %q", name, spec.Name)
			}
			continue
		}
		p, err := d.parse(v)
		if err != nil {
			return nil, fmt.Errorf("invoke: %q: %w", name, err)
		}
		a[spec.Name] = p
	}
	return a, nil
}

// Handler runs one unary invocation.
type Handler func(ctx context.Context, args Args) (Result, error)

// StreamHandlerFunc runs one streaming invocation.
type StreamHandlerFunc func(ctx context.Context, args Args, emit StreamEmit) error

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
	args   []argDecl
	fn     Handler
	stream StreamHandlerFunc
}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{methods: map[string]*registered{}} }

// Builder configures one invocation: chain Args/Mutating/Dangerous and finish
// with Handle.
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

// Args declares the invocation's arguments, made with Arg and OptArg. They are
// checked and parsed before the handler runs.
func (b *Builder) Args(args ...argDecl) *Builder {
	b.m.args = append(b.m.args, args...)
	b.m.spec.Args = specsOf(b.m.args)
	return b
}

// Mutating marks the invocation as changing state.
func (b *Builder) Mutating() *Builder { b.m.spec.Kind = KindMutating; return b }

// Dangerous marks the invocation as warranting a confirmation prompt.
func (b *Builder) Dangerous() *Builder { b.m.spec.Kind = KindDangerous; return b }

// Stream marks the invocation as producing a stream of results (implied by
// HandleStream).
func (b *Builder) Stream() *Builder { b.m.spec.Streaming = true; return b }

// Handle sets the unary implementation.
func (b *Builder) Handle(fn Handler) { b.m.fn = fn }

// HandleStream sets the streaming implementation and marks the invocation
// streaming. An invocation may have both a Handle and a HandleStream.
func (b *Builder) HandleStream(fn StreamHandlerFunc) {
	b.m.stream = fn
	b.m.spec.Streaming = true
}

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
	a, err := parseArgs(name, m.args, args)
	if err != nil {
		return nil, err
	}
	return m.fn(ctx, a)
}

// InvokeStream implements StreamInvokable.
func (s *Set) InvokeStream(ctx context.Context, name string, args map[string]any, emit StreamEmit) error {
	m, ok := s.methods[name]
	if !ok || m.stream == nil {
		return fmt.Errorf("invoke: no streaming method %q", name)
	}
	a, err := parseArgs(name, m.args, args)
	if err != nil {
		return err
	}
	return m.stream(ctx, a, emit)
}
