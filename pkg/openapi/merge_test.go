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
	"net/http"
	"testing"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/getkin/kin-openapi/openapi3"
)

// The services of a fleet each describe what they serve, on servers that can
// authenticate differently; the deployment's document is all of it.
func TestMergeCombinesServices(t *testing.T) {
	ocs := renderWith(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/shares", noop, router.Describe(router.Doc{
			Responses: map[int]any{200: shareResponse{}},
		}))
	}, []auth.Scheme{{Name: "basic", Kind: auth.SchemeBasic}})

	graph := renderWith(t, func(r *router.Router) {
		r.Service("ocgraph").Get("/graph/v1.0/me", noop, router.Describe(router.Doc{
			Responses: map[int]any{200: shareOwner{}},
		}))
		// The same path as ocs, with a method ocs does not serve.
		r.Service("ocgraph").Post("/ocs/v1.php/shares", noop)
	}, []auth.Scheme{{Name: "bearer", Kind: auth.SchemeBearer}})

	doc := &openapi3.T{OpenAPI: Version, Info: &openapi3.Info{Title: "fleet", Version: "test"}}
	Merge(doc, ocs)
	Merge(doc, graph)

	validate(t, doc)

	shares := doc.Paths.Value("/ocs/v1.php/shares")
	if shares == nil || shares.Get == nil || shares.Post == nil {
		t.Fatalf("both services' operations on the shared path should be kept: %+v", shares)
	}
	if doc.Paths.Value("/graph/v1.0/me") == nil {
		t.Error("the second service's path is missing")
	}

	for _, name := range []string{"shareResponse", "shareOwner"} {
		if _, ok := doc.Components.Schemas[schemaPrefix+name]; !ok {
			t.Errorf("schema %q is missing", name)
		}
	}
	for _, name := range []string{"basic", "bearer"} {
		if _, ok := doc.Components.SecuritySchemes[name]; !ok {
			t.Errorf("security scheme %q is missing", name)
		}
	}

	if len(doc.Tags) != 2 || doc.Tags[0].Name != "ocgraph" || doc.Tags[1].Name != "ocs" {
		t.Errorf("got tags %+v, want both services, sorted", doc.Tags)
	}
}

// What the document already has wins, so merging the same service twice, or a
// second instance of it, changes nothing.
func TestMergeKeepsWhatIsThere(t *testing.T) {
	first := renderWith(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/shares", noop, router.Describe(router.Doc{Summary: "first"}))
	}, nil)
	second := renderWith(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/shares", noop, router.Describe(router.Doc{Summary: "second"}))
	}, nil)

	doc := &openapi3.T{OpenAPI: Version, Info: &openapi3.Info{Title: "fleet", Version: "test"}}
	Merge(doc, first)
	Merge(doc, second)

	if got := doc.Paths.Value("/ocs/v1.php/shares").GetOperation(http.MethodGet).Summary; got != "first" {
		t.Errorf("got summary %q, want the first service's", got)
	}
	if len(doc.Tags) != 1 {
		t.Errorf("got tags %+v, want one", doc.Tags)
	}
}
