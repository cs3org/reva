// Copyright 2018-2023 CERN
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

// Package routingtree contains the routing tree implementation.
package routingtree

import (
	"errors"
	"path"
	"strings"
)

// Route represents a route inside a storage provider.
type Route struct {
	Path    string
	MountID string
}

// RoutingTree is a tree made of maps of routes.
type RoutingTree struct {
	route Route
	nodes map[string]*RoutingTree
}

// New returns a new RoutingTree.
func New(rs []Route) *RoutingTree {
	rt := &RoutingTree{
		route: Route{
			Path: "/",
		},
		nodes: make(map[string]*RoutingTree),
	}

	for _, r := range rs {
		rt.AddRoute(r)
	}

	return rt
}

func (t *RoutingTree) addNode(r Route) *RoutingTree {
	if t.route.Path == r.Path {
		return t
	}

	newNode, ok := t.nodes[r.Path]
	if !ok {
		newNode = &RoutingTree{
			route: r,
			nodes: make(map[string]*RoutingTree),
		}
		t.nodes[r.Path] = newNode
	}

	return newNode
}

func getAncestors(p string) []string {
	p = path.Clean(p)
	ancestors := []string{}
	previous := "/"
	parts := strings.Split(p, "/")

	for _, part := range parts {
		previous = path.Join(previous, part)
		ancestors = append(ancestors, previous)
	}

	return ancestors
}

// AddRoute adds a new Route to the RoutingTree based on a path `p`.
func (t *RoutingTree) AddRoute(r Route) {
	parts := getAncestors(r.Path)
	current := t

	for _, path := range parts {
		newNode := Route{
			Path: path,
		}
		if path == r.Path {
			newNode.MountID = r.MountID
		}
		current = current.addNode(newNode)
	}
}

func (t *RoutingTree) findNodeForPath(p string) (*RoutingTree, error) {
	parts := getAncestors(p)
	current := t

	for _, part := range parts {
		// If the current node matches the path part, continue
		if current.route.Path == part {
			continue
		}

		// If not, check if there is a children with the path part
		if childNode, ok := current.nodes[part]; ok {
			current = childNode
		} else {
			// If there is none but there are other children, this is an invalid path
			if len(current.nodes) != 0 {
				return nil, errors.New("invalid route")
			}
			// If there is none and the node is a leaf, then that means the rest of the
			// path is not part of the route
			return current, nil
		}
	}

	return current, nil
}

func (t *RoutingTree) getLeaves() []*RoutingTree {
	if len(t.nodes) == 0 {
		return []*RoutingTree{t}
	}

	leafNodes := []*RoutingTree{}
	queue := []*RoutingTree{t}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if len(node.nodes) == 0 {
			leafNodes = append(leafNodes, node)
		}

		for _, child := range node.nodes {
			queue = append(queue, child)
		}
	}

	return leafNodes
}

// GetProviders returns a list of providers for a given path.
func (t *RoutingTree) GetProviders(p string) ([]string, error) {
	subtree, err := t.findNodeForPath(path.Clean(p))
	if err != nil || subtree == nil {
		return nil, err
	}

	leaves := subtree.getLeaves()
	providers := []string{}
	providerMap := make(map[string]bool)

	for _, l := range leaves {
		providerMap[l.route.MountID] = true
	}

	for p := range providerMap {
		providers = append(providers, p)
	}

	return providers, nil
}
