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

// Package router is the HTTP routing API reva services use to declare the URLs
// they serve. It wraps the standard library's ServeMux, so patterns use its
// syntax ("/shares/{id}", "/dav/{path...}"), and records every declared route:
// the recorded table is what lets the server derive the unprotected set and
// advertise a service's URL space instead of having each service keep its own
// router and its own copy of that knowledge.
package router

import (
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
)

// Route is a single URL declared by a service. It is serialisable because a
// service advertises its routes in the registry, where a gateway reads them to
// mirror what the service serves.
type Route struct {
	// Owner is the reva service name that declared the route.
	Owner string `json:"owner,omitempty"`
	// Method the route matches. Empty matches any method.
	Method string `json:"method,omitempty"`
	// Pattern is the absolute path pattern, in ServeMux syntax.
	Pattern string `json:"pattern"`
	// Subtree is set for mounted handlers: every path under Pattern is served
	// by the mount, which receives the request path untouched.
	Subtree bool `json:"subtree,omitempty"`
	// Methods, on a subtree, are the methods it serves. Empty means every
	// method. It is not serialised: each method is recorded as its own route.
	Methods []string `json:"-"`
	// Unprotected exempts the route from the authentication middleware.
	Unprotected bool `json:"unprotected,omitempty"`
}

// Option customizes a route at declaration time.
type Option func(*Route)

// Unprotected exempts the route from the authentication middleware. Use it for
// endpoints that authenticate at the protocol layer (OCM ingress, public
// links) or that are public by definition (discovery documents).
func Unprotected() Option {
	return func(r *Route) { r.Unprotected = true }
}

// Methods restricts a mounted subtree to the methods it serves. The subtree is
// then recorded as one route per method, so the table says what it answers,
// and the router refuses anything else with a 405 rather than leaving the
// handler to notice.
//
// It is how a subtree declares its methods without giving up a mount: below a
// mount the path is handed over untouched, which is what WebDAV needs and what
// pattern matching cannot do.
func Methods(m ...string) Option {
	return func(r *Route) { r.Methods = m }
}

// Middleware wraps a handler. Middlewares attached to a Group apply to every
// route declared inside it.
type Middleware func(http.Handler) http.Handler

// Router records the routes a service declares and serves them. The zero value
// is not usable; call New.
//
// A Router value is a view: Service and Group return cheap copies that carry a
// different owner, prefix or middleware chain, all writing into the same
// underlying registration table.
type Router struct {
	mux *http.ServeMux
	reg *registrations

	owner  string
	prefix string
	mw     []Middleware
}

type registrations struct {
	routes    []Route
	byPattern map[string]string // "METHOD PATTERN" -> owner, for conflict reporting
	mounts    []mount
}

type mount struct {
	prefix  string
	handler http.Handler
	// methods the subtree serves, empty meaning all of them.
	methods []string
}

func (m mount) serves(method string) bool {
	if len(m.methods) == 0 {
		return true
	}
	return slices.Contains(m.methods, method)
}

// New returns an empty Router.
func New() *Router {
	return &Router{
		mux: http.NewServeMux(),
		reg: &registrations{byPattern: map[string]string{}},
	}
}

// Service returns a view of the router that records every route declared on it
// as belonging to the named service.
func (r *Router) Service(name string) *Router {
	c := *r
	c.owner = name
	return &c
}

// Use returns a view of the router that wraps every route declared on it with
// mw, on top of any middleware already in effect.
func (r *Router) Use(mw ...Middleware) *Router {
	c := *r
	c.mw = append(append(make([]Middleware, 0, len(r.mw)+len(mw)), r.mw...), mw...)
	return &c
}

// Group declares the routes in fn under prefix, with mw wrapping each of them.
// Groups nest, accumulating both prefix and middlewares.
func (r *Router) Group(prefix string, fn func(*Router), mw ...Middleware) {
	c := *r
	c.prefix = join(r.prefix, prefix)
	if len(mw) > 0 {
		c.mw = append(append(make([]Middleware, 0, len(r.mw)+len(mw)), r.mw...), mw...)
	}
	fn(&c)
}

