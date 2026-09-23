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

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
)

// reg is the registry the gateway reads. The global resolver takes the first
// registry installed, so the package shares one and tests add to it.
var reg = func() registry.Registry {
	r := memory.New(nil)
	service.SetGlobalRegistry(r)
	service.SetGlobal(service.NewClients(r))
	return r
}()

// backend starts a server that reports the method and the raw request URI, so
// a test can see exactly what the gateway forwarded.
func backend(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", r.Method, r.URL.RequestURI())
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// advertise registers a service whose nodes declare the given routes.
func advertise(t *testing.T, name, addr string, routes []router.Route) {
	t.Helper()
	encoded, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	node := registry.NewNode(addr+"-"+name, addr, map[string]string{
		registry.MetaTransport: registry.TransportHTTP,
		registry.MetaScheme:    "http",
		registry.MetaState:     registry.StateReady,
		registry.MetaRoutes:    string(encoded),
	})
	if err := reg.Add(registry.NewService(name, []registry.Node{node})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reg.Remove(registry.NewService(name, []registry.Node{node})) })
}

func newGateway(t *testing.T, m map[string]any) *svc {
	t.Helper()
	g, err := New(context.Background(), m)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	s := g.(*svc)
	t.Cleanup(func() { _ = s.Close() })
	s.rebuild()
	return s
}

func request(s *svc, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestForwardsToTheDeclaringService(t *testing.T) {
	ocs := backend(t)
	ocdav := backend(t)

	advertise(t, "ocs", ocs, []router.Route{
		{Owner: "ocs", Method: http.MethodGet, Pattern: "/ocs/v1.php/config"},
		{Owner: "ocs", Method: http.MethodPost, Pattern: "/ocs/v1.php/shares/{id}"},
	})
	advertise(t, "ocdav", ocdav, []router.Route{
		{Owner: "ocdav", Pattern: "/remote.php", Subtree: true},
	})

	s := newGateway(t, map[string]any{})

	tests := map[string]struct {
		method string
		target string
		body   string
	}{
		"exact route":  {http.MethodGet, "/ocs/v1.php/config", "GET /ocs/v1.php/config"},
		"wildcard":     {http.MethodPost, "/ocs/v1.php/shares/42", "POST /ocs/v1.php/shares/42"},
		"mount":        {http.MethodGet, "/remote.php/dav/files/a.txt", "GET /remote.php/dav/files/a.txt"},
		"mount itself": {http.MethodGet, "/remote.php", "GET /remote.php"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec := request(s, tt.method, tt.target)
			if rec.Code != http.StatusOK {
				t.Fatalf("got status %d, expected 200: %s", rec.Code, rec.Body)
			}
			if rec.Body.String() != tt.body {
				t.Errorf("got %q, expected %q", rec.Body.String(), tt.body)
			}
		})
	}
}

// A mounted subtree must reach the service with the path exactly as it
// arrived: WebDAV addresses resources by path, and an encoded separator or a
// doubled slash means something there.
func TestForwardsThePathUntouched(t *testing.T) {
	ocdav := backend(t)
	advertise(t, "ocdav", ocdav, []router.Route{
		{Owner: "ocdav", Pattern: "/remote.php", Subtree: true},
	})

	s := newGateway(t, map[string]any{})

	for _, target := range []string{
		"/remote.php/dav/files/a%2Fb.txt",
		"/remote.php/dav/files//double",
		"/remote.php/dav/files/a%20b.txt",
		"/remote.php/dav/files/x?a=1&b=2",
	} {
		t.Run(target, func(t *testing.T) {
			rec := request(s, http.MethodGet, target)
			if got := rec.Body.String(); got != "GET "+target {
				t.Errorf("got %q, expected %q", got, "GET "+target)
			}
		})
	}
}

func TestUndeclaredRequests(t *testing.T) {
	ocs := backend(t)
	advertise(t, "ocs", ocs, []router.Route{
		{Owner: "ocs", Method: http.MethodGet, Pattern: "/ocs/v1.php/config"},
	})

	s := newGateway(t, map[string]any{})

	tests := map[string]struct {
		method string
		target string
		code   int
	}{
		"unknown path":    {http.MethodGet, "/nothing/here", http.StatusNotFound},
		"method mismatch": {http.MethodDelete, "/ocs/v1.php/config", http.StatusMethodNotAllowed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if rec := request(s, tt.method, tt.target); rec.Code != tt.code {
				t.Errorf("got status %d, expected %d", rec.Code, tt.code)
			}
		})
	}
}

