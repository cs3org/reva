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

package mux

import (
	"net/http"
	"strings"
)

type nodetype int

const (
	static nodetype = iota
	param
	catchall
)

type node struct {
	prefix   string
	ntype    nodetype
	handlers handlers
	opts     nodeOptions
	children nodes

	middlewareFactory func(*Options) []Middleware
}

type nodeOptions struct {
	global *Options // these applies to all methods
	opts   nilMap[*Options]
}

func (n *nodeOptions) set(method string, o *Options) {
	if method == MethodAll {
		n.global = o
		return
	}
	n.opts.add(method, o)
}

func (n *nodeOptions) get(method string) *Options {
	global := n.global
	perMethod := n.opts[method]
	if global != nil {
		return global.merge(perMethod)
	}
	return perMethod
}

type handlers struct {
	global    http.Handler
	perMethod nilMap[http.Handler]
}

func (h *handlers) set(method string, handler http.Handler) {
	if method == MethodAll {
		h.global = handler
		return
	}
	h.perMethod.add(method, handler)
}

func (h *handlers) get(method string) (http.Handler, bool) {
	// always prefer the one specific for the required method
	// otherwise fall back to the global handler for all methods
	// if provided
	if h, ok := h.perMethod[method]; ok {
		return h, true
	}
	return h.global, h.global != nil
}

func (n *node) isEmpty() bool {
	return n.prefix == "" && len(n.children) == 0
}

func newTree() *node {
	return &node{}
}

type nilMap[T any] map[string]T

func (m *nilMap[T]) add(method string, v T) {
	if *m == nil {
		*m = make(nilMap[T])
	}
	(*m)[method] = v
}

type nodes []*node

func (p *Params) add(key, val string) {
	if *p == nil {
		*p = make(Params)
	}
	(*p)[key] = val
}

// search returns the node from the list of nodes having
// the same prefix of s.
func (n nodes) search(s string) (*node, bool) {
	for _, node := range n {
		if node.ntype == catchall || node.ntype == param ||
			s == node.prefix && node.ntype == static && len(node.children) == 0 ||
			strings.HasPrefix(s, node.prefix) && node.ntype == static && len(node.children) != 0 {
			return node, true
		}
	}
	return nil, false
}

func min(n, m int) int {
	if n < m {
		return n
	}
	return m
}

func commonPrefix(a, b string) string {
	i := 0
	for ; i < min(len(a), len(b)) && a[i] == b[i]; i++ {
	}
	return a[:i]
}

// longestCommonPrefix returns the node and the index of the node in the
// list of nodes having the longest common prefix with s, together with the prefix.
// The index is -1 if the common prefix does not exist (empty list or prefix is "")
func (n nodes) longestCommonPrefix(s string) (string, *node, bool) {
	var match *node
	var prefix string
	var has bool
	for _, node := range n {
		if p := commonPrefix(node.prefix, s); len(p) > len(prefix) {
			match, prefix, has = node, p, true
		}
	}
	return prefix, match, has
}

func (n *node) lookup(path string) (*node, Params, bool) {
	// path can be something like
	// path is already cleaned
	// /search/aa/somenthing/bb

	if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
		return nil, nil, false
	}
	if len(path) == len(n.prefix) {
		return n, nil, true
	}

	path = path[len(n.prefix):]

	// path has prefix of the node n
	current := n
	var found bool
	var params Params

	for {
		// select next child having path as prefix
		current, found = current.children.search(path)
		if !found {
			return nil, nil, false
		}

		prefix := current.prefix

		switch current.ntype {
		case static:
			path = path[len(prefix):]
		case param:
			// the prefix is how the param is named
			path = stripSlash(path)
			i := strings.IndexByte(path, '/')
			if i == -1 {
				i = len(path)
			}
			params.add(prefix, path[:i])
			path = path[i:]
		case catchall:
			path = stripSlash(path)
			params.add(prefix, path)
			path = ""
		}

		if path == "" {
			return current, params, true
		}
	}
}

