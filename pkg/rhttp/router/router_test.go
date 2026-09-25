// Copyright 2018-2026 CERN
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

package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func echo(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, tag)
	}
}

func serve(r *Router, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestMethodRouting(t *testing.T) {
	r := New().Service("ocs")
	r.Get("/shares", echo("list"))
	r.Post("/shares", echo("create"))
	r.Delete("/shares/{id}", echo("remove"))

	tests := map[string]struct {
		method string
		target string
		code   int
		body   string
	}{
		"get":            {http.MethodGet, "/shares", http.StatusOK, "list"},
		"post":           {http.MethodPost, "/shares", http.StatusOK, "create"},
		"wildcard":       {http.MethodDelete, "/shares/42", http.StatusOK, "remove"},
		"wrong method":   {http.MethodPut, "/shares", http.StatusMethodNotAllowed, ""},
		"unknown path":   {http.MethodGet, "/nope", http.StatusNotFound, ""},
		"exact not tree": {http.MethodGet, "/shares/42", http.StatusMethodNotAllowed, ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := serve(r, tt.method, tt.target)
			if rec.Code != tt.code {
				t.Errorf("got status %d, expected %d", rec.Code, tt.code)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Errorf("got body %q, expected %q", rec.Body.String(), tt.body)
			}
		})
	}
}

func TestPathValue(t *testing.T) {
	r := New()
	r.Get("/shares/{id}", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, req.PathValue("id"))
	})

	if got := serve(r, http.MethodGet, "/shares/abc").Body.String(); got != "abc" {
		t.Errorf("got %q, expected %q", got, "abc")
	}
}

func TestCustomMethod(t *testing.T) {
	r := New()
	r.Handle("PROPFIND", "/dav/{path...}", echo("propfind"))

	rec := serve(r, "PROPFIND", "/dav/files/einstein/a.txt")
	if rec.Code != http.StatusOK || rec.Body.String() != "propfind" {
		t.Errorf("got %d %q, expected 200 %q", rec.Code, rec.Body.String(), "propfind")
	}
}

func TestGroupPrefix(t *testing.T) {
	r := New()
	r.Group("/ocs/v1.php", func(r *Router) {
		r.Get("/config", echo("config"))
		r.Group("/cloud", func(r *Router) {
			r.Get("/", echo("cloud"))
			r.Get("/capabilities", echo("caps"))
		})
	})

	tests := map[string]struct {
		target string
		body   string
	}{
		"nested leaf":  {"/ocs/v1.php/config", "config"},
		"group root":   {"/ocs/v1.php/cloud", "cloud"},
		"nested twice": {"/ocs/v1.php/cloud/capabilities", "caps"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := serve(r, http.MethodGet, tt.target).Body.String(); got != tt.body {
				t.Errorf("got %q, expected %q", got, tt.body)
			}
		})
	}
}

func TestGroupMiddleware(t *testing.T) {
	stamp := func(tag string) Middleware {
		return func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tag)
				h.ServeHTTP(w, r)
			})
		}
	}

	r := New()
	r.Get("/outside", echo("!"))
	r.Group("/api", func(r *Router) {
		r.Get("/plain", echo("!"))
		r.Group("/deep", func(r *Router) {
			r.Get("/", echo("!"))
		}, stamp("b"))
	}, stamp("a"))

	tests := map[string]struct {
		target string
		body   string
	}{
		"no middleware":      {"/outside", "!"},
		"group middleware":   {"/api/plain", "a!"},
		"nested accumulates": {"/api/deep", "ab!"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := serve(r, http.MethodGet, tt.target).Body.String(); got != tt.body {
				t.Errorf("got %q, expected %q", got, tt.body)
			}
		})
	}
}

// A mount owns its subtree and sees the request exactly as it arrived: the
// path is neither canonicalized nor decoded, which is why WebDAV and foreign
// handlers can use it and patterns cannot.
func TestMountLeavesPathUntouched(t *testing.T) {
	r := New()
	r.Mount("/dav", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, req.URL.Path)
	}))

	tests := map[string]struct {
		target string
		body   string
	}{
		"plain":         {"/dav/files/a.txt", "/dav/files/a.txt"},
		"double slash":  {"/dav/files//a.txt", "/dav/files//a.txt"},
		"dot segments":  {"/dav/files/../a.txt", "/dav/files/../a.txt"},
		"mount itself":  {"/dav", "/dav"},
		"encoded slash": {"/dav/files/a%2Fb.txt", "/dav/files/a/b.txt"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := serve(r, http.MethodGet, tt.target)
			if rec.Code != http.StatusOK {
				t.Fatalf("got status %d, expected 200", rec.Code)
			}
			if rec.Body.String() != tt.body {
				t.Errorf("got %q, expected %q", rec.Body.String(), tt.body)
			}
		})
	}
}

func TestMountDoesNotSwallowSiblings(t *testing.T) {
	r := New()
	r.Mount("/dav", echo("dav"))
	r.Get("/davfoo", echo("sibling"))

	if got := serve(r, http.MethodGet, "/davfoo").Body.String(); got != "sibling" {
		t.Errorf("got %q, expected %q", got, "sibling")
	}
}

