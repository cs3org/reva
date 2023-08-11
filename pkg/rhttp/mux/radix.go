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
	"strings"
	"sync"
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

type trie struct {
	root *node

	paramsPool sync.Pool

	// mutex used only during registration of paths
	// lookup is thread-safe if not registrations are occurring
	m sync.Mutex
}

func (t *trie) getParams() *Params {
	ps, _ := t.paramsPool.Get().(*Params)
	*ps = (*ps)[0:0] // reset slice
	return ps
}

func (t *trie) putParams(ps *Params) {
	if ps != nil {
		t.paramsPool.Put(ps)
	}
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
	if method == MethodAll {
		return n.global
	}
	global := n.global
	perMethod := n.opts[method]
	return global.merge(perMethod)
}

func (n *nodeOptions) merge(other *nodeOptions) *nodeOptions {
	merged := nodeOptions{}
	merged.global = n.global
	merged.opts = nilMap[*Options]{}
	if other.global != nil {
		merged.global = merged.global.merge(other.global)
	}
	for method, opt := range n.opts {
		merged.set(method, other.get(method).merge(opt))
	}
	for method, opt := range other.opts {
		if _, ok := merged.opts[method]; ok {
			continue
		}
		merged.opts.add(method, opt)
	}
	return &merged
}

type handlers struct {
	global    Handler
	perMethod nilMap[Handler]
}

func (h *handlers) set(method string, handler Handler) {
	if method == MethodAll {
		h.global = handler
		return
	}
	h.perMethod.add(method, handler)
}

func (h *handlers) get(method string) (Handler, bool) {
	// always prefer the one specific for the required method
	// otherwise fall back to the global handler for all methods
	// if provided
	if h, ok := h.perMethod[method]; ok {
		return h, true
	}
	return h.global, h.global != nil
}

func (h *handlers) isEmpty() bool {
	return h.global == nil && len(h.perMethod) == 0
}

func (n *node) isEmpty() bool {
	return n.prefix == "" && len(n.children) == 0
}

func newTree() *trie {
	return &trie{
		root:       &node{},
		m:          sync.Mutex{},
		paramsPool: sync.Pool{New: func() any { return &Params{} }},
	}
}

func (t *trie) insert(method, path string, h Handler, opts *Options) {
	t.m.Lock()
	defer t.m.Unlock()
	t.root.insert(method, path, h, opts)
}

type nilMap[T any] map[string]T

func (m *nilMap[T]) add(method string, v T) {
	if *m == nil {
		*m = make(nilMap[T])
	}
	(*m)[method] = v
}

type nodes []*node

func (p *Params) add(key, val string, n func() *Params) {
	if *p == nil {
		new := n()
		*p = *new
	}
	*p = append(*p, Param{key, val})
}

// search returns the node from the list of nodes having
// the same prefix of s.
func (n nodes) search(s string) (*node, bool) {
	for _, node := range n {
		if node.ntype == catchall || node.ntype == param ||
			s == node.prefix && node.ntype == static && len(node.children) == 0 ||
			node.ntype == static && strings.HasPrefix(s, node.prefix) && len(node.children) != 0 {
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
// The index is -1 if the common prefix does not exist (empty list or prefix is "").
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

func (n *node) lookup(path string, ps func() *Params) (*node, Params, bool) {
	// path can be something like
	// path is already cleaned
	// /search/aa/somenthing/bb

	if len(path) < len(n.prefix) || path[:len(n.prefix)] != n.prefix {
		return nil, nil, false
	}
	if len(path) == len(n.prefix) {
		// we found the node. but if the current node does not
		// have any handler, and one of the children is a catch all
		// node, the catch all might be an empty string.
		// so we return the child node
		n, params := n.nextContainingHandlers(nil, ps)
		return n, params, true
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
			params.add(prefix, path[:i], ps)
			path = path[i:]
		case catchall:
			path = stripSlash(path)
			params.add(prefix, path, ps)
			path = ""
		}

		if path == "" {
			// we found the node. but if the current node does not
			// have any handler, and one of the children is a catch all
			// node, the catch all might be an empty string.
			// so we return the child node
			n, params := current.nextContainingHandlers(params, ps)
			return n, params, true
		}
	}
}

func (n *node) nextContainingHandlers(params Params, ps func() *Params) (*node, Params) {
	if n.handlers.isEmpty() {
		for _, c := range n.children {
			if c.ntype == catchall {
				params.add(c.prefix, "", ps)
				return c, params
			}
		}
	}
	return n, params
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
	nodeOpts := n.opts.get(method)
	return opts.merge(nodeOpts)
}

func (n *node) insert(method, path string, handler Handler, opts *Options) {
	if n.prefix == "" {
		// the tree is empty
		n.insertChild(method, path, handler, nil, opts)
		return
	}

	var merged *Options
	// traverse the tree until we cannot make further progresses
	current := &node{
		children:          nodes{n},
		middlewareFactory: n.middlewareFactory,
	}
walk:
	for {
		wildcard, i, wtype := nextWildcard(path)
		for _, node := range current.children {
			// if we found a wildcard in the path as next token (i != -1),
			// we look for a node containing a wildcard of the same type
			// TODO (gdelmont): we should bail out if we find a wildcard node
			// with different type, and if same type different wildcard str
			if i != -1 && node.ntype == wtype {
				current = node
				merged = node.mergeOptions(method, merged)
				path = path[i+len(wildcard):]
				continue walk
			}
			// the next node is the one having the same prefix of a static node
			if node.ntype == static && strings.HasPrefix(path, node.prefix) {
				current = node
				merged = node.mergeOptions(method, merged)
				path = path[len(node.prefix):]
				continue walk
			}
		}

		current.insertChild(method, path, handler, merged, opts)
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

func (n *node) insertChild(method, path string, handler Handler, merged, opts *Options) {
	current := n
	for {
		if path == "" {
			if handler != nil {
				if n.middlewareFactory != nil {
					merged = merged.merge(opts)
					for _, mid := range n.middlewareFactory(merged) {
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
	n.opts = nodeOptions{}
}

func stripSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}