// Handle declares a route for the given method, empty meaning any method.
func (r *Router) Handle(method, pattern string, h http.Handler, opts ...Option) {
	rt := Route{
		Owner:   r.owner,
		Method:  method,
		Pattern: join(r.prefix, pattern),
	}
	for _, o := range opts {
		o(&rt)
	}

	key := rt.Method + " " + rt.Pattern
	if owner, dup := r.reg.byPattern[key]; dup {
		panic("router: " + rt.Owner + " redeclares " + strings.TrimSpace(key) + ", already declared by " + owner)
	}
	r.reg.byPattern[key] = rt.Owner

	r.mux.Handle(muxPattern(rt.Method, rt.Pattern), r.wrap(h))
	r.reg.routes = append(r.reg.routes, rt)
}

// HandleFunc is Handle for a handler function.
func (r *Router) HandleFunc(method, pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(method, pattern, h, opts...)
}

// Any declares a route matching every method.
func (r *Router) Any(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle("", pattern, h, opts...)
}

// Get declares a GET route. ServeMux also matches it for HEAD.
func (r *Router) Get(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodGet, pattern, h, opts...)
}

// Post declares a POST route.
func (r *Router) Post(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodPost, pattern, h, opts...)
}

// Put declares a PUT route.
func (r *Router) Put(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodPut, pattern, h, opts...)
}

// Patch declares a PATCH route.
func (r *Router) Patch(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodPatch, pattern, h, opts...)
}

// Delete declares a DELETE route.
func (r *Router) Delete(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodDelete, pattern, h, opts...)
}

// Head declares a HEAD route.
func (r *Router) Head(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodHead, pattern, h, opts...)
}

// Options declares an OPTIONS route.
func (r *Router) Options(pattern string, h http.HandlerFunc, opts ...Option) {
	r.Handle(http.MethodOptions, pattern, h, opts...)
}

// Mount serves every path under prefix with h, which receives the request with
// its URL untouched: no pattern matching, no path canonicalization, no
// percent-decoding. It is for handlers that own their own URL space and cannot
// be restated as patterns - foreign libraries such as tus, handlers that parse
// the trailing path themselves such as pprof.Index, and WebDAV, whose clients
// are sensitive to the redirects ServeMux issues on non-canonical paths.
//
// Mounts are matched before patterns, longest prefix first, so a service can
// mount a subtree and still declare exact routes inside it.
func (r *Router) Mount(prefix string, h http.Handler, opts ...Option) {
	rt := Route{
		Owner:   r.owner,
		Pattern: join(r.prefix, prefix),
		Subtree: true,
	}
	for _, o := range opts {
		o(&rt)
	}

	r.reg.mounts = append(r.reg.mounts, mount{prefix: rt.Pattern, handler: r.wrap(h), methods: rt.Methods})
	sort.SliceStable(r.reg.mounts, func(i, j int) bool {
		return len(r.reg.mounts[i].prefix) > len(r.reg.mounts[j].prefix)
	})

	// A subtree that named its methods is recorded as one route per method, so
	// that the table lists what it serves rather than just where it lives.
	if len(rt.Methods) == 0 {
		r.reg.routes = append(r.reg.routes, rt)
		return
	}
	for _, m := range rt.Methods {
		per := rt
		per.Method = m
		per.Methods = nil
		r.reg.routes = append(r.reg.routes, per)
	}
}

// Routes returns every declared route, in declaration order.
func (r *Router) Routes() []Route {
	out := make([]Route, len(r.reg.routes))
	copy(out, r.reg.routes)
	return out
}

// Unprotected returns the paths of the routes exempt from authentication. It
// describes the server rather than driving it: authentication is decided per
// matched route, through Match.
func (r *Router) Unprotected() []string {
	var out []string
	seen := map[string]bool{}
	for _, rt := range r.reg.routes {
		if rt.Unprotected && !seen[rt.Pattern] {
			seen[rt.Pattern] = true
			out = append(out, rt.Pattern)
		}
	}
	return out
}

