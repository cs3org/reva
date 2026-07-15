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

package admin

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
	_ "github.com/cs3org/reva/v3/pkg/registry/memory"
)

func testRegistry(t *testing.T) registry.Registry {
	t.Helper()
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatalf("registry.New: %v", err)
	}
	ready := func(host, pid, control string) map[string]string {
		return map[string]string{
			"host": host, "pid": pid,
			registry.MetaState:   registry.StateReady,
			registry.MetaControl: control,
		}
	}
	// One process hosting a storageprovider that advertises its control channel.
	// Two storageprovider instances in two processes, each advertising its own
	// control channel. The control channel is not a service of its own — it is
	// discovered through this metadata.
	if err := reg.Add(registry.NewService("storageprovider", []registry.Node{
		registry.NewNode("hostA:9001/storageprovider", "hostA:9001", ready("hostA", "100", "hostA:9500")),
		registry.NewNode("hostB:9001/storageprovider", "hostB:9001", ready("hostB", "200", "hostB:9500")),
	})); err != nil {
		t.Fatal(err)
	}
	// A second service in the hostA process, for partial-id selectors.
	if err := reg.Add(registry.NewService("userprovider", []registry.Node{
		registry.NewNode("hostA:9002/userprovider", "hostA:9002", ready("hostA", "100", "hostA:9500")),
	})); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestResolveSelectorByAddress(t *testing.T) {
	reg := testRegistry(t)
	_, eps, err := resolveSelector(reg, "hostA:9001")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if len(eps) != 1 || eps[0].node != "hostA:9001/storageprovider" {
		t.Fatalf("expected the hostA:9001 instance, got %+v", eps)
	}
}

func TestResolveSelectorByHost(t *testing.T) {
	reg := testRegistry(t)
	_, eps, err := resolveSelector(reg, "hostA")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected both hostA instances, got %+v", eps)
	}
	// Sorted by node id.
	if eps[0].node != "hostA:9001/storageprovider" || eps[1].node != "hostA:9002/userprovider" {
		t.Fatalf("unexpected instances: %+v", eps)
	}
}

// A bare token that is both a service name and a host resolves as the service.
func TestResolveSelectorNameWinsOverHost(t *testing.T) {
	reg := testRegistry(t)
	if err := reg.Add(registry.NewService("hostA", []registry.Node{
		registry.NewNode("hostB:9100/hostA", "hostB:9100", map[string]string{
			"host": "hostB", registry.MetaState: registry.StateReady, registry.MetaControl: "hostB:9500",
		}),
	})); err != nil {
		t.Fatal(err)
	}
	_, eps, err := resolveSelector(reg, "hostA")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if len(eps) != 1 || eps[0].node != "hostB:9100/hostA" {
		t.Fatalf("expected the service to win over the host, got %+v", eps)
	}
}

// TestResolveSelectorByName checks that a plain service name fans out to every
// instance's control endpoint.
func TestResolveSelectorByName(t *testing.T) {
	reg := testRegistry(t)
	svc, eps, err := resolveSelector(reg, "storageprovider")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if svc != "storageprovider" {
		t.Errorf("unexpected service: %s", svc)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
	got := map[string]bool{eps[0].addr: true, eps[1].addr: true}
	if !got["hostA:9500"] || !got["hostB:9500"] {
		t.Errorf("expected both control endpoints, got %+v", eps)
	}
}

// TestResolveSelectorByID checks that a node id targets exactly one instance and
// still resolves the service name to route with.
func TestResolveSelectorByID(t *testing.T) {
	reg := testRegistry(t)
	svc, eps, err := resolveSelector(reg, "hostB:9001/storageprovider")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if svc != "storageprovider" {
		t.Errorf("unexpected service: %s", svc)
	}
	if len(eps) != 1 || eps[0].node != "hostB:9001/storageprovider" || eps[0].addr != "hostB:9500" {
		t.Fatalf("expected the single hostB instance, got %+v", eps)
	}
	// The control channel routes on the node id, so the exact instance is reached.
	if eps[0].target != "hostB:9001/storageprovider" {
		t.Errorf("expected target to be the node id, got %q", eps[0].target)
	}
}

// TestResolveSelectorUnknownID rejects a node id that is not registered.
func TestResolveSelectorUnknownID(t *testing.T) {
	reg := testRegistry(t)
	if _, _, err := resolveSelector(reg, "hostZ:9999/storageprovider"); err == nil {
		t.Fatal("expected error for unknown instance id")
	}
}

// TestResolveSelectorFleet checks that "*" resolves every live instance.
func TestResolveSelectorFleet(t *testing.T) {
	reg := testRegistry(t)
	_, eps, err := resolveSelector(reg, "*")
	if err != nil {
		t.Fatalf("resolveSelector: %v", err)
	}
	if len(eps) != 3 {
		t.Fatalf("expected all 3 instances, got %+v", eps)
	}
}

// TestResolveSelectorNoMatch checks that a selector matching nothing fails
// cleanly.
func TestResolveSelectorNoMatch(t *testing.T) {
	reg := testRegistry(t)
	if _, _, err := resolveSelector(reg, "nope"); err == nil {
		t.Fatal("expected error for a selector matching nothing")
	}
}
