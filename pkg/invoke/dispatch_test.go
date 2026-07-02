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
	"errors"
	"testing"
)

type extraInvokable struct{}

func (extraInvokable) Invocations() []InvocationSpec {
	return []InvocationSpec{{Name: "dump", Kind: KindReadonly}}
}

func (extraInvokable) Invoke(_ context.Context, name string, _ map[string]any) (Result, error) {
	if name != "dump" {
		return nil, errors.New("unknown invocation")
	}
	return Result{"ok": true}, nil
}

func hasSpec(specs []InvocationSpec, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}

// TestInstanceExposesConfigByID checks that a service instance is addressable by
// its node id and exposes the shared config invocation without implementing
// Invokable, returning its (redacted) config.
func TestInstanceExposesConfigByID(t *testing.T) {
	id := "127.0.0.1:9001/svc-a"
	RegisterInstance(id, "svc-a", map[string]any{"addr": "x", "secret": "s"}, nil)

	specs, ok := Invocations(id)
	if !ok {
		t.Fatal("instance should be known by id")
	}
	if !hasSpec(specs, ConfigInvocation) {
		t.Fatalf("expected default %q invocation, got %+v", ConfigInvocation, specs)
	}

	res, err := Invoke(context.Background(), id, ConfigInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke(config): %v", err)
	}
	if res["addr"] != "x" {
		t.Fatalf("unexpected config: %+v", res)
	}
	if res["secret"] != RedactedValue {
		t.Fatalf("expected redacted secret, got %v", res["secret"])
	}
}

// TestTwoInstancesSameServiceRouteByID checks that two instances of one service
// in a process are addressed distinctly by their ids — the case name routing
// cannot disambiguate.
func TestTwoInstancesSameServiceRouteByID(t *testing.T) {
	RegisterInstance("127.0.0.1:9101/dup", "dup", map[string]any{"which": "one"}, nil)
	RegisterInstance("127.0.0.1:9102/dup", "dup", map[string]any{"which": "two"}, nil)

	one, err := Invoke(context.Background(), "127.0.0.1:9101/dup", ConfigInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke one: %v", err)
	}
	two, err := Invoke(context.Background(), "127.0.0.1:9102/dup", ConfigInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke two: %v", err)
	}
	if one["which"] != "one" || two["which"] != "two" {
		t.Fatalf("ids did not route distinctly: one=%v two=%v", one, two)
	}
}

// TestInstanceExtendsDefaults checks that an instance whose service implements
// Invokable exposes the shared default plus its own invocations.
func TestInstanceExtendsDefaults(t *testing.T) {
	id := "127.0.0.1:9002/svc-b"
	RegisterInstance(id, "svc-b", map[string]any{"a": 1}, extraInvokable{})

	specs, ok := Invocations(id)
	if !ok {
		t.Fatal("instance should be known")
	}
	if !hasSpec(specs, ConfigInvocation) || !hasSpec(specs, "dump") {
		t.Fatalf("expected config + dump, got %+v", specs)
	}
	if _, err := Invoke(context.Background(), id, "dump", nil); err != nil {
		t.Fatalf("Invoke(dump): %v", err)
	}
}

// TestServiceNameFallback checks the async/local path: a bare service name
// resolves to a local instance.
func TestServiceNameFallback(t *testing.T) {
	RegisterInstance("127.0.0.1:9003/svc-c", "svc-c", map[string]any{"k": "v"}, nil)
	res, err := Invoke(context.Background(), "svc-c", ConfigInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke by name: %v", err)
	}
	if res["k"] != "v" {
		t.Fatalf("unexpected: %+v", res)
	}
}

// TestUnknownTarget covers the not-found signal the control channel maps to gRPC
// NotFound.
func TestUnknownTarget(t *testing.T) {
	if _, ok := Invocations("nope/nope"); ok {
		t.Fatal("unknown target must report ok=false")
	}
	if !HasInvocations() {
		t.Fatal("HasInvocations should be true once anything is registered")
	}
}
