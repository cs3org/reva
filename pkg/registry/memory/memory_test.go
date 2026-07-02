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

package memory_test

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
	"github.com/google/uuid"
	"gotest.tools/assert"
)

func node(addr string) registry.Node {
	return registry.NewNode(uuid.NewString(), addr, nil)
}

func TestAddAndGet(t *testing.T) {
	reg := memory.New(nil)
	n1, n2 := node("0.0.0.0:42069"), node("0.0.0.0:7777")
	if err := reg.Add(registry.NewService("auth-provider", []registry.Node{n1, n2})); err != nil {
		t.Fatal(err)
	}

	svc, err := reg.GetService("auth-provider")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(svc.Nodes()))
}

func TestAddMergesNodes(t *testing.T) {
	reg := memory.New(nil)
	_ = reg.Add(registry.NewService("auth-provider", []registry.Node{node("0.0.0.0:1"), node("0.0.0.0:2")}))
	_ = reg.Add(registry.NewService("auth-provider", []registry.Node{node("0.0.0.0:3"), node("0.0.0.0:4")}))

	svc, err := reg.GetService("auth-provider")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 4, len(svc.Nodes()))
}

func TestRemoveAndListServices(t *testing.T) {
	reg := memory.New(nil)
	n := node("0.0.0.0:9000")
	_ = reg.Add(registry.NewService("gateway", []registry.Node{n}))

	svcs, _ := reg.ListServices()
	assert.Equal(t, 1, len(svcs))

	_ = reg.Remove(registry.NewService("gateway", []registry.Node{n}))
	if _, err := reg.GetService("gateway"); err == nil {
		t.Fatal("expected gateway to be removed")
	}
}
