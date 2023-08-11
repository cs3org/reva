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

package mux_test

// import (
// 	"context"
// 	"net/http"
// 	"net/http/httptest"
// 	"reflect"
// 	"testing"

// 	"github.com/cs3org/reva/pkg/rhttp/middlewares"
// 	"github.com/cs3org/reva/pkg/rhttp/mux"
// 	"github.com/gdexlab/go-render/render"
// 	"gotest.tools/assert"
// )

// type mockResponseWriter struct{}

// func (m *mockResponseWriter) Header() (h http.Header) {
// 	return http.Header{}
// }

// func (m *mockResponseWriter) Write(p []byte) (n int, err error) {
// 	return len(p), nil
// }

// func (m *mockResponseWriter) WriteString(s string) (n int, err error) {
// 	return len(s), nil
// }

// func (m *mockResponseWriter) WriteHeader(int) {}

// func TestRouterAPIs(t *testing.T) {
// 	mux := mux.NewServeMux()

// 	var get, post, head, put, connect, delete, options, patch bool
// 	mux.Get("/get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		get = true
// 	}))

// 	mux.Post("/post", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		post = true
// 	}))

// 	mux.Head("/head", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		head = true
// 	}))

// 	mux.Patch("/patch", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		patch = true
// 	}))

// 	mux.Put("/put", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		put = true
// 	}))

// 	mux.Connect("/connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		connect = true
// 	}))

// 	mux.Options("/options", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		options = true
// 	}))

// 	mux.Delete("/delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		delete = true
// 	}))

// 	w := new(mockResponseWriter)

// 	r, _ := http.NewRequest(http.MethodGet, "/get", nil)
// 	mux.ServeHTTP(w, r)
// 	if !get {
// 		t.Fatal("routing GET failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodPost, "/post", nil)
// 	mux.ServeHTTP(w, r)
// 	if !post {
// 		t.Fatal("routing POST failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodHead, "/head", nil)
// 	mux.ServeHTTP(w, r)
// 	if !head {
// 		t.Fatal("routing GET failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodPut, "/put", nil)
// 	mux.ServeHTTP(w, r)
// 	if !put {
// 		t.Fatal("routing PUT failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodConnect, "/connect", nil)
// 	mux.ServeHTTP(w, r)
// 	if !connect {
// 		t.Fatal("routing CONNECT failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodOptions, "/options", nil)
// 	mux.ServeHTTP(w, r)
// 	if !options {
// 		t.Fatal("routing OPTIONS failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodDelete, "/delete", nil)
// 	mux.ServeHTTP(w, r)
// 	if !delete {
// 		t.Fatal("routing DELETE failed")
// 	}

// 	r, _ = http.NewRequest(http.MethodPatch, "/patch", nil)
// 	mux.ServeHTTP(w, r)
// 	if !patch {
// 		t.Fatal("routing PATCH failed")
// 	}
// }

// func TestParamsResolved(t *testing.T) {
// 	router := mux.NewServeMux()

// 	var hit bool
// 	router.Get("/user/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hit = true
// 		want := mux.Params{{"name", "gopher"}}
// 		got := mux.ParamsFromRequest(r)
// 		if !reflect.DeepEqual(want, got) {
// 			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 		}
// 	}))

// 	w := new(mockResponseWriter)

// 	r, _ := http.NewRequest(http.MethodGet, "/user/gopher", nil)
// 	router.ServeHTTP(w, r)

// 	if !hit {
// 		t.Fatal("routing failed")
// 	}
// }

// func TestParamsCatchAll(t *testing.T) {
// 	router := mux.NewServeMux()

// 	var hit bool
// 	router.Get("/path/*path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hit = true
// 		want := mux.Params{{"path", "this/is/a/path"}}
// 		got := mux.ParamsFromRequest(r)
// 		if !reflect.DeepEqual(want, got) {
// 			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 		}
// 	}))

// 	w := new(mockResponseWriter)