func TestMountLongestPrefixWins(t *testing.T) {
	r := New()
	r.Mount("/dav", echo("short"))
	r.Mount("/dav/files", echo("long"))

	if got := serve(r, http.MethodGet, "/dav/files/x").Body.String(); got != "long" {
		t.Errorf("got %q, expected %q", got, "long")
	}
	if got := serve(r, http.MethodGet, "/dav/other").Body.String(); got != "short" {
		t.Errorf("got %q, expected %q", got, "short")
	}
}

func TestRoutesRecorded(t *testing.T) {
	r := New()
	r.Service("ocs").Get("/ocs/config", echo("!"))
	r.Service("ocm").Post("/ocm/shares", echo("!"), Unprotected())
	r.Service("ocdav").Mount("/dav", echo("!"))

	expected := []Route{
		{Owner: "ocs", Method: http.MethodGet, Pattern: "/ocs/config"},
		{Owner: "ocm", Method: http.MethodPost, Pattern: "/ocm/shares", Unprotected: true},
		{Owner: "ocdav", Pattern: "/dav", Subtree: true},
	}
	if got := r.Routes(); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %+v, expected %+v", got, expected)
	}
}

func TestUnprotected(t *testing.T) {
	r := New()
	r.Get("/ocs/config", echo("!"))
	r.Get("/status.php", echo("!"), Unprotected())
	r.Mount("/dav/public-files", echo("!"), Unprotected())

	expected := []string{"/status.php", "/dav/public-files"}
	if got := r.Unprotected(); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, expected %v", got, expected)
	}
}

func TestMatch(t *testing.T) {
	r := New()
	r.Service("ocs").Get("/ocs/config", echo("!"))
	r.Service("ocm").Post("/ocm/shares", echo("!"), Unprotected())
	r.Service("ocdav").Mount("/dav", echo("!"), Unprotected())

	tests := map[string]struct {
		method string
		target string
		found  bool
		owner  string
		unprot bool
	}{
		"pattern":         {http.MethodGet, "/ocs/config", true, "ocs", false},
		"unprotected":     {http.MethodPost, "/ocm/shares", true, "ocm", true},
		"mount":           {http.MethodGet, "/dav/files/a.txt", true, "ocdav", true},
		"unknown":         {http.MethodGet, "/nope", false, "", false},
		"method mismatch": {http.MethodPut, "/ocs/config", false, "", false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rt, ok := r.Match(httptest.NewRequest(tt.method, tt.target, nil))
			if ok != tt.found {
				t.Fatalf("got found %v, expected %v", ok, tt.found)
			}
			if !ok {
				return
			}
			if rt.Owner != tt.owner {
				t.Errorf("got owner %q, expected %q", rt.Owner, tt.owner)
			}
			if rt.Unprotected != tt.unprot {
				t.Errorf("got unprotected %v, expected %v", rt.Unprotected, tt.unprot)
			}
		})
	}
}

// A request ServeMux answers itself, by redirecting a non-canonical path, has
// no declared route: callers must read that as "not matched", so that such a
// request is never treated as unprotected.
func TestMatchCanonicalizedIsNotARoute(t *testing.T) {
	r := New()
	r.Get("/ocs/config", echo("!"), Unprotected())

	if _, ok := r.Match(httptest.NewRequest(http.MethodGet, "/ocs//config", nil)); ok {
		t.Error("expected no match for a non-canonical path")
	}
}

func TestDuplicateRoutePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic on a redeclared route")
		}
	}()

	r := New()
	r.Service("first").Get("/shares", echo("!"))
	r.Service("second").Get("/shares", echo("!"))
}

func TestJoin(t *testing.T) {
	tests := map[string]struct {
		prefix   string
		pattern  string
		expected string
	}{
		"no prefix":        {"", "/shares", "/shares"},
		"empty both":       {"", "", "/"},
		"group root":       {"/ocs", "/", "/ocs"},
		"group empty":      {"/ocs", "", "/ocs"},
		"nested":           {"/ocs", "/shares", "/ocs/shares"},
		"prefix slash end": {"/ocs/", "/shares", "/ocs/shares"},
		"wildcard":         {"/ocs", "/shares/{id}", "/ocs/shares/{id}"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := join(tt.prefix, tt.pattern); got != tt.expected {
				t.Errorf("got %q, expected %q", got, tt.expected)
			}
		})
	}
}

// A subtree that names its methods is refused anything else by the router, so
// the handler below it never sees a method it does not serve.
func TestMountMethods(t *testing.T) {
	r := New()
	r.Service("ocdav").Mount("/dav", echo("dav"), Methods("PROPFIND", http.MethodGet))

	tests := map[string]struct {
		method string
		code   int
	}{
		"declared":        {http.MethodGet, http.StatusOK},
		"custom method":   {"PROPFIND", http.StatusOK},
		"not declared":    {http.MethodPost, http.StatusMethodNotAllowed},
		"also undeclared": {"LOCK", http.StatusMethodNotAllowed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := serve(r, tt.method, "/dav/files/a.txt")
			if rec.Code != tt.code {
				t.Errorf("got status %d, expected %d", rec.Code, tt.code)
			}
			if tt.code == http.StatusMethodNotAllowed && rec.Header().Get("Allow") == "" {
				t.Error("expected an Allow header on the refusal")
			}
		})
	}
}

