// Copyright 2018-2024 CERN
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

package service

import (
	"context"
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
)

func meta(state string) map[string]string {
	return metaTransport(state, registry.TransportGRPC)
}

func metaTransport(state, transport string) map[string]string {
	return map[string]string{
		registry.MetaState:     state,
		registry.MetaTransport: transport,
	}
}

func TestSelectorPrefersReadyAndSkipsOfflineDraining(t *testing.T) {
	nodes := []registry.Node{
		registry.NewNode("a", "10.0.0.1:1", meta(registry.StateOffline)),
		registry.NewNode("b", "10.0.0.2:1", meta(registry.StateReady)),
		registry.NewNode("c", "10.0.0.3:1", meta(registry.StateDraining)),
	}
	n, ok := FirstSelector{}.Pick(nodes)
	if !ok || n.Address() != "10.0.0.2:1" {
		t.Fatalf("expected ready node 10.0.0.2:1, got %v ok=%v", n, ok)
	}
}

func TestSelectorFallsBackToDegraded(t *testing.T) {
	nodes := []registry.Node{
		registry.NewNode("a", "10.0.0.1:1", meta(registry.StateOffline)),
		registry.NewNode("b", "10.0.0.2:1", meta(registry.StateDegraded)),
	}
	n, ok := FirstSelector{}.Pick(nodes)
	if !ok || n.Address() != "10.0.0.2:1" {
		t.Fatalf("expected degraded fallback, got %v ok=%v", n, ok)
	}
}

func TestSelectorNoSelectableNode(t *testing.T) {
	nodes := []registry.Node{
		registry.NewNode("a", "10.0.0.1:1", meta(registry.StateOffline)),
	}
	if _, ok := (FirstSelector{}).Pick(nodes); ok {
		t.Fatal("expected no selectable node")
	}
}

func TestResolveUnknownService(t *testing.T) {
	c := NewClients(memory.New(nil)).(*clients)
	if _, _, err := c.resolve("nope"); err == nil {
		t.Fatal("expected error resolving unknown service")
	}
}

func TestResolveReturnsAddressAndCachesConn(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService(NameGateway, []registry.Node{
		registry.NewNode("g1", "127.0.0.1:19000", meta(registry.StateReady)),
	}))
	c := NewClients(reg).(*clients)

	conn1, addr, err := c.resolve(NameGateway)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if addr != "127.0.0.1:19000" {
		t.Fatalf("unexpected address %q", addr)
	}
	conn2, _, err := c.resolve(NameGateway)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if conn1 != conn2 {
		t.Fatal("expected the conn cache to return the same connection")
	}
}

func TestResolveSkipsSameNamedHTTPNode(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService(NamePreferences, []registry.Node{
		registry.NewNode("http", "127.0.0.1:19143", metaTransport(registry.StateReady, registry.TransportHTTP)),
		registry.NewNode("grpc", "127.0.0.1:19142", metaTransport(registry.StateReady, registry.TransportGRPC)),
	}))
	c := NewClients(reg).(*clients)

	for range 20 {
		_, addr, err := c.resolve(NamePreferences)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if addr != "127.0.0.1:19142" {
			t.Fatalf("resolved the http node %q", addr)
		}
	}
}

func TestResolveNoGRPCNode(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService(NamePreferences, []registry.Node{
		registry.NewNode("http", "127.0.0.1:19143", metaTransport(registry.StateReady, registry.TransportHTTP)),
	}))
	c := NewClients(reg).(*clients)

	if _, _, err := c.resolve(NamePreferences); err == nil {
		t.Fatal("expected no selectable grpc node")
	}
}

func TestHTTPEndpointSkipsSameNamedGRPCNode(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService(NamePreferences, []registry.Node{
		registry.NewNode("grpc", "127.0.0.1:19142", metaTransport(registry.StateReady, registry.TransportGRPC)),
		registry.NewNode("http", "127.0.0.1:19143", metaTransport(registry.StateReady, registry.TransportHTTP)),
	}))
	c := NewClients(reg)

	for range 20 {
		ep, err := c.HTTPEndpoint(context.Background(), ByName(NamePreferences))
		if err != nil {
			t.Fatalf("HTTPEndpoint failed: %v", err)
		}
		if ep.Address() != "127.0.0.1:19143" {
			t.Fatalf("resolved the grpc node %q", ep.Address())
		}
	}
}

func TestDegradeMarksNode(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService(NameGateway, []registry.Node{
		registry.NewNode("g1", "127.0.0.1:19000", meta(registry.StateReady)),
	}))
	c := NewClients(reg).(*clients)
	c.Degrade(NameGateway, "127.0.0.1:19000")

	svc, _ := reg.GetService(NameGateway)
	if got := svc.Nodes()[0].Metadata()[registry.MetaState]; got != registry.StateDegraded {
		t.Fatalf("expected degraded, got %q", got)
	}
}