// 	r, _ := http.NewRequest(http.MethodGet, "/path/this/is/a/path", nil)
// 	router.ServeHTTP(w, r)

// 	if !hit {
// 		t.Fatal("routing failed")
// 	}
// }

// func TestRouteNotFound(t *testing.T) {
// 	router := mux.NewServeMux()

// 	router.Get("/get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

// 	w := httptest.NewRecorder()
// 	r, _ := http.NewRequest(http.MethodGet, "/i/do/not/exist", nil)

// 	router.ServeHTTP(w, r)

// 	if w.Code != http.StatusNotFound {
// 		t.Fatalf("expected not found. got %v", w.Code)
// 	}

// 	w = httptest.NewRecorder()
// 	r, _ = http.NewRequest(http.MethodGet, "/", nil)

// 	router.ServeHTTP(w, r)

// 	if w.Code != http.StatusNotFound {
// 		t.Fatalf("expected not found. got %v", w.Code)
// 	}

// 	w = httptest.NewRecorder()
// 	r, _ = http.NewRequest(http.MethodPost, "/get", nil)

// 	router.ServeHTTP(w, r)

// 	if w.Code != http.StatusNotFound {
// 		t.Fatalf("expected not found. got %v", w.Code)
// 	}
// }

// func TestNestedRoutes(t *testing.T) {
// 	router := mux.NewServeMux()

// 	var users, gopherGet, gopherDelete bool
// 	router.Route("/users", func(r mux.Router) {
// 		r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			users = true
// 		}))
// 		r.Route("/:name", func(r mux.Router) {
// 			r.Get("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				gopherGet = true
// 				want := mux.Params{{"name", "gopher"}}
// 				got := mux.ParamsFromRequest(r)
// 				if !reflect.DeepEqual(want, got) {
// 					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 				}
// 			}))
// 			r.Delete("/delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				gopherDelete = true
// 				want := mux.Params{{"name", "gopher"}}
// 				got := mux.ParamsFromRequest(r)
// 				if !reflect.DeepEqual(want, got) {
// 					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 				}
// 			}))
// 		})
// 	})

// 	w := httptest.NewRecorder()
// 	r, _ := http.NewRequest(http.MethodGet, "/users", nil)
// 	router.ServeHTTP(w, r)
// 	if w.Code != http.StatusNotFound {
// 		t.Fatalf("expected not found. got %v", w.Code)
// 	}

// 	w = httptest.NewRecorder()
// 	r, _ = http.NewRequest(http.MethodGet, "/users/", nil)
// 	router.ServeHTTP(w, r)
// 	if !users {
// 		t.Fatal("routing to /users/ failed")
// 	}

// 	w = httptest.NewRecorder()
// 	r, _ = http.NewRequest(http.MethodGet, "/users/gopher", nil)
// 	router.ServeHTTP(w, r)
// 	if !gopherGet {
// 		t.Fatal("routing to GET /users/gopher failed")
// 	}

// 	w = httptest.NewRecorder()
// 	r, _ = http.NewRequest(http.MethodDelete, "/users/gopher/delete", nil)
// 	router.ServeHTTP(w, r)
// 	if !gopherDelete {
// 		t.Fatal("routing to DELETE /users/gopher/delete failed")
// 	}
// }

// func TestParams(t *testing.T) {
// 	tests := []struct {
// 		p        mux.Params
// 		key, val string
// 	}{
// 		{
// 			p:   mux.Params{},
// 			key: "key",
// 			val: "",
// 		},
// 		{
// 			p:   nil,
// 			key: "key",
// 			val: "",
// 		},
// 		{
// 			p:   mux.Params{{"name", "gopher"}, {"version", "2"}},
// 			key: "name",
// 			val: "gopher",
// 		},
// 	}

// 	for _, tt := range tests {
// 		val, _ := tt.p.Get(tt.key)
// 		if val != tt.val {
// 			t.Fatalf("values do not match. got %s exp %s", val, tt.val)
// 		}
// 	}
// }

