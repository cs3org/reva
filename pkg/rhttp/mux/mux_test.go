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

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/cs3org/reva/pkg/rhttp/mux"
	"gotest.tools/assert"
)

type mockResponseWriter struct{}

func (m *mockResponseWriter) Header() (h http.Header) {
	return http.Header{}
}

func (m *mockResponseWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockResponseWriter) WriteString(s string) (n int, err error) {
	return len(s), nil
}

func (m *mockResponseWriter) WriteHeader(int) {}

func TestRouterAPIs(t *testing.T) {
	m := mux.NewServeMux()

	var get, post, head, put, connect, delete, options, patch bool
	m.Get("/get", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		get = true
	}))

	m.Post("/post", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		post = true
	}))

	m.Head("/head", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		head = true
	}))

	m.Patch("/patch", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		patch = true
	}))

	m.Put("/put", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		put = true
	}))

	m.Connect("/connect", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		connect = true
	}))

	m.Options("/options", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		options = true
	}))

	m.Delete("/delete", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		delete = true
	}))

	w := new(mockResponseWriter)

	r, _ := http.NewRequest(http.MethodGet, "/get", nil)
	m.ServeHTTP(w, r)
	if !get {
		t.Fatal("routing GET failed")
	}

	r, _ = http.NewRequest(http.MethodPost, "/post", nil)
	m.ServeHTTP(w, r)
	if !post {
		t.Fatal("routing POST failed")
	}

	r, _ = http.NewRequest(http.MethodHead, "/head", nil)
	m.ServeHTTP(w, r)
	if !head {
		t.Fatal("routing GET failed")
	}

	r, _ = http.NewRequest(http.MethodPut, "/put", nil)
	m.ServeHTTP(w, r)
	if !put {
		t.Fatal("routing PUT failed")
	}

	r, _ = http.NewRequest(http.MethodConnect, "/connect", nil)
	m.ServeHTTP(w, r)
	if !connect {
		t.Fatal("routing CONNECT failed")
	}

	r, _ = http.NewRequest(http.MethodOptions, "/options", nil)
	m.ServeHTTP(w, r)
	if !options {
		t.Fatal("routing OPTIONS failed")
	}

	r, _ = http.NewRequest(http.MethodDelete, "/delete", nil)
	m.ServeHTTP(w, r)
	if !delete {
		t.Fatal("routing DELETE failed")
	}

	r, _ = http.NewRequest(http.MethodPatch, "/patch", nil)
	m.ServeHTTP(w, r)
	if !patch {
		t.Fatal("routing PATCH failed")
	}
}

func TestParamsResolved(t *testing.T) {
	router := mux.NewServeMux()

	var hit bool
	router.Get("/user/:name", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
		hit = true
		want := mux.Params{{"name", "gopher"}}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
		}
	}))

	w := new(mockResponseWriter)

	r, _ := http.NewRequest(http.MethodGet, "/user/gopher", nil)
	router.ServeHTTP(w, r)

	if !hit {
		t.Fatal("routing failed")
	}
}

func TestParamsCatchAll(t *testing.T) {
	router := mux.NewServeMux()

	var hit bool
	router.Get("/path/*path", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
		hit = true
		want := mux.Params{{"path", "this/is/a/path"}}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
		}
	}))

	w := new(mockResponseWriter)

	r, _ := http.NewRequest(http.MethodGet, "/path/this/is/a/path", nil)
	router.ServeHTTP(w, r)

	if !hit {
		t.Fatal("routing failed")
	}
}

func TestRouteNotFound(t *testing.T) {
	router := mux.NewServeMux()

	router.Get("/get", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/i/do/not/exist", nil)

	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected not found. got %v", w.Code)
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodGet, "/", nil)

	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected not found. got %v", w.Code)
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodPost, "/get", nil)

	router.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected not found. got %v", w.Code)
	}
}

func TestNestedRoutes(t *testing.T) {
	router := mux.NewServeMux()

	var users, gopherGet, gopherDelete bool
	router.Route("/users", func(r mux.Router) {
		r.Get("/", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
			users = true
		}))
		r.Route("/:name", func(r mux.Router) {
			r.Get("", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
				gopherGet = true
				want := mux.Params{{"name", "gopher"}}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
				}
			}))
			r.Delete("/delete", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
				gopherDelete = true
				want := mux.Params{{"name", "gopher"}}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
				}
			}))
		})
	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/users", nil)
	router.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected not found. got %v", w.Code)
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodGet, "/users/", nil)
	router.ServeHTTP(w, r)
	if !users {
		t.Fatal("routing to /users/ failed")
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodGet, "/users/gopher", nil)
	router.ServeHTTP(w, r)
	if !gopherGet {
		t.Fatal("routing to GET /users/gopher failed")
	}

	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodDelete, "/users/gopher/delete", nil)
	router.ServeHTTP(w, r)
	if !gopherDelete {
		t.Fatal("routing to DELETE /users/gopher/delete failed")
	}
}

