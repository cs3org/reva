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

package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/openapi"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/getkin/kin-openapi/openapi3"
)

var noop = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// serve builds a router holding the service plus a service of its own, which
// is what the document should describe.
func serve(t *testing.T, m map[string]any) *router.Router {
	return serveWith(t, context.Background(), m)
}

func serveWith(t *testing.T, ctx context.Context, m map[string]any) *router.Router {
	return serveFleet(t, ctx, m, noFleet{})
}

func serveFleet(t *testing.T, ctx context.Context, m map[string]any, f fleet) *router.Router {
	t.Helper()

	s, err := New(ctx, m)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.(*svc).fleet = f

	r := router.New()
	s.Routes(r.Service("openapi"))
	r.Service("ocs").Get("/ocs/v1.php/cloud/capabilities", noop, router.Unprotected())
	return r
}

// noFleet stands in for a deployment without the Admin API.
type noFleet struct{}

func (noFleet) Documents(context.Context) (map[string][]byte, error) {
	return nil, errors.New("no admin api")
}

// fakeFleet answers with a document per service, and counts how often it was
// asked.
type fakeFleet struct {
	documents map[string][]byte
	asked     int
}

func (f *fakeFleet) Documents(context.Context) (map[string][]byte, error) {
	f.asked++
	return f.documents, nil
}

// describe renders what a service of the fleet would answer with.
func describe(t *testing.T, declare func(*router.Router), schemes []auth.Scheme) []byte {
	t.Helper()
	r := router.New()
	declare(r)
	doc, err := openapi.From(r.Routes(), &openapi3.Info{Title: "service", Version: "test"}, schemes)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func get(r *router.Router, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// The document describes what the other services declared, which is only
// complete once they have: it is built when first asked for, not when this
// service is constructed.
func TestDocumentDescribesTheWholeRouter(t *testing.T) {
	r := serve(t, map[string]any{"public": true})

	rec := get(r, "/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, expected 200: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("got content type %q", ct)
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the document is not valid JSON: %v", err)
	}

	paths, _ := doc["paths"].(map[string]any)
	for _, want := range []string{"/ocs/v1.php/cloud/capabilities", "/openapi.json", "/openapi"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("%q is missing from the document", want)
		}
	}

	info, _ := doc["info"].(map[string]any)
	if info["title"] != "reva" {
		t.Errorf("got title %v, expected the default", info["title"])
	}
}

func TestPageRendersAndPointsAtTheDocument(t *testing.T) {
	r := serve(t, map[string]any{"public": true, "title": "CERNBox"})

	rec := get(r, "/openapi")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, expected 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("got content type %q", ct)
	}

	// The browser decodes the entities html/template writes, such as the
	// "+" of a hash.
	body := html.UnescapeString(rec.Body.String())
	for _, want := range []string{"CERNBox", "openapi.json", "SwaggerUIBundle", defaultUIBundle, defaultUIBundleIntegrity, defaultUIStylesheetIntegrity} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not mention %q", want)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Error("the page still holds an unsubstituted placeholder")
	}
}

// A title is operator input and ends up in the page, so it is escaped.
func TestConfiguredValuesAreEscaped(t *testing.T) {
	r := serve(t, map[string]any{"public": true, "title": `x"><script>alert(1)</script>`})

	body := get(r, "/openapi").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("the configured title was not escaped")
	}
}

