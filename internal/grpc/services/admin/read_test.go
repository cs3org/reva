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
	"context"
	"testing"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
)

func TestListServicesByInvocation(t *testing.T) {
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	// The first registry set in the process stays, so add to that one.
	service.SetGlobalRegistry(reg)
	reg = service.GlobalRegistry()
	for name, invocations := range map[string]string{
		"usershareprovider": "config,stats",
		"gateway":           "config",
	} {
		node := registry.NewNode("hostA:9001/"+name, "hostA:9001", map[string]string{invoke.MetaInvocations: invocations})
		if err := reg.Add(registry.NewService(name, []registry.Node{node})); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reg.Remove(registry.NewService(name, nil)) })
	}

	res, err := (&svc{}).ListServices(context.Background(), &adminpb.ListServicesRequest{Invocation: "stats"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 1 || res.Services[0].Name != "usershareprovider" {
		t.Errorf("services = %v, want only usershareprovider", res.Services)
	}
}