func TestParams(t *testing.T) {
	tests := []struct {
		p        mux.Params
		key, val string
	}{
		{
			p:   mux.Params{},
			key: "key",
			val: "",
		},
		{
			p:   nil,
			key: "key",
			val: "",
		},
		{
			p:   mux.Params{{"name", "gopher"}, {"version", "2"}},
			key: "name",
			val: "gopher",
		},
	}

	for _, tt := range tests {
		val, _ := tt.p.Get(tt.key)
		if val != tt.val {
			t.Fatalf("values do not match. got %s exp %s", val, tt.val)
		}
	}
}

func TestWalk(t *testing.T) {
	router := mux.NewServeMux()
	h := mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {})

	router.Route("/users", func(r mux.Router) {
		r.Get("/", h)
		r.Route("/:name", func(r mux.Router) {
			r.Get("", h)
			r.Delete("/delete", h)
			r.Post("/test/*all", h)
		})
	})

	type tuple struct {
		method, path string
	}
	routes := make(map[tuple]bool)

	router.Walk(context.Background(), func(method, path string, handler mux.Handler) {
		tu := tuple{method: method, path: path}
		if _, ok := routes[tu]; ok {
			t.Fatalf("route already visited %v", tu)
		}
		routes[tu] = true
	})

	expected := map[tuple]bool{
		{method: "GET", path: "/users/"}:                 true,
		{method: "GET", path: "/users/:name"}:            true,
		{method: "DELETE", path: "/users/:name/delete"}:  true,
		{method: "POST", path: "/users/:name/test/*all"}: true,
	}
	if !reflect.DeepEqual(expected, routes) {
		t.Fatalf("got not expected routes. got %v exp %v", routes, expected)
	}
}

func TestWalkStop(t *testing.T) {
	router := mux.NewServeMux()
	h := mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {})

	router.Route("/users", func(r mux.Router) {
		r.Get("/", h)
		r.Route("/:name", func(r mux.Router) {
			r.Get("", h)
			r.Delete("/delete", h)
			r.Post("/test/*all", h)
		})
	})

	type tuple struct {
		method, path string
	}
	routes := make(map[tuple]bool)

	ctx, cancel := context.WithCancel(context.Background())
	router.Walk(ctx, func(method, path string, handler mux.Handler) {
		tu := tuple{method: method, path: path}
		if _, ok := routes[tu]; ok {
			t.Fatalf("route already visited %v", tu)
		}
		routes[tu] = true
		cancel()
	})

	expected := map[tuple]bool{
		{method: "GET", path: "/users/"}: true,
	}
	if !reflect.DeepEqual(expected, routes) {
		t.Fatalf("got not expected routes. got %v exp %v", routes, expected)
	}
}

func TestParamsInMiddleware(t *testing.T) {
	router := mux.NewServeMux()

	var hit, middleware bool
	router.Route("/path/:key", func(r mux.Router) {
		r.Use(func(next mux.Handler) mux.Handler {
			return mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
				middleware = true
				want := mux.Params{{"key", "value"}}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
				}
				next.ServeHTTP(w, r, got)
			})
		})
		r.Get("", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
			hit = true
			want := mux.Params{{"key", "value"}}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("wrong wildcard values: want %v got %v", want, got)
			}
		}))
	})

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodGet, "/path/value", nil)
	router.ServeHTTP(w, r)

	if !hit {
		t.Fatal("routing failed")
	}
	if !middleware {
		t.Fatal("middleware call failed")
	}
}

func TestDefaultRoute(t *testing.T) {
	router := mux.NewServeMux()

	var hit10, hitNumber bool
	router.Get("/var/10", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		hit10 = true
	}))
	router.Get("/var/:number", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, got mux.Params) {
		hitNumber = true
		want := mux.Params{{"number", "100"}}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("wrong wildcard values: want %v got %v", want, got)
		}
	}))

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodGet, "/var/10", nil)
	router.ServeHTTP(w, r)

	assert.Equal(t, hit10, true)
	assert.Equal(t, hitNumber, false)

	hit10, hitNumber = false, false

	r, _ = http.NewRequest(http.MethodGet, "/var/100", nil)
	router.ServeHTTP(w, r)

	assert.Equal(t, hit10, false)
	assert.Equal(t, hitNumber, true)
}

func TestMountHandler(t *testing.T) {
	router := mux.NewServeMux()

	var hit bool
	router.Mount("/mounted", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		hit = true
		if r.URL.Path != "/some/path/rooted" {
			t.Fatalf("path expected to be /some/path/rooted. got %s", r.URL.Path)
		}
	}))

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodHead, "/mounted/some/path/rooted", nil)
	router.ServeHTTP(w, r)

	assert.Equal(t, hit, true)
}

func TestCatchAll(t *testing.T) {
	router := mux.NewServeMux()

	var hit bool
	router.Handle("/test/*", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		hit = true
	}))

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodGet, "/test/some/deep/path/to/test/if/this/is/called/i/hope/yes", nil)
	router.ServeHTTP(w, r)
	assert.Equal(t, hit, true)

	hit = false
	r, _ = http.NewRequest(http.MethodGet, "/test/", nil)
	router.ServeHTTP(w, r)
	assert.Equal(t, hit, true)
}