// func TestWalk(t *testing.T) {
// 	router := mux.NewServeMux()
// 	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

// 	router.Route("/users", func(r mux.Router) {
// 		r.Get("/", h)
// 		r.Route("/:name", func(r mux.Router) {
// 			r.Get("", h)
// 			r.Delete("/delete", h)
// 			r.Post("/test/*all", h)
// 		})
// 	})

// 	type tuple struct {
// 		method, path string
// 	}
// 	routes := make(map[tuple]bool)

// 	router.Walk(context.Background(), func(method, path string, handler http.Handler, _ *mux.Options) {
// 		tu := tuple{method: method, path: path}
// 		if _, ok := routes[tu]; ok {
// 			t.Fatalf("route already visited %v", tu)
// 		}
// 		routes[tu] = true
// 	})

// 	expected := map[tuple]bool{
// 		{method: "GET", path: "/users/"}:                 true,
// 		{method: "GET", path: "/users/:name"}:            true,
// 		{method: "DELETE", path: "/users/:name/delete"}:  true,
// 		{method: "POST", path: "/users/:name/test/*all"}: true,
// 	}
// 	if !reflect.DeepEqual(expected, routes) {
// 		t.Fatalf("got not expected routes. got %v exp %v", routes, expected)
// 	}
// }

// func TestWalkStop(t *testing.T) {
// 	router := mux.NewServeMux()
// 	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

// 	router.Route("/users", func(r mux.Router) {
// 		r.Get("/", h)
// 		r.Route("/:name", func(r mux.Router) {
// 			r.Get("", h)
// 			r.Delete("/delete", h)
// 			r.Post("/test/*all", h)
// 		})
// 	})

// 	type tuple struct {
// 		method, path string
// 	}
// 	routes := make(map[tuple]bool)

// 	ctx, cancel := context.WithCancel(context.Background())
// 	router.Walk(ctx, func(method, path string, handler http.Handler, _ *mux.Options) {
// 		tu := tuple{method: method, path: path}
// 		if _, ok := routes[tu]; ok {
// 			t.Fatalf("route already visited %v", tu)
// 		}
// 		routes[tu] = true
// 		cancel()
// 	})

// 	expected := map[tuple]bool{
// 		{method: "GET", path: "/users/"}: true,
// 	}
// 	if !reflect.DeepEqual(expected, routes) {
// 		t.Fatalf("got not expected routes. got %v exp %v", routes, expected)
// 	}
// }

// func TestUnprotected(t *testing.T) {
// 	var auth, hit bool

// 	authMid := func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			auth = true
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// 	factory := func(o *mux.Options) (m []middlewares.Middleware) {
// 		if !o.Unprotected {
// 			m = append(m, authMid)
// 		}
// 		return
// 	}

// 	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(factory)

// 	router.Route("/users", func(r mux.Router) {
// 		r.Get("/", nop)
// 		r.Get("/me", nop)
// 		r.Post("/change-password", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			hit = true
// 		}), mux.Unprotected())
// 	})

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/users/", nil)
// 	router.ServeHTTP(w, r)
// 	if !auth {
// 		t.Fatal("/users/ should be authenticated")
// 	}

// 	auth = false
// 	w = new(mockResponseWriter)
// 	r, _ = http.NewRequest(http.MethodPost, "/users/change-password", nil)
// 	router.ServeHTTP(w, r)
// 	if !hit {
// 		t.Fatal("/users/change-password not hit")
// 	}
// 	if auth {
// 		t.Fatal("/users/change-password is unprotected")
// 	}
// }

// func TestOptionsRecursive(t *testing.T) {
// 	var auth bool
// 	authMid := func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			auth = true
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// 	factory := func(o *mux.Options) (m []middlewares.Middleware) {
// 		if !o.Unprotected {
// 			m = append(m, authMid)
// 		}
// 		return
// 	}

