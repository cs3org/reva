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

package registry

type basicNode struct {
	id       string
	address  string
	metadata map[string]string
}

func (n *basicNode) ID() string                  { return n.id }
func (n *basicNode) Address() string             { return n.address }
func (n *basicNode) Metadata() map[string]string { return n.metadata }

// NewNode builds a Node.
func NewNode(id, address string, metadata map[string]string) Node {
	if metadata == nil {
		metadata = map[string]string{}
	}
	return &basicNode{id: id, address: address, metadata: metadata}
}

type basicService struct {
	name  string
	nodes []Node
}

func (s *basicService) Name() string  { return s.name }
func (s *basicService) Nodes() []Node { return s.nodes }

// NewService builds a Service.
func NewService(name string, nodes []Node) Service {
	return &basicService{name: name, nodes: nodes}
}
