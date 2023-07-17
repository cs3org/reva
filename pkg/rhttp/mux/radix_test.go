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
	"net/http/httptest"
	"testing"

	"github.com/cs3org/reva/pkg/rhttp/middlewares"
	"github.com/stretchr/testify/assert"
)

func TestRadixLookup(t *testing.T) {
	tests := []struct {
		tree   *node
		path   string
		node   *node
		params Params
		ok     bool
	}{
		{
			tree: &node{
				prefix: "/",
				ntype:  static,
			},
			path: "/",
			node: &node{
				prefix: "/",
				ntype:  static,
			},
			params: nil,
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/blog",
				ntype:  static,
			},
			path:   "/",
			node:   nil,
			params: nil,
			ok:     false,
		},
		{
			tree: &node{
				prefix: "/blog",
				ntype:  static,
			},
			path: "/blog",
			node: &node{
				prefix: "/blog",
				ntype:  static,
			},
			params: nil,
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/blog",
				ntype:  static,
			},
			path:   "/blog/post1",
			node:   nil,
			params: nil,
			ok:     false,
		},
		{
			tree: &node{
				prefix: "/blog",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "item",
						ntype:  param,
					},
				},
			},
			path: "/blog/post1",
			node: &node{
				prefix: "item",
				ntype:  param,
			},
			params: Params{"item": "post1"},
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "blog",
						ntype:  static,
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix: "earch",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "item",
										ntype:  param,
										children: nodes{
											&node{
												prefix: "/something",
												ntype:  static,
											},
										},
									},
								},
							},
							&node{
								prefix: "upport",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "key",
										ntype:  catchall,
									},
								},
							},
						},
					},
				},
			},
			path: "/support",
			node: &node{
				prefix: "key",
				ntype:  catchall,
			},
			params: Params{"key": ""},
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "blog",
						ntype:  static,
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix: "earch",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "item",
										ntype:  param,
										children: nodes{
											&node{
												prefix: "/something",
												ntype:  static,
											},
										},
									},
								},
							},
							&node{
								prefix: "upport",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "key",
										ntype:  catchall,
									},
								},
							},
						},
					},
				},
			},
			path: "/search/123/something",
			node: &node{
				prefix: "/something",
				ntype:  static,
			},
			params: Params{"item": "123"},
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "blog",
						ntype:  static,
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix: "earch",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "item",
										ntype:  param,
										children: nodes{
											&node{
												prefix: "/something",
												ntype:  static,
											},
										},
									},
								},
							},
							&node{
								prefix: "upport",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "key",
										ntype:  catchall,
									},
								},
							},
						},
					},
				},
			},
			path: "/support/hello/everyone",
			node: &node{
				prefix: "key",
				ntype:  catchall,
			},
			params: Params{"key": "hello/everyone"},
			ok:     true,
		},
		{
			tree: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "blog",
						ntype:  static,
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix: "earch",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "item",
										ntype:  param,
									},
								},
							},
							&node{
								prefix: "upport",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "key",
										ntype:  catchall,
									},
								},
							},
						},
					},
				},
			},
			path:   "/search/1234/item",
			node:   nil,
			params: nil,
			ok:     false,
		},
	}

	for _, tt := range tests {
		node, params, ok := tt.tree.lookup(tt.path)
		assert.Equal(t, tt.node, node)
		assert.Equal(t, tt.params, params)
		assert.Equal(t, tt.ok, ok)
	}
}

