// Copyright 2018-2021 CERN
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

package memory

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"gotest.tools/assert"
)

var (
	in    = make(map[string]interface{})
	reg   = New(in)
	node1 = node{
		id:      uuid.New().String(),
		address: "0.0.0.0:42069",
		metadata: map[string]string{
			"type": "auth-bearer",
		},
	}

	node2 = node{
		id:      uuid.New().String(),
		address: "0.0.0.0:7777",
		metadata: map[string]string{
			"type": "auth-basic",
		},
	}

	node3 = node{id: uuid.NewString(), address: "0.0.0.0:8888"}
	node4 = node{id: uuid.NewString(), address: "0.0.0.0:9999"}
)

var scenarios = []struct {
	name          string // scenario name
	in            string // used to query the Registry by service name
	services      []service
	expectedNodes []node // expected set of nodes
}{
	{
		name: "single service with 2 nodes",
		in:   "auth-provider",
		services: []service{
			{name: "auth-provider", nodes: []node{node1, node2}},
		},
		expectedNodes: []node{node1, node2},
	},
	{
		name: "single service with 2 nodes scaled x2",
		in:   "auth-provider",
		services: []service{
			{name: "auth-provider", nodes: []node{node1, node2}},
			{name: "auth-provider", nodes: []node{node3, node4}},
		},
		expectedNodes: []node{node1, node2, node3, node4},
	},
}

func TestAdd(t *testing.T) {
	reg = New(in)
	s1 := scenarios[1].services[0]
	s2 := scenarios[1].services[1]
	_ = reg.Add(s1)
	_ = reg.Add(s2)

	_ = reg.Add(service{
		name: "test",
		nodes: []node{
			{
				id:       "1234",
				address:  "localhost:8899",
				metadata: nil,
			},
		},
	})

	expectedNumberOfNodes := len(s1.nodes) + len(s2.nodes)
	if s, err := reg.GetService(s1.name); err != nil {
		t.Error(err)
		collectedNumberOfNodes := len(s.Nodes())

		if expectedNumberOfNodes == collectedNumberOfNodes {
			t.Error(fmt.Errorf("expected %v nodes, got: %v", expectedNumberOfNodes, collectedNumberOfNodes))
		}
	}
}

func TestGetService(t *testing.T) {
	for _, scenario := range scenarios {
		reg = New(in)
		for _, service := range scenario.services {
			if err := reg.Add(&service); err != nil {
				os.Exit(1)
			}
		}

		t.Run(scenario.name, func(t *testing.T) {
			svc, err := reg.GetService(scenario.in)
			if err != nil {
				t.Error(err)
			}

			totalNodes := len(svc.Nodes())
			assert.Equal(t, len(scenario.expectedNodes), totalNodes)
		})
	}
}

//	func contains(a []registry.Node, b registry.Node) bool {
//		for i := range a {
//			if a[i].Address() == b.Address() {
//				return true
//			}
//		}
//		return false
//	}
