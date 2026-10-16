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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// noop stands in for the handlers, which the document does not look at.
var noop = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func render(t *testing.T, declare func(*router.Router)) *Document {
	t.Helper()
	r := router.New()
	declare(r)
	return From(r.Routes(), Info{Title: "reva", Version: "test"}, nil)
}

// What a service declared is what the document says, without the service
// having to describe it: the path, the method, and whether it needs
// credentials are all in the route already.
func TestDocumentFromRoutesAlone(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/cloud/capabilities", noop, router.Unprotected())
		r.Service("ocs").Post("/ocs/v1.php/apps/files_sharing/api/v1/shares", noop)
	})

	caps := doc.Paths["/ocs/v1.php/cloud/capabilities"]
	if caps == nil || caps.Get == nil {
		t.Fatalf("the capabilities endpoint is missing: %+v", doc.Paths)
	}
	if len(caps.Get.Security) != 0 {
		t.Errorf("an unprotected route should need no credentials, got %+v", caps.Get.Security)
	}
	if got := caps.Get.Tags; len(got) != 1 || got[0] != "ocs" {
		t.Errorf("got tags %v, expected the owning service", got)
	}

	shares := doc.Paths["/ocs/v1.php/apps/files_sharing/api/v1/shares"]
	if shares == nil || shares.Post == nil {
		t.Fatal("the shares endpoint is missing")
	}
	if len(shares.Post.Security) == 0 {
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

	item := doc.Paths["/graph/v1beta1/drives/{spaceID}/items/{resourceID}"]
	if item == nil {
		t.Fatalf("the path is missing: %+v", doc.Paths)
	}

	var names []string
	for _, p := range item.Parameters {
		if p.In != "path" || !p.Required {
			t.Errorf("got %+v, expected a required path parameter", p)
		}
		names = append(names, p.Name)
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

	item := doc.Paths["/remote.php/dav/files/{path}"]
	if item == nil {
		t.Fatalf("the subtree is missing: %+v", doc.Paths)
	}
	if item.Get == nil {
		t.Error("the subtree's GET is missing")
	}
	if len(item.Parameters) != 1 || item.Parameters[0].Name != "path" {
		t.Errorf("got %+v, expected a single catch-all parameter", item.Parameters)
	}
	if !strings.Contains(item.Parameters[0].Description, "slashes") {
		t.Errorf("got %q, expected the parameter to say it spans segments", item.Parameters[0].Description)
	}
}

// OpenAPI has no field for the WebDAV methods, so they are reported rather
// than dropped: a reader should still learn that the path answers them.
func TestWebDAVMethodsAreReported(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocdav").Mount("/remote.php/dav/files", nil,
			router.Methods("PROPFIND", "MKCOL", http.MethodGet))
	})

	item := doc.Paths["/remote.php/dav/files/{path}"]
	if item == nil {
		t.Fatal("the subtree is missing")
	}
	if item.Get == nil {
		t.Error("GET should still be an operation")
	}
	if strings.Join(item.WebDAVMethods, ",") != "MKCOL,PROPFIND" {
		t.Errorf("got %v, expected the methods OpenAPI cannot name, sorted", item.WebDAVMethods)
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

	op := doc.Paths["/ocs/v1.php/shares"].Post
	if op.Summary != "Create a share" {
		t.Errorf("got summary %q", op.Summary)
	}

	if len(op.Parameters) != 2 {
		t.Fatalf("got %d parameters, expected 2", len(op.Parameters))
	}
	if op.Parameters[0].Name != "format" || !op.Parameters[0].Required || op.Parameters[0].Example != "json" {
		t.Errorf("got %+v, expected the required query parameter with its example", op.Parameters[0])
	}
	if op.Parameters[1].Required {
		t.Error("the second parameter is optional")
	}

	if op.RequestBody == nil || op.RequestBody.Content[jsonMedia] == nil {
		t.Fatal("the request body is missing")
	}

	ok := op.Responses["200"]
	if ok == nil || ok.Content[jsonMedia] == nil {
		t.Fatal("the 200 response is missing a body")
	}
	// A status that returns nothing says so, rather than describing a body.
	bad := op.Responses["400"]
	if bad == nil {
		t.Fatal("the 400 response is missing")
	}
	if bad.Content != nil {
		t.Errorf("got %+v, expected no body for a status that returns none", bad.Content)
	}

	// Every schema lives centrally and is referred to, so a type used twice is
	// described once.
	for _, name := range []string{"shareRequest", "shareResponse", "shareOwner", "shareLink"} {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("schema %q is missing from the components", name)
		}
	}
}

// The references the reflected schemas carry have to point at where an
// OpenAPI document keeps its schemas, or every one of them dangles.
func TestSchemaReferencesPointIntoComponents(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Post("/ocs/v1.php/shares", noop, router.Describe(router.Doc{
			Responses: map[int]any{200: shareResponse{}},
		}))
	})

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), defsRef) {
		t.Errorf("the document still refers to %s", defsRef)
	}
	if !strings.Contains(string(encoded), componentsRef+"shareOwner") {
		t.Error("the nested type is not referred to in the components")
	}
}

func TestDocumentIsValidJSON(t *testing.T) {
	doc := render(t, func(r *router.Router) {
		r.Service("ocs").Get("/ocs/v1.php/config", noop, router.Unprotected())
		r.Service("ocdav").Mount("/remote.php/dav", noop, router.Methods("PROPFIND"))
	})

	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	var round map[string]any
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatalf("the document is not valid JSON: %v", err)
	}
	if round["openapi"] != Version {
		t.Errorf("got version %v, expected %s", round["openapi"], Version)
	}
	if _, ok := round["paths"]; !ok {
		t.Error("the document has no paths")
	}
}