// Match reports the route a request resolves to, without serving it. It
// returns false when nothing matches, and for requests ServeMux answers itself
// (the redirect it issues for non-canonical paths), so callers treat an
// unmatched request as protected.
func (r *Router) Match(req *http.Request) (Route, bool) {
	if i := r.matchMount(req.URL.EscapedPath()); i >= 0 {
		if !r.reg.mounts[i].serves(req.Method) {
			return Route{}, false
		}
		return r.routeForMount(r.reg.mounts[i].prefix, req.Method), true
	}
	_, pattern := r.mux.Handler(req)
	if pattern == "" {
		return Route{}, false
	}
	method, p := splitMuxPattern(pattern)
	// ServeMux reports the pattern a request would reach *after* it redirects,
	// both for a non-canonical path and for a subtree pattern addressed
	// without its trailing slash. Neither request reaches a handler, so
	// neither has a route, and reporting one would hand the caller an
	// Unprotected flag for a request nobody declared.
	if !isCanonical(req.URL.Path) {
		return Route{}, false
	}
	if strings.HasSuffix(p, "/") && req.URL.Path == strings.TrimSuffix(p, "/") {
		return Route{}, false
	}
	for _, rt := range r.reg.routes {
		if !rt.Subtree && rt.Method == method && rt.Pattern == p {
			return rt, true
		}
	}
	return Route{}, false
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if i := r.matchMount(req.URL.EscapedPath()); i >= 0 {
		m := r.reg.mounts[i]
		if !m.serves(req.Method) {
			w.Header().Set("Allow", strings.Join(m.methods, ", "))
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		m.handler.ServeHTTP(w, req)
		return
	}
	r.mux.ServeHTTP(w, req)
}

// matchMount returns the index of the longest mount whose prefix covers path,
// or -1. Mounts are kept sorted longest first, so the first hit wins.
func (r *Router) matchMount(path string) int {
	for i, m := range r.reg.mounts {
		if m.prefix == "/" || path == m.prefix || strings.HasPrefix(path, strings.TrimSuffix(m.prefix, "/")+"/") {
			return i
		}
	}
	return -1
}

func (r *Router) routeForMount(prefix, method string) Route {
	var any Route
	for _, rt := range r.reg.routes {
		if !rt.Subtree || rt.Pattern != prefix {
			continue
		}
		if rt.Method == method {
			return rt
		}
		if rt.Method == "" {
			any = rt
		}
	}
	if any.Pattern != "" {
		return any
	}
	return Route{Pattern: prefix, Subtree: true}
}

func (r *Router) wrap(h http.Handler) http.Handler {
	for _, v := range slices.Backward(r.mw) {
		h = v(h)
	}
	return h
}

// join appends a pattern to a group prefix. A pattern of "/" addresses the
// prefix itself, so that a group can declare a handler for its own root.
func join(prefix, pattern string) string {
	if prefix == "" {
		if pattern == "" {
			return "/"
		}
		return pattern
	}
	if pattern == "" || pattern == "/" {
		return prefix
	}
	return strings.TrimSuffix(prefix, "/") + pattern
}

// isCanonical reports whether ServeMux serves the path as it arrived, rather
// than redirecting to its cleaned form. It mirrors the standard library's own
// path cleaning.
func isCanonical(p string) bool {
	if p == "" {
		return false
	}
	cleaned := path.Clean(p)
	if strings.HasSuffix(p, "/") && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned == p
}

func muxPattern(method, pattern string) string {
	if method == "" {
		return pattern
	}
	return method + " " + pattern
}

func splitMuxPattern(p string) (method, pattern string) {
	if before, after, ok := strings.Cut(p, " "); ok {
		return before, strings.TrimLeft(after, " ")
	}
	return "", p
}
