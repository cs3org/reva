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

	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

var noop = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// serve builds a router holding the service plus a service of its own, which
// is what the document should describe.
func serve(t *testing.T, m map[string]any) *router.Router {
	t.Helper()

	s, err := New(context.Background(), m)
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
