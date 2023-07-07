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
	children nodes
}

func (n *node) isEmpty() bool {
	return n.prefix == "" && len(n.children) == 0
}

func newTree() *node {
	return &node{}
}

type handlers map[string]http.Handler

func (h *handlers) add(method string, handler http.Handler) {
	if *h == nil {
		*h = make(handlers)
	}
	(*h)[method] = handler
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
			strings.HasPrefix(s, node.prefix) && node.ntype == static {
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

func (n *node) insert(method, path string, handler http.Handler) {
	if n.prefix == "" {
		// the tree is empty
		n.insertChild(method, path, handler)
		return
	}

	// traverse the tree until we cannot make futher progresses
	current := &node{
		children: nodes{n},
	}
walk:
	for {
		wildcard, i, wtype := nextWildcard(path)
		for _, node := range current.children {
			if i != -1 { // wildcard found
				if node.ntype != wtype || wildcard != node.prefix {
					panic("only one wildcard is allowed per path segment")
				}
				current = node
				path = path[i+len(wildcard):]
				continue walk
			}
			// the next node is the one having the same prefix of a static node
			if node.ntype == static && strings.HasPrefix(path, node.prefix) {
				current = node
				path = path[len(node.prefix):]
				continue walk
			}
		}

		current.insertChild(method, path, handler)
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

func (n *node) insertChild(method, path string, handler http.Handler) {
	current := n
	for {
		if path == "" {
			current.handlers.add(method, handler)
			return
		}

		wildcard, i, wtype := nextWildcard(path)
		if i != -1 { // wildcard found
			if len(current.children) != 0 {
				panic("only one wildcard is allowed per path segment")
			}
			next := &node{
				prefix: wildcard,
				ntype:  wtype,
			}
			current.children = nodes{next}
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
				// if one of the children is already a pattern, we cannot add the node
				for _, node := range current.children {
					if node.ntype == param || node.ntype == catchall {
						panic("only one wildcard is allowed per path segment")
					}
				}
				other := &node{
					prefix: childPrefix,
					ntype:  static,
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
		prefix:   n.prefix[len(prefix):],
		ntype:    static,
		handlers: n.handlers,
		children: n.children,
	}
	n.children = nodes{s}
	n.prefix = prefix
	n.handlers = nil
}

func stripSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}