// Mirroring its own catch-all would send every request back to itself.
func TestNeverMirrorsItself(t *testing.T) {
	advertise(t, "gateway", "127.0.0.1:1", []router.Route{
		{Owner: "gateway", Pattern: "/", Subtree: true},
	})
	ocs := backend(t)
	advertise(t, "ocs", ocs, []router.Route{
		{Owner: "ocs", Method: http.MethodGet, Pattern: "/ocs/v1.php/config"},
	})

	s := newGateway(t, map[string]any{})

	if rec := request(s, http.MethodGet, "/nothing/here"); rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, expected 404: the gateway mirrored itself", rec.Code)
	}
}

func TestServiceFilters(t *testing.T) {
	ocs := backend(t)
	graph := backend(t)
	advertise(t, "ocs", ocs, []router.Route{
		{Owner: "ocs", Method: http.MethodGet, Pattern: "/ocs/v1.php/config"},
	})
	advertise(t, "ocgraph", graph, []router.Route{
		{Owner: "ocgraph", Method: http.MethodGet, Pattern: "/graph/v1.0/me"},
	})

	t.Run("exclude", func(t *testing.T) {
		s := newGateway(t, map[string]any{"exclude": []any{"ocgraph"}})
		if rec := request(s, http.MethodGet, "/graph/v1.0/me"); rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, expected the excluded service not to be mirrored", rec.Code)
		}
		if rec := request(s, http.MethodGet, "/ocs/v1.php/config"); rec.Code != http.StatusOK {
			t.Errorf("got status %d, expected 200", rec.Code)
		}
	})

	t.Run("allow list", func(t *testing.T) {
		s := newGateway(t, map[string]any{"services": []any{"ocgraph"}})
		if rec := request(s, http.MethodGet, "/ocs/v1.php/config"); rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, expected only the listed service to be mirrored", rec.Code)
		}
		if rec := request(s, http.MethodGet, "/graph/v1.0/me"); rec.Code != http.StatusOK {
			t.Errorf("got status %d, expected 200", rec.Code)
		}
	})
}

// Two services claiming patterns that overlap without one being more specific
// makes the router refuse the pair. That must cost the offending service, not
// the whole table.
func TestConflictingRoutesSkipOnlyThatService(t *testing.T) {
	good := backend(t)
	advertise(t, "good", good, []router.Route{
		{Owner: "good", Method: http.MethodGet, Pattern: "/good/thing"},
	})
	advertise(t, "bad", backend(t), []router.Route{
		{Owner: "bad", Method: http.MethodPost, Pattern: "/bad/{a}/x"},
		{Owner: "bad", Method: http.MethodPost, Pattern: "/bad/x/{b}"},
	})

	s := newGateway(t, map[string]any{})

	if rec := request(s, http.MethodGet, "/good/thing"); rec.Code != http.StatusOK {
		t.Errorf("got status %d, expected the healthy service to still be served", rec.Code)
	}
	if rec := request(s, http.MethodPost, "/bad/y/x"); rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, expected the conflicting service to be skipped", rec.Code)
	}
}

// A service with no reachable node answers 502 rather than hanging or panicking.
func TestUnreachableServiceIsABadGateway(t *testing.T) {
	advertise(t, "gone", "127.0.0.1:1", []router.Route{
		{Owner: "gone", Method: http.MethodGet, Pattern: "/gone"},
	})

	s := newGateway(t, map[string]any{})

	if rec := request(s, http.MethodGet, "/gone"); rec.Code != http.StatusBadGateway {
		t.Errorf("got status %d, expected 502", rec.Code)
	}
}

func TestRoutesClaimsEverything(t *testing.T) {
	s := newGateway(t, map[string]any{})

	r := router.New()
	s.Routes(r.Service("gateway"))

	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("got %d routes, expected 1", len(routes))
	}
	if routes[0].Pattern != "/" || !routes[0].Subtree || !routes[0].Unprotected {
		t.Errorf("got %+v, expected an unprotected subtree at /", routes[0])
	}
}

func TestRejectsABadRefreshInterval(t *testing.T) {
	if _, err := New(context.Background(), map[string]any{"refresh_interval": "soon"}); err == nil {
		t.Error("expected an error for an unparsable refresh interval")
	}
}