func TestRadixInsert(t *testing.T) {
	tests := []struct {
		init         *trie
		method, path string
		exp          *node
	}{
		{
			init:   newTree(),
			method: "GET",
			path:   "/",
			exp: &node{
				prefix: "/",
				ntype:  static,
			},
		},
		{
			init:   newTree(),
			method: "POST",
			path:   "/something",
			exp: &node{
				prefix: "/something",
				ntype:  static,
			},
		},
		{
			init:   newTree(),
			method: "POST",
			path:   "/something/test/multi/level",
			exp: &node{
				prefix: "/something/test/multi/level",
				ntype:  static,
			},
		},
		{
			init:   newTree(),
			method: "GET",
			path:   "/something/:item",
			exp: &node{
				prefix: "/something/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "item",
						ntype:  param,
					},
				},
			},
		},
		{
			init:   newTree(),
			method: "GET",
			path:   "/:item",
			exp: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "item",
						ntype:  param,
					},
				},
			},
		},
		{
			init:   newTree(),
			method: "GET",
			path:   "/:item/some/thing",
			exp: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "item",
						ntype:  param,
						children: nodes{
							&node{
								prefix: "/some/thing",
								ntype:  static,
							},
						},
					},
				},
			},
		},
		{
			init:   newTree(),
			method: "GET",
			path:   "/path/:item/some/thing",
			exp: &node{
				prefix: "/path/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "item",
						ntype:  param,
						children: nodes{
							&node{
								prefix: "/some/thing",
								ntype:  static,
							},
						},
					},
				},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/key/search",
					ntype:  static,
				},
			},
			method: "POST",
			path:   "/key/support",
			exp: &node{
				prefix: "/key/s",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "earch",
						ntype:  static,
					},
					&node{
						prefix: "upport",
						ntype:  static,
					},
				},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/key/search",
					ntype:  static,
				},
			},
			method: "POST",
			path:   "/key/:other",
			exp: &node{
				prefix: "/key/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "search",
						ntype:  static,
					},
					&node{
						prefix: "other",
						ntype:  param,
					},
				},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/",
					ntype:  static,
					children: nodes{
						{
							prefix: "value/",
							ntype:  static,
						},
					},
				},
			},
			method: "POST",
			path:   "/:key",
			exp: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					{
						prefix: "value/",
						ntype:  static,
					},
					{
						prefix: "key",
						ntype:  param,
					},
				},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/",
					ntype:  static,
					children: nodes{
						&node{
							prefix: "blog",
							ntype:  static,
						},
						&node{
							prefix: "search/",
							ntype:  static,
							children: nodes{
								&node{
									prefix: "item",
									ntype:  param,
									children: nodes{
										&node{
											prefix: "/something",
											ntype:  static,
										},
									},
								},
							},
						},
					},
				},
			},
			method: "POST",
			path:   "/support/*key",
			exp: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "blog",
						ntype:  static,
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix: "earch/",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "item",
										ntype:  param,
										children: nodes{
											&node{
												prefix: "/something",
												ntype:  static,
											},
										},
									},
								},
							},
							&node{
								prefix: "upport/",
								ntype:  static,
								children: nodes{
									&node{
										prefix: "key",
										ntype:  catchall,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt.init.insert(tt.method, tt.path, nil, nil)
		assert.Equal(t, tt.exp, tt.init.root)
	}
}

func TestInsertOptions(t *testing.T) {
	tests := []struct {
		init         *trie
		method, path string
		opt          *Options
		exp          *node
	}{
		{
			init:   newTree(),
			method: "GET",
			path:   "/test",
			opt:    &Options{Unprotected: true},
			exp: &node{
				prefix: "/test",
				ntype:  static,
				opts:   nodeOptions{opts: nilMap[*Options]{"GET": &Options{Unprotected: true}}},
			},
		},
		{
			init:   newTree(),
			method: MethodAll,
			path:   "/test",
			opt:    &Options{Unprotected: true},
			exp: &node{
				prefix: "/test",
				ntype:  static,
				opts:   nodeOptions{global: &Options{Unprotected: true}},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/",
					ntype:  static,
					opts:   nodeOptions{},
				},
			},
			method: "GET",
			path:   "/blog",
			opt:    &Options{Unprotected: true},
			exp: &node{
				prefix: "/",
				ntype:  static,
				opts:   nodeOptions{},
				children: nodes{
					{
						prefix: "blog",
						ntype:  static,
						opts:   nodeOptions{opts: nilMap[*Options]{"GET": &Options{Unprotected: true}}},
					},
				},
			},
		},
		{
			init: &trie{
				root: &node{
					prefix: "/",
					ntype:  static,
					opts:   nodeOptions{global: &Options{Unprotected: true}},
				},
			},
			method: "GET",
			path:   "/blog",
			opt:    nil,
			exp: &node{
				prefix: "/",
				ntype:  static,
				opts:   nodeOptions{global: &Options{Unprotected: true}},
				children: nodes{
					{
						prefix: "blog",
						ntype:  static,
						opts:   nodeOptions{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt.init.insert(tt.method, tt.path, nil, tt.opt)
		assert.Equal(t, tt.exp, tt.init.root)
	}
}

func TestMultipleMiddlewaresAlongTheWay(t *testing.T) {
	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	var count int
	m := middlewares.Middleware(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count++
			h.ServeHTTP(w, r)
		})
	})

	tree := newTree()
	tree.root.middlewareFactory = func(o *Options) []middlewares.Middleware {
		return o.Middlewares
	}

	tree.insert("GET", "/", nop, &Options{Middlewares: []middlewares.Middleware{m}})
	tree.insert("POST", "/", nop, &Options{Middlewares: []middlewares.Middleware{m}})
	tree.insert(MethodAll, "/test/path", nop, &Options{Middlewares: []middlewares.Middleware{m}})
	tree.insert(MethodAll, "/testing", nop, &Options{Middlewares: []middlewares.Middleware{m}})
	tree.insert("POST", "/test/path/other", nop, &Options{Middlewares: []middlewares.Middleware{m}})
	tree.insert("POST", "/test/path/other/some/thing", nop, &Options{Middlewares: []middlewares.Middleware{m}})

	n, _, ok := tree.root.lookup("/test/path/other/some/thing")
	assert.Equal(t, true, ok)
	assert.Equal(t, 1, len(n.opts.opts["POST"].Middlewares))

	handler, ok := n.handlers.get("POST")
	assert.Equal(t, true, ok)
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/test/path/other/some/thing", nil)
	handler.ServeHTTP(w, r)
	assert.Equal(t, 4, count)
}
