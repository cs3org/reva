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
	"reflect"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/getkin/kin-openapi/openapi3"
)

// noop stands in for the handlers, which the document does not look at.
var noop = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

// schemes stand in for how a deployment was configured to authenticate.
var schemes = []auth.Scheme{
	{Name: "basic", Kind: auth.SchemeBasic, Description: "A username and password."},
	{Name: "revaToken", Kind: auth.SchemeHeader, Param: "x-access-token"},
}

func render(t *testing.T, declare func(*router.Router)) *openapi3.T {
	return renderWith(t, declare, schemes)
}

// renderWith builds the document and checks it is valid OpenAPI, which every
// test then gets for free.
func renderWith(t *testing.T, declare func(*router.Router), schemes []auth.Scheme) *openapi3.T {
	t.Helper()
	r := router.New()
	declare(r)
	doc, err := From(r.Routes(), &openapi3.Info{Title: "reva", Version: "test"}, schemes)
	if err != nil {
		t.Fatalf("From returned error: %v", err)
	}
	validate(t, doc)
	return doc
}

// validate checks the document as a client reads it: written out and loaded
// back, which is also what resolves the references between its schemas.
func validate(t *testing.T, doc *openapi3.T) *openapi3.T {
	t.Helper()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	back, err := openapi3.NewLoader().LoadFromData(encoded)
	if err != nil {
		t.Fatalf("the document does not load back: %v", err)
	}
	if err := back.Validate(context.Background()); err != nil {
		t.Fatalf("the document is not valid OpenAPI: %v", err)
	}
	return back
}

// What a service declared is what the document says, without the service
// having to describe it: the path, the method, and whether it needs
// credentials are all in the route already.
func TestDocumentFromRoutesAlone(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/cloud/capabilities", noop, router.Unprotected())
		r.Service("ocs").Post("/ocs/v1.php/apps/files_sharing/api/v1/shares", noop)
	})

	caps := doc.Paths.Value("/ocs/v1.php/cloud/capabilities")
	if caps == nil || caps.Get == nil {
		t.Fatalf("the capabilities endpoint is missing: %v", doc.Paths.InMatchingOrder())
	}
	if caps.Get.Security != nil {
		t.Errorf("an unprotected route should need no credentials, got %+v", caps.Get.Security)
	}
	if got := caps.Get.Tags; len(got) != 1 || got[0] != "ocs" {
		t.Errorf("got tags %v, expected the owning service", got)
	}

	shares := doc.Paths.Value("/ocs/v1.php/apps/files_sharing/api/v1/shares")
	if shares == nil || shares.Post == nil {
		t.Fatal("the shares endpoint is missing")
	}
	if shares.Post.Security == nil || len(*shares.Post.Security) == 0 {
		t.Error("a protected route should say it needs credentials")
	}

	if len(doc.Tags) != 1 || doc.Tags[0].Name != "ocs" {
		t.Errorf("got tags %+v, expected one per owning service", doc.Tags)
	}
}

// The parameters a pattern names are parameters of the path, and need no
// describing.
func TestPathParametersComeFromThePattern(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocgraph").Get("/graph/v1beta1/drives/{spaceID}/items/{resourceID}", noop)
	})

	item := doc.Paths.Value("/graph/v1beta1/drives/{spaceID}/items/{resourceID}")
	if item == nil {
		t.Fatal("the path is missing")
	}

	var names []string
	for _, p := range item.Parameters {
		if p.Value.In != "path" || !p.Value.Required {
			t.Errorf("got %+v, expected a required path parameter", p.Value)
		}
		names = append(names, p.Value.Name)
	}
	if strings.Join(names, ",") != "spaceID,resourceID" {
		t.Errorf("got parameters %v, expected both of the pattern's", names)
	}
}

// A mounted subtree addresses everything below it, which the document says
// with a catch-all parameter rather than pretending the prefix is the whole
// path.
func TestSubtreeBecomesACatchAll(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocdav").Mount("/remote.php/dav/files", noop, router.Methods(http.MethodGet, "PROPFIND"))
	})

	item := doc.Paths.Value("/remote.php/dav/files/{path}")
	if item == nil {
		t.Fatal("the subtree is missing")
	}
	if item.Get == nil {
		t.Error("the subtree's GET is missing")
	}
	if len(item.Parameters) != 1 || item.Parameters[0].Value.Name != "path" {
		t.Errorf("got %+v, expected a single catch-all parameter", item.Parameters)
	}
	if !strings.Contains(item.Parameters[0].Value.Description, "slashes") {
		t.Errorf("got %q, expected the parameter to say it spans segments", item.Parameters[0].Value.Description)
	}
}

