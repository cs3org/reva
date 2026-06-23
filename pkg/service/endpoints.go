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
	"fmt"
	"net"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// Endpoint is a resolved registry node with convenience accessors over its
// address and metadata. It is returned by HTTPEndpoint(s) for services whose
// reachable URL another service needs (data gateway, data provider, ...).
type Endpoint interface {
	Name() string    // service name
	Address() string // bind "host:port"
	Host() string
	Port() string
	Scheme() string              // "http" | "https"
	Prefix() string              // URL path prefix
	URL() string                 // public_url, else scheme://address/prefix
	Meta(key string) string      // single metadata value
	Metadata() map[string]string // all advertised metadata
	Node() registry.Node         // raw node
}

type endpoint struct {
	name string
	node registry.Node
}

func (e endpoint) Name() string                { return e.name }
func (e endpoint) Address() string             { return e.node.Address() }
func (e endpoint) Metadata() map[string]string { return e.node.Metadata() }
func (e endpoint) Meta(k string) string        { return e.node.Metadata()[k] }
func (e endpoint) Node() registry.Node         { return e.node }

func (e endpoint) Host() string {
	if h, _, err := net.SplitHostPort(e.node.Address()); err == nil {
		return h
	}
	return e.node.Address()
}

func (e endpoint) Port() string {
	if _, p, err := net.SplitHostPort(e.node.Address()); err == nil {
		return p
	}
	return ""
}

func (e endpoint) Scheme() string {
	if s := e.Meta(registry.MetaScheme); s != "" {
		return s
	}
	return "http"
}

func (e endpoint) Prefix() string { return e.Meta(registry.MetaPrefix) }

func (e endpoint) URL() string {
	if u := e.Meta(registry.MetaPublicURL); u != "" {
		return u
	}
	u := e.Scheme() + "://" + e.Address()
	if p := e.Prefix(); p != "" {
		u += "/" + p
	}
	return u
}

// EndpointOption narrows the candidate node set.
type EndpointOption func(*endpointQuery)

type endpointQuery struct {
	name string
	meta map[string]string
}

// ByName selects a service by registry name.
func ByName(name string) EndpointOption {
	return func(q *endpointQuery) { q.name = name }
}

// ByMetadata keeps only nodes whose metadata[key] == value. Repeatable.
func ByMetadata(key, value string) EndpointOption {
	return func(q *endpointQuery) {
		if q.meta == nil {
			q.meta = map[string]string{}
		}
		q.meta[key] = value
	}
}

// HTTPEndpoints returns every ready node matching the filters.
func (c *clients) HTTPEndpoints(_ context.Context, opts ...EndpointOption) ([]Endpoint, error) {
	q := endpointQuery{}
	for _, o := range opts {
		o(&q)
	}
	if q.name == "" {
		return nil, fmt.Errorf("service registry: HTTPEndpoint requires ByName")
	}
	svc, err := c.registry.GetService(q.name)
	if err != nil {
		return nil, fmt.Errorf("service registry: resolving %q: %w", q.name, err)
	}
	nodes := filterByMetadata(svc.Nodes(), q.meta)
	out := make([]Endpoint, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, endpoint{name: q.name, node: n})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("service registry: no node for %q matching filters", q.name)
	}
	return out, nil
}

// HTTPEndpoint resolves one ready node matching the filters (selection §5.2).
func (c *clients) HTTPEndpoint(ctx context.Context, opts ...EndpointOption) (Endpoint, error) {
	q := endpointQuery{}
	for _, o := range opts {
		o(&q)
	}
	if q.name == "" {
		return nil, fmt.Errorf("service registry: HTTPEndpoint requires ByName")
	}
	svc, err := c.registry.GetService(q.name)
	if err != nil {
		return nil, fmt.Errorf("service registry: resolving %q: %w", q.name, err)
	}
	node, ok := c.selector.Pick(filterByMetadata(svc.Nodes(), q.meta))
	if !ok {
		return nil, fmt.Errorf("service registry: no selectable node for %q matching filters", q.name)
	}
	return endpoint{name: q.name, node: node}, nil
}

func filterByMetadata(nodes []registry.Node, meta map[string]string) []registry.Node {
	if len(meta) == 0 {
		return nodes
	}
	out := make([]registry.Node, 0, len(nodes))
	for _, n := range nodes {
		md := n.Metadata()
		match := true
		for k, v := range meta {
			if md[k] != v {
				match = false
				break
			}
		}
		if match {
			out = append(out, n)
		}
	}
	return out
}
