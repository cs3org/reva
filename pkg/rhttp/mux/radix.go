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
	handlers map[string]http.Handler
	children nodes
}

type nodes []*node

func (p *Params) add(key, val string) {
	if *p == nil {
		*p = make(Params)
	}
	(*p)[key] = val
}

// seach returns the node from the list of nodes having
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

func stripSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}
