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
	"testing"

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
				prefix: "upport",
				ntype:  static,
				children: nodes{
					&node{
						prefix: "key",
						ntype:  catchall,
					},
				},
			},
			params: nil,
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
		init         *node
		method, path string
		exp          *node
		panic        bool
	}{
		{
			init:   newTree(),
			method: "GET",
			path:   "/",
			exp: &node{
				prefix:   "/",
				ntype:    static,
				handlers: handlers{"GET": nil},
			},
		},
		{
			init:   newTree(),
			method: "POST",
			path:   "/something",
			exp: &node{
				prefix:   "/something",
				ntype:    static,
				handlers: handlers{"POST": nil},
			},
		},
		{
			init:   newTree(),
			method: "POST",
			path:   "/something/test/multi/level",
			exp: &node{
				prefix:   "/something/test/multi/level",
				ntype:    static,
				handlers: handlers{"POST": nil},
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
						prefix:   "item",
						ntype:    param,
						handlers: handlers{"GET": nil},
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
						prefix:   "item",
						ntype:    param,
						handlers: handlers{"GET": nil},
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
								prefix:   "/some/thing",
								ntype:    static,
								handlers: handlers{"GET": nil},
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
								prefix:   "/some/thing",
								ntype:    static,
								handlers: handlers{"GET": nil},
							},
						},
					},
				},
			},
		},
		{
			init: &node{
				prefix:   "/key/search",
				ntype:    static,
				handlers: handlers{"GET": nil},
			},
			method: "POST",
			path:   "/key/support",
			exp: &node{
				prefix: "/key/s",
				ntype:  static,
				children: nodes{
					&node{
						prefix:   "earch",
						ntype:    static,
						handlers: handlers{"GET": nil},
					},
					&node{
						prefix:   "upport",
						ntype:    static,
						handlers: handlers{"POST": nil},
					},
				},
			},
		},
		{
			init: &node{
				prefix:   "/key/search",
				ntype:    static,
				handlers: handlers{"GET": nil},
			},
			method: "POST",
			path:   "/key/:item",
			panic:  true,
		},
		{
			init: &node{
				prefix:   "/key/",
				ntype:    static,
				handlers: handlers{"GET": nil},
				children: nodes{
					&node{
						prefix:   "item",
						ntype:    param,
						handlers: handlers{"get": nil},
					},
				},
			},
			method: "POST",
			path:   "/key/search",
			panic:  true,
		},
		{
			init: &node{
				prefix: "/",
				ntype:  static,
				children: nodes{
					&node{
						prefix:   "blog",
						ntype:    static,
						handlers: handlers{"get": nil},
					},
					&node{
						prefix:   "search/",
						ntype:    static,
						handlers: handlers{"PUT": nil},
						children: nodes{
							&node{
								prefix:   "item",
								ntype:    param,
								handlers: handlers{"POST": nil},
								children: nodes{
									&node{
										prefix:   "/something",
										ntype:    static,
										handlers: handlers{"GET": nil, "POST": nil},
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
						prefix:   "blog",
						ntype:    static,
						handlers: handlers{"get": nil},
					},
					&node{
						prefix: "s",
						ntype:  static,
						children: nodes{
							&node{
								prefix:   "earch/",
								ntype:    static,
								handlers: handlers{"PUT": nil},
								children: nodes{
									&node{
										prefix:   "item",
										ntype:    param,
										handlers: handlers{"POST": nil},
										children: nodes{
											&node{
												prefix:   "/something",
												ntype:    static,
												handlers: handlers{"GET": nil, "POST": nil},
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
										prefix:   "key",
										ntype:    catchall,
										handlers: handlers{"POST": nil},
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
		if tt.panic {
			assert.Panics(t, func() {
				tt.init.insert(tt.method, tt.path, nil)
			})
		} else {
			tt.init.insert(tt.method, tt.path, nil)
			assert.Equal(t, tt.exp, tt.init)
		}
	}
}