// Naming the methods records the subtree once per method, so the table says
// what it serves rather than only where it lives.
func TestMountMethodsAreRecorded(t *testing.T) {
	r := New()
	r.Service("ocdav").Mount("/dav", echo("!"), Methods(http.MethodGet, "PROPFIND"), Unprotected())

	expected := []Route{
		{Owner: "ocdav", Method: http.MethodGet, Pattern: "/dav", Subtree: true, Unprotected: true},
		{Owner: "ocdav", Method: "PROPFIND", Pattern: "/dav", Subtree: true, Unprotected: true},
	}
	if got := r.Routes(); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %+v, expected %+v", got, expected)
	}

	// The path is still listed once, however many methods it was recorded for.
	if got := r.Unprotected(); !reflect.DeepEqual(got, []string{"/dav"}) {
		t.Errorf("got %v, expected %v", got, []string{"/dav"})
	}
}

// A method the subtree does not serve resolves to no route, so a middleware
// reads it as the stricter case rather than as an exempt one.
func TestMatchMountMethods(t *testing.T) {
	r := New()
	r.Service("ocdav").Mount("/dav", echo("!"), Methods(http.MethodGet), Unprotected())

	if rt, ok := r.Match(httptest.NewRequest(http.MethodGet, "/dav/x", nil)); !ok || !rt.Unprotected {
		t.Errorf("got %+v %v, expected the declared unprotected route", rt, ok)
	}
	if _, ok := r.Match(httptest.NewRequest(http.MethodPost, "/dav/x", nil)); ok {
		t.Error("expected an undeclared method to resolve to no route")
	}
}

// A subtree declares a handler per method, so the router picks the operation
// and the handler below it never switches on the method.
func TestSubtree(t *testing.T) {
	r := New()
	r.Service("ocdav").Subtree("/dav", func(sub *Subtree) {
		sub.Get(echo("get"))
		sub.HandleFunc("PROPFIND", echo("propfind"))
		sub.Delete(echo("delete"))
	})

	tests := map[string]struct {
		method string
		code   int
		body   string
	}{
		"get":           {http.MethodGet, http.StatusOK, "get"},
		"custom method": {"PROPFIND", http.StatusOK, "propfind"},
		"delete":        {http.MethodDelete, http.StatusOK, "delete"},
		"undeclared":    {http.MethodPut, http.StatusMethodNotAllowed, ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := serve(r, tt.method, "/dav/files/a.txt")
			if rec.Code != tt.code {
				t.Fatalf("got status %d, expected %d", rec.Code, tt.code)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Errorf("got %q, expected %q", rec.Body.String(), tt.body)
			}
			if tt.code == http.StatusMethodNotAllowed && rec.Header().Get("Allow") == "" {
				t.Error("expected an Allow header on the refusal")
			}
		})
	}
}

// The path below a subtree reaches its handler untouched, as it does below a
// mount: that is why WebDAV can use one and patterns it cannot.
func TestSubtreeLeavesPathUntouched(t *testing.T) {
	var seen string
	r := New()
	r.Subtree("/dav", func(sub *Subtree) {
		sub.Get(func(_ http.ResponseWriter, req *http.Request) { seen = req.URL.Path })
	})

	for _, target := range []string{"/dav/files/a%2Fb.txt", "/dav/files//double", "/dav/files/x/."} {
		t.Run(target, func(t *testing.T) {
			seen = ""
			serve(r, http.MethodGet, target)
			decoded := strings.ReplaceAll(target, "%2F", "/")
			if seen != decoded {
				t.Errorf("handler saw %q, expected %q", seen, decoded)
			}
		})
	}
}

// Every method a subtree declares is its own route, so the table says what the
// subtree serves rather than only where it lives.
func TestSubtreeRoutesRecorded(t *testing.T) {
	r := New()
	r.Service("ocdav").Subtree("/dav", func(sub *Subtree) {
		sub.HandleFunc("PROPFIND", echo("!"))
		sub.Get(echo("!"))
	}, Unprotected())

	expected := []Route{
		{Owner: "ocdav", Method: "PROPFIND", Pattern: "/dav", Subtree: true, Unprotected: true},
		{Owner: "ocdav", Method: http.MethodGet, Pattern: "/dav", Subtree: true, Unprotected: true},
	}
	if got := r.Routes(); !reflect.DeepEqual(got, expected) {
		t.Errorf("got %+v, expected %+v", got, expected)
	}
}

func TestSubtreeRejectsDuplicateAndEmpty(t *testing.T) {
	t.Run("duplicate method", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic on a redeclared method")
			}
		}()
		New().Subtree("/dav", func(sub *Subtree) {
			sub.Get(echo("!"))
			sub.Get(echo("!"))
		})
	})

	t.Run("no methods", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic on a subtree serving nothing")
			}
		}()
		New().Subtree("/dav", func(*Subtree) {})
	})
}
