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
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// MethodAll is a constant used to specify that
// and endpoint should be used in all the HTTP methods.
const MethodAll = "*"

type paramsKey struct{}

// ServeMux is a http.Handler which can be used to dispatch requests to different
// handler functions via configurable routes, implementing the Router interface.
type ServeMux struct {
	// radix tree where routes are registered
	tree *trie

	path string // used for sub-routers
}

// NewServeMux creates a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{
		tree: newTree(),
	}
}

// ensure Mux implements Router interface.
var _ Router = (*ServeMux)(nil)

func (m *ServeMux) Route(path string, f func(Router)) {
	path = filepath.Join(m.path, path)
	sub := &ServeMux{
		tree: m.tree,
		path: path,
	}
	f(sub)
}

func (m *ServeMux) Method(method, path string, handler Handler) {
	if m.path != "" {
		var err error
		path, err = url.JoinPath(m.path, path)
		if err != nil {
			panic(err)
		}
	}
	if method == "" {
		panic("method must not be empty")
	}
	if len(path) < 1 || path[0] != '/' {
		panic("path must begin with '/' in path '" + path + "'")
	}
	if handler == nil {
		panic("handle must not be nil")
	}

	m.tree.insert(method, path, handler)
}

func (m *ServeMux) Get(path string, handler Handler) {
	m.Method(http.MethodGet, path, handler)
}

func (m *ServeMux) Head(path string, handler Handler) {
	m.Method(http.MethodHead, path, handler)
}

func (m *ServeMux) Post(path string, handler Handler) {
	m.Method(http.MethodPost, path, handler)
}

func (m *ServeMux) Put(path string, handler Handler) {
	m.Method(http.MethodPut, path, handler)
}

func (m *ServeMux) Patch(path string, handler Handler) {
	m.Method(http.MethodPatch, path, handler)
}

func (m *ServeMux) Delete(path string, handler Handler) {
	m.Method(http.MethodDelete, path, handler)
}

func (m *ServeMux) Connect(path string, handler Handler) {
	m.Method(http.MethodConnect, path, handler)
}

func (m *ServeMux) Options(path string, handler Handler) {
	m.Method(http.MethodOptions, path, handler)
}

func (m *ServeMux) Walk(ctx context.Context, f WalkFunc) {
	m.tree.root.walk(ctx, "", f)
}

func (n *node) walk(ctx context.Context, prefix string, f WalkFunc) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if n == nil {
		return
	}

	var current string
	switch n.ntype {
	case static:
		current = n.prefix
	case param:
		current = ":" + n.prefix
	case catchall:
		current = "*" + n.prefix
	default:
		panic("node type not recognised")
	}

	path := prefix + current

	for method, h := range n.handlers.perMethod {
		f(method, path, h)
	}

	if g := n.handlers.global; g != nil {
		f(MethodAll, path, g)
	}

	for _, c := range n.children {
		c.walk(ctx, path, f)
	}
}

func (m *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n, params, ok := m.tree.root.lookup(r.URL.Path, m.tree.getParams)
	if !ok {
		m.notFound(w, r)
		return
	}
	handler, ok := n.handlers.get(r.Method)
	if !ok {
		m.notFound(w, r)
		return
	}
	if params == nil {
		handler.ServeHTTP(w, r, nil)
	} else {
		handler.ServeHTTP(w, r, *params)
	}
	if params != nil {
		m.tree.putParams(params)
	}
}

func (m *ServeMux) notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func trimPrefix(prefix string) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(w http.ResponseWriter, r *http.Request, p Params) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
			next.ServeHTTP(w, r, p)
		})
	}
}

func (m *ServeMux) mountRouter(prefix string, r Router, t string) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	r.Walk(ctx, func(method, path string, handler Handler) {
		path, _ = url.JoinPath(prefix, path)
		m.Method(method, path, trimPrefix(t)(handler))
	})
}

func (m *ServeMux) Mount(path string, handler Handler) {
	prefix, _ := url.JoinPath("/", m.path, path)
	if router, ok := handler.(Router); ok {
		m.mountRouter(path, router, prefix)
		return
	}
	m.Use(trimPrefix(prefix))
	m.Handle(path+"/*", handler)
}

func (m *ServeMux) Use(middlewares ...Middleware) {
	// TODO
}

func (m *ServeMux) Handle(path string, handler Handler) {
	m.Method(MethodAll, path, handler)
}
