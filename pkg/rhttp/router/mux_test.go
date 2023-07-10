package mux_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	mux "github.com/cs3org/reva/pkg/rhttp/router"
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
	mux := mux.NewServeMux()

	var get, post, head, put, connect, delete, options bool
	mux.Get("/get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		get = true
	}))

	mux.Post("/post", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		post = true
	}))

	mux.Head("/head", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		head = true
	}))

	mux.Put("/put", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		put = true
	}))

	mux.Connect("/connect", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connect = true
	}))

	mux.Options("/options", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		options = true
	}))

	mux.Delete("/delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delete = true
	}))

	w := new(mockResponseWriter)

	r, _ := http.NewRequest(http.MethodGet, "/get", nil)
	mux.ServeHTTP(w, r)
	if !get {
		t.Fatal("routing GET failed")
	}

	r, _ = http.NewRequest(http.MethodPost, "/post", nil)
	mux.ServeHTTP(w, r)
	if !post {
		t.Fatal("routing POST failed")
	}

	r, _ = http.NewRequest(http.MethodHead, "/head", nil)
	mux.ServeHTTP(w, r)
	if !head {
		t.Fatal("routing GET failed")
	}

	r, _ = http.NewRequest(http.MethodPut, "/put", nil)
	mux.ServeHTTP(w, r)
	if !put {
		t.Fatal("routing PUT failed")
	}

	r, _ = http.NewRequest(http.MethodConnect, "/connect", nil)
	mux.ServeHTTP(w, r)
	if !connect {
		t.Fatal("routing CONNECT failed")
	}

	r, _ = http.NewRequest(http.MethodOptions, "/options", nil)
	mux.ServeHTTP(w, r)
	if !options {
		t.Fatal("routing OPTIONS failed")
	}

	r, _ = http.NewRequest(http.MethodDelete, "/delete", nil)
	mux.ServeHTTP(w, r)
	if !delete {
		t.Fatal("routing DELETE failed")
	}
}

func TestParamsResolved(t *testing.T) {
	router := mux.NewServeMux()

	var hit bool
	router.Get("/user/:name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		want := mux.Params{"name": "gopher"}
		got := mux.ParamsFromRequest(r)
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
	router.Get("/path/*path", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		want := mux.Params{"path": "this/is/a/path"}
		got := mux.ParamsFromRequest(r)
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

	router.Get("/get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

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
		r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			users = true
		}))
		r.Route("/:name", func(r mux.Router) {
			r.Get("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gopherGet = true
				want := mux.Params{"name": "gopher"}
				got := mux.ParamsFromRequest(r)
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("wrong wildcard values: want %v got %v", want, got)
				}
			}))
			r.Delete("/delete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gopherDelete = true
				want := mux.Params{"name": "gopher"}
				got := mux.ParamsFromRequest(r)
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
			p:   mux.Params{"name": "gopher", "version": "2"},
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
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

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

	router.Walk(context.Background(), func(method, path string, handler http.Handler, _ *mux.Options) {
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

func TestUnprotected(t *testing.T) {
	var auth, hit bool

	authMid := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth = true
			next.ServeHTTP(w, r)
		})
	}
	factory := func(o *mux.Options) (m []mux.Middleware) {
		if !o.Unprotected {
			m = append(m, authMid)
		}
		return
	}

	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	router := mux.NewServeMux()
	router.SetMiddlewaresFactory(factory)

	router.Route("/users", func(r mux.Router) {
		r.Get("/", nop)
		r.Get("/me", nop)
		r.Post("/change-password", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
		}), mux.Unprotected())
	})

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodGet, "/users/", nil)
	router.ServeHTTP(w, r)
	if !auth {
		t.Fatal("/users/ should be authenticated")
	}

	auth = false
	w = new(mockResponseWriter)
	r, _ = http.NewRequest(http.MethodPost, "/users/change-password", nil)
	router.ServeHTTP(w, r)
	if !hit {
		t.Fatal("/users/change-password not hit")
	}
	if auth {
		t.Fatal("/users/change-password is unprotected")
	}
}

func TestOptionsRecursive(t *testing.T) {
	var auth bool
	authMid := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth = true
			next.ServeHTTP(w, r)
		})
	}
	factory := func(o *mux.Options) (m []mux.Middleware) {
		if !o.Unprotected {
			m = append(m, authMid)
		}
		return
	}

	nop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	router := mux.NewServeMux()
	router.SetMiddlewaresFactory(factory)

	router.Route("/users", func(r mux.Router) {
		r.Get("/", nop)
		r.Get("/me", nop)
		r.Post("/change-password", nop)
	}, mux.Unprotected())

	router.Get("/users/other-unprotected", nop)

	w := new(mockResponseWriter)
	r, _ := http.NewRequest(http.MethodGet, "/users/", nil)
	router.ServeHTTP(w, r)
	if auth {
		t.Fatal("/users/ is unprotected")
	}

	auth = false
	w = new(mockResponseWriter)
	r, _ = http.NewRequest(http.MethodPost, "/users/change-password", nil)
	router.ServeHTTP(w, r)
	if auth {
		t.Fatal("/users/change-password is unprotected")
	}

	auth = false
	w = new(mockResponseWriter)
	r, _ = http.NewRequest(http.MethodGet, "/users/other-unprotected", nil)
	router.ServeHTTP(w, r)
	if auth {
		t.Fatal("/users/other-unprotected is unprotected")
	}
}
