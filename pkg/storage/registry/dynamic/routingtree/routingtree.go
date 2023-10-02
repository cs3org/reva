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

	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
)

// Route represents a route inside a storage provider.
type Route struct {
	Name      string
	MountID   string
	MountPath string
}

// RoutingTree is a tree containing routes.
type RoutingTree struct {
	route Route
	nodes map[string]*RoutingTree
}

// New returns a new RoutingTree.
func New(routes map[string]string) *RoutingTree {
	t := RoutingTree{
		nodes: map[string]*RoutingTree{},
		route: Route{},
	}

	for r, m := range routes {
		t.addRoute(r, m)
	}

	return &t
}

func (t *RoutingTree) addNode(r Route) *RoutingTree {
	if t.route.Name == r.Name {
		return t
	}

	newNode, ok := t.nodes[r.Name]
	if !ok {
		newNode = &RoutingTree{
			route: r,
			nodes: make(map[string]*RoutingTree),
		}
		t.nodes[r.Name] = newNode
	}

	return newNode
}

func (t *RoutingTree) addRoute(route, mountID string) {
	parts := strings.Split(path.Clean(route), "/")
	current := t

	for i, name := range parts {
		newNode := Route{
			Name: name,
		}

		if i == len(parts)-1 {
			newNode.MountID = mountID
			newNode.MountPath = route
		}

		current = current.addNode(newNode)
	}
}

func (t *RoutingTree) findRoute(p string) (*RoutingTree, error) {
	parts := strings.Split(path.Clean(p), "/")
	current := t

	for _, name := range parts {
		// If the current node matches the path part, continue
		if current.route.Name == name {
			continue
		}

		// If not, check if there is a children with the path part
		if childNode, ok := current.nodes[name]; ok {
			current = childNode
		} else {
			// If there is none but there are other children, this is an invalid path
			if len(current.nodes) != 0 {
				return nil, errors.New("route not found")
			}
			// If there is none and the node is a leaf, then that means the rest of the
			// path is not part of the route
			return current, nil
		}
	}

	return current, nil
}

// Resolve returns a list of providers for a given path.
func (t *RoutingTree) Resolve(p string) ([]*registrypb.ProviderInfo, error) {
	r, err := t.findRoute(p)
	if err != nil {
		return nil, err
	}

	providerMap := r.getMountID(p, map[string]*registrypb.ProviderInfo{})

	providers := make([]*registrypb.ProviderInfo, 0, len(providerMap))
	for _, p := range providerMap {
		providers = append(providers, p)
	}

	return providers, nil
}

func (t *RoutingTree) getMountID(p string, providerMap map[string]*registrypb.ProviderInfo) map[string]*registrypb.ProviderInfo {
	if len(t.nodes) == 0 {
		if _, ok := providerMap[t.route.MountID]; !ok {
			providerMap[t.route.MountID] = &registrypb.ProviderInfo{
				ProviderId:   t.route.MountID,
				ProviderPath: t.route.MountPath,
			}
		}
	}

	for _, r := range t.nodes {
		r.getMountID(p, providerMap)
	}

	return providerMap
}
