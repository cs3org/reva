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
	"sync"
)

type paramsKey struct{}

type ServeMux struct {
	// radix tree where routes are registered
	tree *node

	path string // used for sub-routers

	// mutex used only during registration of paths
	// lookup is thread-safe if not registrations are occurring
	m *sync.Mutex
}

func NewServeMux() *ServeMux {
	return &ServeMux{
		tree: newTree(),
		m:    &sync.Mutex{},
	}
}

// ensure Mux implements Router interface
var _ Router = (*ServeMux)(nil)

func (m *ServeMux) Route(path string, f func(Router)) {
	sub := &ServeMux{
		tree: m.tree,
		path: filepath.Join(m.path, path),
		m:    m.m,
	}
	f(sub)
}

func (m *ServeMux) Handle(method, path string, handler http.Handler) {
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

	m.m.Lock()
	defer m.m.Unlock()

	m.tree.insert(method, path, handler)
}

func (m *ServeMux) Get(path string, handler http.Handler) {
	m.Handle(http.MethodGet, path, handler)
}

func (m *ServeMux) Head(path string, handler http.Handler) {
	m.Handle(http.MethodHead, path, handler)
}

func (m *ServeMux) Post(path string, handler http.Handler) {
	m.Handle(http.MethodPost, path, handler)
}

func (m *ServeMux) Put(path string, handler http.Handler) {
	m.Handle(http.MethodPut, path, handler)
}

func (m *ServeMux) Delete(path string, handler http.Handler) {
	m.Handle(http.MethodDelete, path, handler)
}

func (m *ServeMux) Connect(path string, handler http.Handler) {
	m.Handle(http.MethodConnect, path, handler)
}

func (m *ServeMux) Options(path string, handler http.Handler) {
	m.Handle(http.MethodOptions, path, handler)
}

func (m *ServeMux) Walk(ctx context.Context, f WalkFunc) {
	m.tree.walk(ctx, "", f)
}

func (n *node) walk(ctx context.Context, prefix string, f WalkFunc) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if n == nil || n.isEmpty() {
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
	for method, h := range n.handlers {
		f(method, path, h)
	}

	for _, c := range n.children {
		c.walk(ctx, path, f)
	}
}

func (m *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n, params, ok := m.tree.lookup(r.URL.Path)
	if !ok {
		m.notFound(w, r)
		return
	}
	handler, ok := n.handlers[r.Method]
	if !ok {
		m.notFound(w, r)
		return
	}

	if len(params) > 0 {
		ctx := context.WithValue(r.Context(), paramsKey{}, params)
		r = r.WithContext(ctx)
	}
	handler.ServeHTTP(w, r)
}

func (m *ServeMux) notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
