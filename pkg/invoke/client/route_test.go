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

package client

import (
	"context"
	"net"
	"slices"
	"testing"

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	_ "github.com/cs3org/reva/v3/pkg/registry/memory"
	"google.golang.org/grpc"
)

// sharedRegistry has two storageprovider instances that advertise "flush" as
// shared, and one userprovider instance in the hostA process.
func sharedRegistry(t *testing.T) registry.Registry {
	t.Helper()
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatalf("registry.New: %v", err)
	}
	meta := func(host, control string) map[string]string {
		return map[string]string{
			"host":                       host,
			registry.MetaState:           registry.StateReady,
			registry.MetaControl:         control,
			invoke.MetaSharedInvocations: "flush",
		}
	}
	if err := reg.Add(registry.NewService("storageprovider", []registry.Node{
		registry.NewNode("hostA:9001/storageprovider", "hostA:9001", meta("hostA", "hostA:9500")),
		registry.NewNode("hostB:9001/storageprovider", "hostB:9001", meta("hostB", "hostB:9500")),
	})); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(registry.NewService("userprovider", []registry.Node{
		registry.NewNode("hostA:9002/userprovider", "hostA:9002", meta("hostA", "hostA:9500")),
	})); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestRouteSharedInvocation(t *testing.T) {
	reg := sharedRegistry(t)
	tests := []struct {
		selector, invocation string
		groups               []int // endpoints per group
	}{
		{"storageprovider", "flush", []int{2}},
		{"storageprovider", "config", []int{1, 1}},
		{"hostB:9001/storageprovider", "flush", []int{1}},
		// One group per service.
		{"*", "flush", []int{2, 1}},
		{"hostA", "flush", []int{1, 1}},
	}
	for _, tt := range tests {
		_, eps, err := Resolve(reg, tt.selector)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tt.selector, err)
		}
		groups := Route(eps, tt.invocation)
		var got []int
		for _, g := range groups {
			got = append(got, len(g))
		}
		if !slices.Equal(got, tt.groups) {
			t.Errorf("%s %s: expected groups %v, got %v", tt.selector, tt.invocation, tt.groups, got)
		}
	}
}

// TestFanOutSharedGivesOneResult checks that a shared invocation reports one
// result for the service, not one per instance.
func TestFanOutSharedGivesOneResult(t *testing.T) {
	eps := []Endpoint{
		{Node: "n1", Service: "s", Shared: []string{"op"}, Err: "node advertises no control endpoint"},
		{Node: "n2", Service: "s", Shared: []string{"op"}, Err: "node advertises no control endpoint"},
	}
	res := FanOut(context.Background(), eps, "op", nil)
	if len(res) != 1 || res[0].Error == "" {
		t.Fatalf("expected one failed result, got %+v", res)
	}
}

// liveControl starts a gRPC server and returns its address.
func liveControl(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	return ln.Addr().String()
}

// deadControl returns an address where nothing listens.
func deadControl(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestPickSkipsUnreachable(t *testing.T) {
	live := liveControl(t)
	eps := []Endpoint{
		{Node: "none", Err: "node advertises no control endpoint"},
		{Node: "dead", Addr: deadControl(t)},
		{Node: "live", Addr: live},
	}
	if got := Pick(context.Background(), eps); got.Node != "live" {
		t.Fatalf("expected the live instance, got %q", got.Node)
	}
}

func TestPickPrefersInstanceInRotation(t *testing.T) {
	eps := []Endpoint{
		{Node: "draining", Addr: liveControl(t), Draining: true},
		{Node: "ready", Addr: liveControl(t)},
	}
	if got := Pick(context.Background(), eps); got.Node != "ready" {
		t.Fatalf("expected the instance in rotation, got %q", got.Node)
	}
	// A draining instance takes the call when no other can.
	eps[1].Addr = deadControl(t)
	if got := Pick(context.Background(), eps); got.Node != "draining" {
		t.Fatalf("expected the draining instance, got %q", got.Node)
	}
}