func wildcardType(c byte) nodetype {
	switch c {
	case ':':
		return param
	case '*':
		return catchall
	}
	panic("wildcard character not recognised")
}

// nextWildcard return the wildcard and its type (param, catchall)
// and the index in s of it. The index is -1 if the next token
// is not a wildcard.
func nextWildcard(s string) (string, int, nodetype) {
	// the string can be :key/... rest if is a wildcard
	// otherwise is just a part of a normal path, like search/:item/...
	if len(s) == 0 {
		return "", -1, 0
	}
	if s[0] == ':' || s[0] == '*' {
		// this is a wildcard
		// TODO: this panics if the user provide a string like /:
		for i, c := range s[1:] {
			// check if the wildcard is valid (i.e. does not have any ':' and '*' chars)
			if c == ':' || c == '*' {
				panic("wildcard in string " + s + " is not valid")
			}
			if c == '/' {
				return s[1 : i+1], 1, wildcardType(s[0])
			}
		}
		return s[1:], 1, wildcardType(s[0])
	}
	return "", -1, 0
}

func (n *node) mergeOptions(method string, opts *Options) *Options {
	return n.opts.get(method).merge(opts)
}

func (n *node) insert(method, path string, handler http.Handler, opts *Options) {
	if n.prefix == "" {
		// the tree is empty
		n.insertChild(method, path, handler, opts)
		return
	}

	// traverse the tree until we cannot make futher progresses
	current := &node{
		children:          nodes{n},
		middlewareFactory: n.middlewareFactory,
	}
walk:
	for {
		wildcard, i, _ := nextWildcard(path)
		for _, node := range current.children {
			if i != -1 { // wildcard found
				current = node
				opts = n.mergeOptions(method, opts)
				path = path[i+len(wildcard):]
				continue walk
			}
			// the next node is the one having the same prefix of a static node
			if node.ntype == static && strings.HasPrefix(path, node.prefix) {
				current = node
				opts = n.mergeOptions(method, opts)
				path = path[len(node.prefix):]
				continue walk
			}
		}

		current.insertChild(method, path, handler, opts)
		return
	}
}

func wildcardIndex(s string) int {
	for i, c := range s {
		if c == ':' || c == '*' {
			return i
		}
	}
	return -1
}

func (n *node) insertChild(method, path string, handler http.Handler, opts *Options) {
	current := n
	for {
		if path == "" {
			if handler != nil {
				if n.middlewareFactory != nil && opts != nil {
					for _, mid := range n.middlewareFactory(opts) {
						handler = mid(handler)
					}
				}
				current.handlers.set(method, handler)
			}
			if opts != nil {
				current.opts.set(method, opts)
			}
			return
		}

		wildcard, i, wtype := nextWildcard(path)
		if i != -1 { // wildcard found
			next := &node{
				prefix:            wildcard,
				ntype:             wtype,
				middlewareFactory: n.middlewareFactory,
			}
			current.children = append(current.children, next)
			path = path[len(wildcard)+1:]
			current = next
			continue
		}

		prefix, target, has := current.children.longestCommonPrefix(path)
		if has {
			target.split(prefix)
			current = target

			// strip prefix from path for building next node
			path = path[len(prefix):]
		}

		windex := wildcardIndex(path)
		childPrefix := path
		if windex != -1 {
			childPrefix = childPrefix[:windex]
		}
		if childPrefix != "" {
			// special case when tree is empty
			if current.isEmpty() {
				current.prefix = childPrefix
				current.ntype = static
			} else {
				other := &node{
					prefix:            childPrefix,
					ntype:             static,
					middlewareFactory: n.middlewareFactory,
				}
				current.children = append(current.children, other)
				current = other

			}
			path = path[len(childPrefix):]
		}
	}
}

func (n *node) split(prefix string) {
	s := &node{
		prefix:            n.prefix[len(prefix):],
		ntype:             static,
		handlers:          n.handlers,
		children:          n.children,
		opts:              n.opts,
		middlewareFactory: n.middlewareFactory,
	}
	n.children = nodes{s}
	n.prefix = prefix
	n.handlers = handlers{}
}

func stripSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}
