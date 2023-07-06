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
	}

	for _, tt := range tests {
		node, params, ok := tt.tree.lookup(tt.path)
		assert.Equal(t, tt.node, node)
		assert.Equal(t, tt.params, params)
		assert.Equal(t, tt.ok, ok)
	}
}
