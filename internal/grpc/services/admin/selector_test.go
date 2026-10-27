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
	"slices"
	"testing"

	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
)

// mountRegistry has two live storageprovider instances for eoshome-i01, one
// offline instance for it, and one instance for eosproject.
func mountRegistry(t *testing.T) registry.Registry {
	t.Helper()
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatalf("registry.New: %v", err)
	}
	node := func(id, state, mountID, mountPath string) registry.Node {
		return registry.NewNode(id, id[:len(id)-len("/storageprovider")], map[string]string{
			registry.MetaState:           state,
			registry.MetaControl:         "ctl-" + id,
			registry.MetaMountID:         mountID,
			registry.MetaMountPath:       mountPath,
			invoke.MetaSharedInvocations: "quota_get",
		})
	}
	if err := reg.Add(registry.NewService("storageprovider", []registry.Node{
		node("hostA:9001/storageprovider", registry.StateReady, "eoshome-i01", "/eos/user"),
		node("hostB:9001/storageprovider", registry.StateDraining, "eoshome-i01", "/eos/user"),
		node("hostC:9001/storageprovider", registry.StateOffline, "eoshome-i01", "/eos/user"),
		node("hostD:9001/storageprovider", registry.StateReady, "eosproject", "/eos/project"),
	})); err != nil {
		t.Fatal(err)
	}
	return reg
}

func nodeIDs(eps []client.Endpoint) []string {
	var ids []string
	for _, ep := range eps {
		ids = append(ids, ep.Node)
	}
	slices.Sort(ids)
	return ids
}

func TestResolveSelectorByMetadata(t *testing.T) {
	reg := mountRegistry(t)
	for selector, want := range map[string][]string{
		"storageprovider[mount_id=eoshome-i01]": {"hostA:9001/storageprovider", "hostB:9001/storageprovider"},
		// A value can contain "/", so this must not resolve as a node id.
		"storageprovider[mount_id=eosproject, mount_path=/eos/project]": {"hostD:9001/storageprovider"},
	} {
		svc, eps, err := client.Resolve(reg, selector)
		if err != nil {
			t.Fatalf("%s: client.Resolve: %v", selector, err)
		}
		if svc != "storageprovider" {
			t.Errorf("%s: got service %q, want storageprovider", selector, svc)
		}
		if got := nodeIDs(eps); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", selector, got, want)
		}
	}
}

func TestResolveSelectorByMetadataRoutesShared(t *testing.T) {
	_, eps, err := client.Resolve(mountRegistry(t), "storageprovider[mount_id=eoshome-i01]")
	if err != nil {
		t.Fatalf("client.Resolve: %v", err)
	}
	if groups := client.Route(eps, "quota_get"); len(groups) != 1 || len(groups[0]) != 2 {
		t.Errorf("shared invocation: got groups %+v, want one group of 2", groups)
	}
	if groups := client.Route(eps, "config"); len(groups) != 2 {
		t.Errorf("unshared invocation: got groups %+v, want 2", groups)
	}
}

func TestResolveSelectorByMetadataErrors(t *testing.T) {
	reg := mountRegistry(t)
	for _, selector := range []string{
		"storageprovider[mount_id=eoshome-i01,mount_path=/eos/project]",
		"storageprovider[mount_id=eoshome-i99]",
		"userprovider[mount_id=eoshome-i01]",
		"storageprovider[mount_id=eoshome-i01",
		"storageprovider[mount_id]",
		"storageprovider[=eoshome-i01]",
		"[mount_id=eoshome-i01]",
	} {
		if _, eps, err := client.Resolve(reg, selector); err == nil {
			t.Errorf("%s: expected an error, got %+v", selector, eps)
		}
	}
}