// The WebDAV methods are operations like any other: OpenAPI 3.2 names the
// methods outside its fixed set in additionalOperations.
func TestWebDAVMethodsAreOperations(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocdav").Mount("/remote.php/dav/files", nil,
			router.Methods("PROPFIND", "MKCOL", http.MethodGet))
	})

	item := doc.Paths.Value("/remote.php/dav/files/{path}")
	if item == nil {
		t.Fatal("the subtree is missing")
	}
	if item.Get == nil {
		t.Error("GET should still be an operation")
	}
	for _, m := range []string{"PROPFIND", "MKCOL"} {
		op := item.AdditionalOperations[m]
		if op == nil {
			t.Errorf("%s is missing from the additional operations: %v", m, item.AdditionalOperations)
			continue
		}
		if op.Security == nil {
			t.Errorf("%s should need credentials like the other operations", m)
		}
	}
}

type shareRequest struct {
	Path        string `json:"path"`
	ShareType   int    `json:"shareType"`
	Password    string `json:"password,omitempty"`
	Description string `json:"description,omitempty"`
}

type shareResponse struct {
	ID    string      `json:"id"`
	Owner *shareOwner `json:"owner,omitempty"`
	Links []shareLink `json:"links"`
}

type shareOwner struct {
	Username string `json:"username"`
}

type shareLink struct {
	URL string `json:"url"`
}

// A route that describes itself contributes its parameters, its body and its
// responses, with the schemas reflected out of the Go types it uses.
func TestDescribedRoute(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Post("/ocs/v1.php/shares", noop, router.Describe(router.Doc{
			Summary: "Create a share",
			Params: []router.Param{
				router.RequiredQuery("format", "json or xml").WithExample("json"),
				router.Query("shared_with_me", "only shares received"),
			},
			Body: shareRequest{},
			Responses: map[int]any{
				200: shareResponse{},
				400: nil,
			},
		}))
	})

	op := doc.Paths.Value("/ocs/v1.php/shares").Post
	if op.Summary != "Create a share" {
		t.Errorf("got summary %q", op.Summary)
	}

	if len(op.Parameters) != 2 {
		t.Fatalf("got %d parameters, expected 2", len(op.Parameters))
	}
	if p := op.Parameters[0].Value; p.Name != "format" || !p.Required || p.Example != "json" {
		t.Errorf("got %+v, expected the required query parameter with its example", p)
	}
	if op.Parameters[1].Value.Required {
		t.Error("the second parameter is optional")
	}

	if op.RequestBody == nil || op.RequestBody.Value.Content.Get(jsonMedia) == nil {
		t.Fatal("the request body is missing")
	}

	ok := op.Responses.Status(200)
	if ok == nil || ok.Value.Content.Get(jsonMedia) == nil {
		t.Fatal("the 200 response is missing a body")
	}
	// A status that returns nothing says so, rather than describing a body.
	bad := op.Responses.Status(400)
	if bad == nil {
		t.Fatal("the 400 response is missing")
	}
	if bad.Value.Content != nil {
		t.Errorf("got %+v, expected no body for a status that returns none", bad.Value.Content)
	}

	// Every schema lives centrally and is referred to, so a type used twice is
	// described once.
	for _, name := range []string{"shareRequest", "shareResponse", "shareOwner", "shareLink"} {
		if _, ok := doc.Components.Schemas[schemaPrefix+name]; !ok {
			t.Errorf("schema %q is missing from the components: %v", name, keys(doc.Components.Schemas))
		}
	}
}

// schemaPrefix is how this package's types are named in the components.
const schemaPrefix = "github.com.cs3org.reva.v3.pkg.openapi."

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A field is required only if something says it is. Leaving out omitempty
// does not make a field required: a PATCH body is a partial update, and a
// pointer field is one that may be left out.
func TestFieldsAreNotRequiredByDefault(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Patch("/ocs/v1.php/shares/{id}", noop, router.Describe(router.Doc{
			Body: shareResponse{},
		}))
	})

	s := doc.Components.Schemas[schemaPrefix+"shareResponse"]
	if s == nil {
		t.Fatal("the body's schema is missing")
	}
	if len(s.Value.Required) != 0 {
		t.Errorf("got required fields %v, want none", s.Value.Required)
	}
	if owner := s.Value.Properties["owner"]; owner == nil || owner.Value != nil && owner.Value.Nullable {
		t.Errorf("a pointer field should be optional, not nullable: %+v", owner)
	}
}

type selfReferencing struct {
	Name     string             `json:"name"`
	Children []*selfReferencing `json:"children,omitempty"`
}

// A type that refers to itself is described once, and refers to its own
// component.
func TestRecursiveTypesTerminate(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/tree", noop, router.Describe(router.Doc{
			Responses: map[int]any{200: selfReferencing{}},
		}))
	})

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "#/components/schemas/"+schemaPrefix+"selfReferencing") {
		t.Errorf("the type does not refer to its own component: %s", encoded)
	}
}

// Two paths can spell the same operation id; the document still gives each
// operation its own.
func TestOperationIDsAreUnique(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/a.b", noop)
		r.Service("ocs").Get("/a/b", noop)
	})

	first := doc.Paths.Value("/a.b").Get.OperationID
	second := doc.Paths.Value("/a/b").Get.OperationID
	if first == second {
		t.Errorf("both operations are called %q", first)
	}
}

