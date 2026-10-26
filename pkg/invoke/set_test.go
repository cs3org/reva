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
	"net/netip"
	"testing"
	"time"
)

// TestSet exercises the developer-facing builder: declaring a method, its
// catalog entry, required-arg validation, and routing to the handler.
func TestSet(t *testing.T) {
	type memberArgs struct {
		Group string `arg:"group,required" help:"the group id"`
		User  string `arg:"user,required" help:"the user id"`
	}
	set := NewSet()
	set.Add("is_member", "Report whether a user is a member of a group").
		Handle(Typed(func(_ context.Context, a memberArgs) (Result, error) {
			return Result{"member": a.Group == "admins" && a.User == "alice"}, nil
		}))

	// Catalog reflects the declaration.
	specs := set.Invocations()
	if len(specs) != 1 || specs[0].Name != "is_member" || specs[0].Kind != KindReadonly {
		t.Fatalf("unexpected catalog: %+v", specs)
	}
	if len(specs[0].Args) != 2 || !specs[0].Args[0].Required {
		t.Fatalf("unexpected args: %+v", specs[0].Args)
	}

	// Routing + typed args.
	res, err := set.Invoke(context.Background(), "is_member", map[string]any{"group": "admins", "user": "alice"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res["member"] != true {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Missing required arg is rejected before the handler runs.
	if _, err := set.Invoke(context.Background(), "is_member", map[string]any{"group": "admins"}); err == nil {
		t.Fatal("expected error for missing required arg")
	}

	// Unknown method.
	if _, err := set.Invoke(context.Background(), "nope", nil); err == nil {
		t.Fatal("expected error for unknown method")
	}
}

// TestSetStreaming exercises a streaming invocation: the catalog flags it, the
// handler emits several results, required args are enforced, and a unary Invoke
// of a stream-only method is rejected (and vice versa).
func TestSetStreaming(t *testing.T) {
	type tailArgs struct {
		N int `arg:"n,required" help:"how many"`
	}
	set := NewSet()
	set.Add("tail", "stream N ticks").
		Stream().
		HandleStream(TypedStream(func(_ context.Context, a tailArgs, emit StreamEmit) error {
			for i := range a.N {
				if err := emit(Result{"i": i}); err != nil {
					return err
				}
			}
			return nil
		}))

	specs := set.Invocations()
	if len(specs) != 1 || !specs[0].Streaming {
		t.Fatalf("expected a streaming spec, got %+v", specs)
	}

	var got []int
	err := set.InvokeStream(context.Background(), "tail", map[string]any{"n": "3"},
		func(r Result) error { got = append(got, r["i"].(int)); return nil })
	if err != nil {
		t.Fatalf("InvokeStream: %v", err)
	}
	if len(got) != 3 || got[0] != 0 || got[2] != 2 {
		t.Fatalf("unexpected stream: %+v", got)
	}

	// Missing required arg is rejected before the handler runs.
	if err := set.InvokeStream(context.Background(), "tail", nil, func(Result) error { return nil }); err == nil {
		t.Fatal("expected error for missing required arg")
	}

	// A stream-only method cannot be invoked unary.
	if _, err := set.Invoke(context.Background(), "tail", map[string]any{"n": "1"}); err == nil {
		t.Fatal("expected error invoking a stream-only method unary")
	}

	// A unary-only method cannot be invoked as a stream.
	set.Add("ping", "unary").Handle(Typed(func(context.Context, struct{}) (Result, error) { return Result{}, nil }))
	if err := set.InvokeStream(context.Background(), "ping", nil, func(Result) error { return nil }); err == nil {
		t.Fatal("expected error stream-invoking a unary-only method")
	}
}

// TestSetParsesTypedArgs checks that arguments arrive in the handler as the
// types of their fields, and that a value of the wrong type is rejected before
// the handler runs.
func TestSetParsesTypedArgs(t *testing.T) {
	type runID string
	type opArgs struct {
		Limit int           `arg:"limit"`
		All   bool          `arg:"all"`
		Wait  time.Duration `arg:"wait"`
		Since time.Time     `arg:"since"`
		ID    runID         `arg:"id"`
		Owner *string       `arg:"owner"`
		IDs   []runID       `arg:"ids"`
		Port  uint16        `arg:"port"`
		Ratio float64       `arg:"ratio"`
		Addrs []netip.Addr  `arg:"addrs"`
		Color color         `arg:"color"`
	}
	set := NewSet()
	var got opArgs
	set.Add("op", "").Handle(Typed(func(_ context.Context, a opArgs) (Result, error) { got = a; return Result{}, nil }))

	before := time.Now()
	_, err := set.Invoke(context.Background(), "op", map[string]any{
		"limit": "7", "all": "true", "wait": "2s", "since": "5m", "id": "r1", "owner": "",
		"ids": "r1, r2,,", "port": "8080", "ratio": "0.5", "addrs": "10.0.0.1,::1", "color": "red",
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got.Limit != 7 || !got.All || got.Wait != 2*time.Second || got.ID != "r1" ||
		got.Port != 8080 || got.Ratio != 0.5 || got.Color != "red" {
		t.Fatalf("unexpected args: %+v", got)
	}
	// A slice takes comma-separated values and drops the empty ones.
	if len(got.IDs) != 2 || got.IDs[0] != "r1" || got.IDs[1] != "r2" {
		t.Fatalf("ids: want [r1 r2], got %v", got.IDs)
	}
	if len(got.Addrs) != 2 || got.Addrs[1] != netip.IPv6Loopback() {
		t.Fatalf("addrs: want [10.0.0.1 ::1], got %v", got.Addrs)
	}
	// A pointer is set when the argument is given, even if it is empty.
	if got.Owner == nil || *got.Owner != "" {
		t.Fatalf("owner: want a pointer to \"\", got %v", got.Owner)
	}
	// A duration means that long ago.
	if got.Since.Before(before.Add(-5*time.Minute)) || got.Since.After(after.Add(-5*time.Minute)) {
		t.Fatalf("since %v is not 5m ago", got.Since)
	}

	if _, err := set.Invoke(context.Background(), "op", map[string]any{"since": "2026-10-19T10:00:00Z"}); err != nil {
		t.Fatalf("Invoke with an RFC3339 time: %v", err)
	}
	if want := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC); !got.Since.Equal(want) {
		t.Fatalf("since: want %v, got %v", want, got.Since)
	}
	if got.Owner != nil || got.Limit != 0 || got.IDs != nil {
		t.Fatalf("absent arguments must be zero, got %+v", got)
	}

	for _, bad := range []map[string]any{
		{"limit": "abc"}, {"all": "maybe"}, {"wait": "soon"}, {"since": "yesterday"},
		{"port": "70000"}, {"ratio": "half"}, {"addrs": "10.0.0.1,nope"}, {"color": "blue"},
	} {
		if _, err := set.Invoke(context.Background(), "op", bad); err == nil {
			t.Errorf("expected an error for %v", bad)
		}
	}
}

// TestTypedRejectsUnsupportedType checks that a field type the framework cannot
// parse fails when the handler is made, not at the first call.
func TestTypedRejectsUnsupportedType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for an unsupported argument type")
		}
	}()
	Typed(func(context.Context, struct {
		Labels map[string]string `arg:"labels"`
	}) (Result, error) {
		return nil, nil
	})
}

// color is an argument type that parses itself.
type color string

func (c *color) UnmarshalText(b []byte) error {
	if string(b) != "red" && string(b) != "green" {
		return errors.New("want red or green")
	}
	*c = color(b)
	return nil
}

func TestSetStreamOnly(t *testing.T) {
	stream := StreamHandler{fn: func(context.Context, map[string]any, StreamEmit) error { return nil }}
	set := NewSet()
	set.Add("tail", "").HandleStream(stream)
	set.Add("ping", "").Handle(Handler{})
	// logs has both forms, as the built-in logs invocation.
	logs := set.Add("logs", "")
	logs.Handle(Handler{})
	logs.HandleStream(stream)

	got := map[string]bool{}
	for _, s := range set.Invocations() {
		got[s.Name] = s.StreamOnly
	}
	want := map[string]bool{"tail": true, "logs": false, "ping": false}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: StreamOnly = %v, want %v", name, got[name], w)
		}
	}
}