// 	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(factory)

// 	router.Route("/users", func(r mux.Router) {
// 		r.Get("/", nop)
// 		r.Get("/me", nop)
// 		r.Post("/change-password", nop)
// 	}, mux.Unprotected())

// 	router.Get("/users/other-unprotected", nop)

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/users/", nil)
// 	router.ServeHTTP(w, r)
// 	if auth {
// 		t.Fatal("/users/ is unprotected")
// 	}

// 	auth = false
// 	w = new(mockResponseWriter)
// 	r, _ = http.NewRequest(http.MethodPost, "/users/change-password", nil)
// 	router.ServeHTTP(w, r)
// 	if auth {
// 		t.Fatal("/users/change-password is unprotected")
// 	}

// 	auth = false
// 	w = new(mockResponseWriter)
// 	r, _ = http.NewRequest(http.MethodGet, "/users/other-unprotected", nil)
// 	router.ServeHTTP(w, r)
// 	if auth {
// 		t.Fatal("/users/other-unprotected is unprotected")
// 	}
// }

// func TestUnprotectedRoutes(t *testing.T) {
// 	var auth bool
// 	authMid := func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			auth = true
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// 	factory := func(o *mux.Options) (m []middlewares.Middleware) {
// 		if !o.Unprotected {
// 			m = append(m, authMid)
// 		}
// 		return
// 	}

// 	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(factory)

// 	router.Route("/unprotected", func(r mux.Router) {
// 		r.Get("/users", nop)
// 	}, mux.Unprotected())

// 	router.Route("/protected", func(r mux.Router) {
// 		r.Post("/change-password", nop)
// 	})

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/unprotected/users", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, auth, false)

// 	auth = false
// 	r, _ = http.NewRequest(http.MethodPost, "/protected/change-password", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, auth, true)
// }

// func TestUnprotectedRoutesReversed(t *testing.T) {
// 	var auth bool
// 	authMid := func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			auth = true
// 			next.ServeHTTP(w, r)
// 		})
// 	}
// 	factory := func(o *mux.Options) (m []middlewares.Middleware) {
// 		if !o.Unprotected {
// 			m = append(m, authMid)
// 		}
// 		return
// 	}

// 	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(factory)

// 	router.Route("/protected", func(r mux.Router) {
// 		r.Post("/change-password", nop)
// 	})

// 	router.Route("/unprotected", func(r mux.Router) {
// 		r.Get("/users", nop)
// 	}, mux.Unprotected())

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/unprotected/users", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, auth, false)

// 	auth = false
// 	r, _ = http.NewRequest(http.MethodPost, "/protected/change-password", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, auth, true)
// }

