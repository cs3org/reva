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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

var noop = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// serve builds a router holding the service plus a service of its own, which
// is what the document should describe.
func serve(t *testing.T, m map[string]any) *router.Router {
	return serveWith(t, context.Background(), m)
}

func serveWith(t *testing.T, ctx context.Context, m map[string]any) *router.Router {
	t.Helper()

	s, err := New(ctx, m)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	r := router.New()
	s.Routes(r.Service("openapi"))
	r.Service("ocs").Get("/ocs/v1.php/cloud/capabilities", noop, router.Unprotected())
	return r
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

	body := rec.Body.String()
	for _, want := range []string{"CERNBox", "/openapi.json", "SwaggerUIBundle"} {
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
// behind the auth middleware unless an operator opens it. The page holds
// nothing of the deployment's own, and is where a client without a browser
// session enters its token, so it is always open.
func TestDocumentIsProtectedByDefault(t *testing.T) {
	match := func(r *router.Router, target string) router.Route {
		t.Helper()
		rt, ok := r.Match(httptest.NewRequest(http.MethodGet, target, nil))
		if !ok {
			t.Fatalf("%s resolves to no route", target)
		}
		return rt
	}

	r := serve(t, map[string]any{})
	if match(r, "/openapi.json").Unprotected {
		t.Error("the document is unprotected by default")
	}
	if !match(r, "/openapi").Unprotected {
		t.Error("the page should be open, or a token cannot be entered anywhere")
	}

	public := serve(t, map[string]any{"public": true})
	if !match(public, "/openapi.json").Unprotected {
		t.Error("public = true should open the document")
	}
}

// The viewer runs next to the credentials of whoever opens the page, so the
// default one is a fixed release the browser checks against its hash.
func TestDefaultViewerIsPinned(t *testing.T) {
	body := get(serve(t, map[string]any{}), "/openapi").Body.String()
	for _, want := range []string{
		defaultUIBundle, `integrity="` + defaultUIBundleIntegrity + `"`,
		defaultUIStylesheet, `integrity="` + defaultUIStylesheetIntegrity + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not carry %q", want)
		}
	}
}

// The default hash belongs to the default file: it would refuse any other.
func TestOwnViewerTakesItsOwnHash(t *testing.T) {
	body := get(serve(t, map[string]any{
		"ui_bundle":           "/static/swagger-ui-bundle.js",
		"ui_bundle_integrity": "sha384-own",
		"ui_stylesheet":       "/static/swagger-ui.css",
	}), "/openapi").Body.String()

	if !strings.Contains(body, `src="/static/swagger-ui-bundle.js" integrity="sha384-own"`) {
		t.Error("the configured bundle should carry the configured hash")
	}
	if !strings.Contains(body, `href="/static/swagger-ui.css" integrity=""`) {
		t.Error("a stylesheet configured without a hash should carry none")
	}
	if strings.Contains(body, defaultUIBundleIntegrity) || strings.Contains(body, defaultUIStylesheetIntegrity) {
		t.Error("a default hash was applied to a file it does not belong to")
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