// fakeDriver is a driver that exposes its invocations with an embedded Set.
type fakeDriver struct{ *Set }

// unaryDriver is Invokable but not StreamInvokable.
type unaryDriver struct{ specs []InvocationSpec }

func (d unaryDriver) Invocations() []InvocationSpec { return d.specs }
func (d unaryDriver) Invoke(context.Context, string, map[string]any) (Result, error) {
	return Result{}, nil
}

func TestSetMergeDriver(t *testing.T) {
	type accountArgs struct {
		User string `arg:"user,required" help:"the user id"`
	}
	drv := fakeDriver{NewSet()}
	drv.Add("account", "the account of a user").Mutating().Shared().
		Handle(Typed(func(_ context.Context, a accountArgs) (Result, error) {
			return Result{"user": a.User}, nil
		}))
	drv.Add("tail", "").HandleStream(TypedStream(func(_ context.Context, _ struct{}, emit StreamEmit) error {
		return emit(Result{"line": 1})
	}))

	set := NewSet()
	set.Add("ping", "").Handle(Typed(func(context.Context, struct{}) (Result, error) { return Result{}, nil }))
	if err := set.MergeDriver(drv); err != nil {
		t.Fatalf("MergeDriver: %v", err)
	}

	specs := set.Invocations()
	if len(specs) != 3 || specs[0].Name != "ping" {
		t.Fatalf("unexpected catalog: %+v", specs)
	}
	account, tail := specs[1], specs[2]
	if account.Name != "driver.account" || account.Kind != KindMutating || account.Scope != ScopeShared ||
		account.Streaming || len(account.Args) != 1 || !account.Args[0].Required {
		t.Fatalf("driver.account: spec not kept: %+v", account)
	}
	if tail.Name != "driver.tail" || !tail.Streaming || !tail.StreamOnly {
		t.Fatalf("driver.tail: spec not kept: %+v", tail)
	}

	res, err := set.Invoke(context.Background(), "driver.account", map[string]any{"user": "alice"})
	if err != nil || res["user"] != "alice" {
		t.Fatalf("Invoke driver.account: %v, %v", res, err)
	}
	if _, err := set.Invoke(context.Background(), "driver.account", nil); err == nil {
		t.Fatal("expected the driver to reject a missing required arg")
	}
	if _, err := set.Invoke(context.Background(), "driver.tail", nil); err == nil {
		t.Fatal("expected error invoking a stream-only driver method unary")
	}
	var lines int
	if err := set.InvokeStream(context.Background(), "driver.tail", nil, func(Result) error { lines++; return nil }); err != nil || lines != 1 {
		t.Fatalf("InvokeStream driver.tail: %d lines, %v", lines, err)
	}
}

func TestSetMergeDriverRefuses(t *testing.T) {
	set := NewSet()
	set.Add("driver.account", "").Handle(Handler{})
	drv := unaryDriver{specs: []InvocationSpec{{Name: "account"}}}
	if err := set.MergeDriver(drv); err == nil {
		t.Error("expected an error for a name collision")
	}

	drv = unaryDriver{specs: []InvocationSpec{{Name: "tail", Streaming: true}}}
	if err := NewSet().MergeDriver(drv); err == nil {
		t.Error("expected an error for a streaming invocation without InvokeStream")
	}

	set = NewSet()
	if err := set.MergeDriver(struct{}{}); err != nil || len(set.Invocations()) != 0 {
		t.Errorf("a driver that is not Invokable must add nothing: %v, %v", set.Invocations(), err)
	}
}