// func TestParamsInMiddleware(t *testing.T) {
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(func(o *mux.Options) []middlewares.Middleware {
// 		return o.Middlewares
// 	})

// 	var hit, middleware bool
// 	router.Route("/path/:key", func(r mux.Router) {
// 		r.Get("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			hit = true
// 			want := mux.Params{{"key", "value"}}
// 			got := mux.ParamsFromRequest(r)
// 			if !reflect.DeepEqual(want, got) {
// 				t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 			}
// 		}), mux.WithMiddleware(func(next http.Handler) http.Handler {
// 			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				middleware = true
// 				want := mux.Params{{"key", "value"}}
// 				got := mux.ParamsFromRequest(r)
// 				if !reflect.DeepEqual(want, got) {
// 					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 				}
// 				next.ServeHTTP(w, r)
// 			})
// 		}))
// 	})

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/path/value", nil)
// 	router.ServeHTTP(w, r)

// 	if !hit {
// 		t.Fatal("routing failed")
// 	}
// 	if !middleware {
// 		t.Fatal("middleware call failed")
// 	}
// }

// func TestWalkInerithedOptions(t *testing.T) {
// 	router := mux.NewServeMux()

// 	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
// 	router.Route("/inherit", func(r mux.Router) {
// 		r.Get("/unprotected", h)
// 		r.Route("/deep", func(r mux.Router) {
// 			r.Post("/unprotected", h)
// 		})
// 	}, mux.Unprotected())
// 	router.Get("/inherit/other", h)

// 	type tuple struct {
// 		method, path string
// 		opts         *mux.Options
// 	}
// 	routes := []tuple{}
// 	router.Walk(context.Background(), func(method, path string, handler http.Handler, opts *mux.Options) {
// 		routes = append(routes, tuple{method, path, opts})
// 	})

// 	expected := []tuple{
// 		{method: "GET", path: "/inherit/unprotected", opts: &mux.Options{Unprotected: true}},
// 		{method: "POST", path: "/inherit/deep/unprotected", opts: &mux.Options{Unprotected: true}},
// 		{method: "GET", path: "/inherit/other", opts: &mux.Options{Unprotected: true}},
// 	}
// 	if !reflect.DeepEqual(expected, routes) {
// 		t.Fatalf("got not expected routes.\ngot %+v\nexp %+v", render.AsCode(routes), render.AsCode(expected))
// 	}
// }

// func TestDefaultRoute(t *testing.T) {
// 	router := mux.NewServeMux()

// 	var hit10, hitNumber bool
// 	router.Get("/var/10", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hit10 = true
// 	}))
// 	router.Get("/var/:number", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hitNumber = true
// 		got := mux.ParamsFromRequest(r)
// 		want := mux.Params{{"number", "100"}}
// 		if !reflect.DeepEqual(want, got) {
// 			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
// 		}
// 	}))

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/var/10", nil)
// 	router.ServeHTTP(w, r)

// 	assert.Equal(t, hit10, true)
// 	assert.Equal(t, hitNumber, false)

// 	hit10, hitNumber = false, false

// 	r, _ = http.NewRequest(http.MethodGet, "/var/100", nil)
// 	router.ServeHTTP(w, r)

// 	assert.Equal(t, hit10, false)
// 	assert.Equal(t, hitNumber, true)
// }

// func TestMountHandler(t *testing.T) {
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(func(o *mux.Options) []middlewares.Middleware {
// 		return o.Middlewares
// 	})

// 	var hit bool
// 	router.Mount("/mounted", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hit = true
// 		if r.URL.Path != "/some/path/rooted" {
// 			t.Fatalf("path expected to be /some/path/rooted. got %s", r.URL.Path)
// 		}
// 	}))

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodHead, "/mounted/some/path/rooted", nil)
// 	router.ServeHTTP(w, r)

// 	assert.Equal(t, hit, true)
// }

// func TestMountRouter(t *testing.T) {
// 	router := mux.NewServeMux()
// 	router.SetMiddlewaresFactory(func(o *mux.Options) []middlewares.Middleware {
// 		return o.Middlewares
// 	})

// 	var hit bool
// 	sub := mux.NewServeMux()
// 	sub.Route("/some", func(r mux.Router) {
// 		r.Head("/path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			hit = true
// 			if r.URL.Path != "/some/path" {
// 				t.Fatalf("path expected to be //some/path. got %s", r.URL.Path)
// 			}
// 		}))
// 	})

// 	router.Mount("/mounted", sub)

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodHead, "/mounted/some/path", nil)
// 	router.ServeHTTP(w, r)

// 	assert.Equal(t, hit, true)
// }

// func TestCatchAll(t *testing.T) {
// 	router := mux.NewServeMux()

// 	var hit bool
// 	router.Handle("/test/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		hit = true
// 	}))

// 	w := new(mockResponseWriter)
// 	r, _ := http.NewRequest(http.MethodGet, "/test/some/deep/path/to/test/if/this/is/called/i/hope/yes", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, hit, true)

// 	hit = false
// 	r, _ = http.NewRequest(http.MethodGet, "/test/", nil)
// 	router.ServeHTTP(w, r)
// 	assert.Equal(t, hit, true)
// }