// The document is an inventory of everything the deployment serves, so it is
// behind the auth middleware unless an operator opens it.
func TestDocumentationIsProtectedByDefault(t *testing.T) {
	r := serve(t, map[string]any{})

	for _, target := range []string{"/openapi.json", "/openapi"} {
		rt, ok := r.Match(httptest.NewRequest(http.MethodGet, target, nil))
		if !ok {
			t.Fatalf("%s resolves to no route", target)
		}
		if rt.Unprotected {
			t.Errorf("%s is unprotected by default", target)
		}
	}

	public := serve(t, map[string]any{"public": true})
	rt, _ := public.Match(httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if !rt.Unprotected {
		t.Error("public = true should open the documentation")
	}
}

// How the server authenticates is settled by its auth middleware, so the
// service is told rather than configured with it a second time.
func TestAuthenticationComesFromTheServer(t *testing.T) {
	ctx := auth.WithSchemes(context.Background(), []auth.Scheme{
		{Name: "appauth", Kind: auth.SchemeBasic, Description: "An app password."},
		{Name: "revaToken", Kind: auth.SchemeHeader, Param: "x-access-token"},
	})
	r := serveWith(t, ctx, map[string]any{"public": true})

	var doc struct {
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `json:"type"`
				Scheme string `json:"scheme"`
				In     string `json:"in"`
				Name   string `json:"name"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(get(r, "/openapi.json").Body.Bytes(), &doc); err != nil {
		t.Fatalf("the document is not valid JSON: %v", err)
	}

	got := doc.Components.SecuritySchemes
	if len(got) != 2 {
		t.Fatalf("got security schemes %+v, want the two the server accepts", got)
	}
	if s := got["appauth"]; s.Type != "http" || s.Scheme != "basic" {
		t.Errorf("appauth: got %+v, want http basic", s)
	}
	if s := got["revaToken"]; s.Type != "apiKey" || s.In != "header" || s.Name != "x-access-token" {
		t.Errorf("revaToken: got %+v, want the x-access-token header", s)
	}
}

// A default hash fits only the default file: a copy elsewhere is checked only
// against the hash it is given.
func TestIntegrityFollowsTheBundle(t *testing.T) {
	body := html.UnescapeString(get(serve(t, map[string]any{"public": true, "ui_bundle": "https://mirror.example.org/swagger-ui-bundle.js"}), "/openapi").Body.String())
	if strings.Contains(body, defaultUIBundleIntegrity) {
		t.Error("the default hash should not be applied to another file")
	}
	if !strings.Contains(body, defaultUIStylesheetIntegrity) {
		t.Error("the default stylesheet keeps its hash")
	}

	body = get(serve(t, map[string]any{
		"public":              true,
		"ui_bundle":           "https://mirror.example.org/swagger-ui-bundle.js",
		"ui_bundle_integrity": "sha384-mirror",
	}), "/openapi").Body.String()
	if !strings.Contains(body, `integrity="sha384-mirror"`) {
		t.Error("the configured hash is missing")
	}
}

// Every service of the fleet describes its own routes, authenticated as its
// own server is; the document is all of them.
func TestDocumentDescribesTheFleet(t *testing.T) {
	f := &fakeFleet{documents: map[string][]byte{
		"ocgraph": describe(t, func(r *router.Router) {
			r.Service("ocgraph").Get("/graph/v1.0/me", noop)
		}, []auth.Scheme{{Name: "bearer", Kind: auth.SchemeBearer}}),
		"ocdav": describe(t, func(r *router.Router) {
			r.Service("ocdav").Mount("/remote.php/dav", noop, router.Methods("PROPFIND"))
		}, []auth.Scheme{{Name: "basic", Kind: auth.SchemeBasic}}),
	}}
	r := serveFleet(t, context.Background(), map[string]any{"public": true}, f)

	doc := loadDocument(t, r)
	for _, want := range []string{"/graph/v1.0/me", "/remote.php/dav/{path}"} {
		if doc.Paths.Value(want) == nil {
			t.Errorf("%q is missing from the document", want)
		}
	}
	// The fleet answered, so this server's own routes come from the fleet
	// too, and are not added a second time from here.
	if doc.Paths.Value("/ocs/v1.php/cloud/capabilities") != nil {
		t.Error("the local routes should come from the fleet when it answers")
	}
	for _, name := range []string{"bearer", "basic"} {
		if doc.Components.SecuritySchemes[name] == nil {
			t.Errorf("security scheme %q is missing", name)
		}
	}
}

// Building the document asks every service through the Admin API, which audits
// each call: it is built again only when it is older than refresh_interval.
func TestDocumentIsKeptUntilTheRefreshInterval(t *testing.T) {
	f := &fakeFleet{documents: map[string][]byte{
		"ocgraph": describe(t, func(r *router.Router) {
			r.Service("ocgraph").Get("/graph/v1.0/me", noop)
		}, nil),
	}}
	r := serveFleet(t, context.Background(), map[string]any{"public": true, "refresh_interval": "1h"}, f)

	get(r, "/openapi.json")
	get(r, "/openapi.json")
	if f.asked != 1 {
		t.Errorf("the fleet was asked %d times, want once", f.asked)
	}
}

func loadDocument(t *testing.T, r *router.Router) *openapi3.T {
	t.Helper()
	rec := get(r, "/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", rec.Code, rec.Body)
	}
	doc, err := openapi3.NewLoader().LoadFromData(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("the document does not load: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("the document is not valid OpenAPI: %v", err)
	}
	return doc
}