// The auth middleware lets every OPTIONS request through, so the document
// does not ask for credentials there.
func TestOptionsNeedsNoCredentials(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Any("/ocs/v1.php/anything", noop)
	})

	item := doc.Paths.Value("/ocs/v1.php/anything")
	if item.Options == nil || item.Get == nil {
		t.Fatal("a route for every method should describe OPTIONS and GET")
	}
	if item.Options.Security != nil {
		t.Errorf("OPTIONS should need no credentials, got %+v", item.Options.Security)
	}
	if item.Get.Security == nil {
		t.Error("GET should still need credentials")
	}
}

// How a deployment authenticates is the deployment's business: the document
// reports the schemes it was given, each described the way OpenAPI names it,
// and asks protected operations for any one of them.
func TestSecurityComesFromTheGivenSchemes(t *testing.T) {
	doc := renderWith(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/cloud/user", noop)
		r.Service("ocs").Get("/ocs/v1.php/cloud/capabilities", noop, router.Unprotected())
	}, []auth.Scheme{
		{Name: "appauth", Kind: auth.SchemeBasic, Description: "An app password."},
		{Name: "publicshares", Kind: auth.SchemeHeader, Param: "public-token"},
		{Name: "bearer", Kind: auth.SchemeBearer},
		{Name: "spnego", Kind: auth.SchemeNegotiate},
		{Name: "signature", Kind: auth.SchemeQuery, Param: "OC-Signature"},
	})

	type scheme struct{ Type, Scheme, In, Name, Description string }
	got := map[string]scheme{}
	for name, ref := range doc.Components.SecuritySchemes {
		s := ref.Value
		got[name] = scheme{s.Type, s.Scheme, s.In, s.Name, s.Description}
	}
	want := map[string]scheme{
		"appauth":      {Type: "http", Scheme: "basic", Description: "An app password."},
		"publicshares": {Type: "apiKey", In: "header", Name: "public-token"},
		"bearer":       {Type: "http", Scheme: "bearer"},
		"spnego":       {Type: "http", Scheme: "negotiate"},
		"signature":    {Type: "apiKey", In: "query", Name: "OC-Signature"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got security schemes %+v, want %+v", got, want)
	}

	user := doc.Paths.Value("/ocs/v1.php/cloud/user")
	if user == nil || user.Get == nil {
		t.Fatal("the user endpoint is missing")
	}
	wantReq := openapi3.SecurityRequirements{
		{"appauth": {}}, {"publicshares": {}}, {"bearer": {}}, {"spnego": {}}, {"signature": {}},
	}
	if got := user.Get.Security; got == nil || !reflect.DeepEqual(*got, wantReq) {
		t.Errorf("got requirement %+v, want any one of the schemes in order: %+v", got, wantReq)
	}

	caps := doc.Paths.Value("/ocs/v1.php/cloud/capabilities")
	if caps.Get.Security != nil {
		t.Errorf("an unprotected route should need no credentials, got %+v", caps.Get.Security)
	}
}

// A deployment whose authentication could not be described is not given a
// made-up one.
func TestNoSchemesLeavesSecurityOut(t *testing.T) {
	doc := renderWith(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/cloud/user", noop)
	}, nil)

	if doc.Components.SecuritySchemes != nil {
		t.Errorf("got security schemes %+v, want none", doc.Components.SecuritySchemes)
	}
	if got := doc.Paths.Value("/ocs/v1.php/cloud/user").Get.Security; got != nil {
		t.Errorf("got requirement %+v, want none", got)
	}
}

// The document is written in the version it claims, and reads back as one.
func TestDocumentRoundTrips(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/config", noop, router.Unprotected())
		r.Service("ocdav").Mount("/remote.php/dav", noop, router.Methods("PROPFIND"))
	})

	back := validate(t, doc)
	if back.OpenAPI != Version {
		t.Errorf("got version %v, expected %s", back.OpenAPI, Version)
	}
	if back.Paths.Value("/remote.php/dav/{path}").AdditionalOperations["PROPFIND"] == nil {
		t.Error("PROPFIND did not survive the round trip")
	}
}

// opaqueTime writes its own JSON from a field reflection cannot see, as the
// libregraph Nullable types do.
type opaqueTime struct{ value string }

func (o opaqueTime) MarshalJSON() ([]byte, error) { return json.Marshal(o.value) }

type withOpaque struct {
	At opaqueTime `json:"at"`
}

// A type that writes its own JSON still gets a component, so that the
// reference to it resolves.
func TestOpaqueTypesResolve(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/opaque", noop, router.Describe(router.Doc{
			Responses: map[int]any{200: withOpaque{}},
		}))
	})

	if _, ok := doc.Components.Schemas[schemaPrefix+"opaqueTime"]; !ok {
		t.Errorf("the opaque type has no component: %v", keys(doc.Components.Schemas))
	}
}
