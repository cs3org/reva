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

// Package openapi describes a reva deployment's HTTP surface as an OpenAPI
// document, built from the routes its services declared.
//
// The routes alone already carry most of what a reader wants: which paths
// exist, which methods they answer, which parameters their patterns name, and
// which of them need credentials. A route that was given a router.Doc adds the
// rest - what it accepts, what it returns, and what it is for.
package openapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/getkin/kin-openapi/openapi3"
)

// Version of the OpenAPI specification the document conforms to. 3.2 is the
// first that can name an operation for a method outside its fixed set, which
// is what the WebDAV methods (PROPFIND, MKCOL, ...) need.
const Version = "3.2.0"

const jsonMedia = "application/json"

// From builds the document describing the given routes, authenticated by the
// given schemes.
//
// The schemes are the deployment's own, resolved from how its authentication
// is configured, so that what the document tells a client to send is what the
// server reads. Passing none leaves the document silent about authentication
// rather than guessing at it.
func From(routes []router.Route, info *openapi3.Info, schemes []auth.Scheme) (*openapi3.T, error) {
	doc := &openapi3.T{
		OpenAPI: Version,
		Info:    info,
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas:         openapi3.Schemas{},
			SecuritySchemes: securitySchemes(schemes),
		},
	}
	b := &builder{
		schemas:  newSchemas(doc.Components.Schemas),
		security: requirement(schemes),
		ids:      map[string]bool{},
	}

	owners := map[string]bool{}
	for _, rt := range routes {
		owners[rt.Owner] = true
		path, params := specPath(rt)

		item := doc.Paths.Value(path)
		if item == nil {
			item = &openapi3.PathItem{Parameters: params}
			doc.Paths.Set(path, item)
		}

		// A route matching every method describes every operation the path
		// has; OpenAPI has no way to say that, so it is reported as such.
		methods := []string{rt.Method}
		if rt.Method == "" {
			methods = anyMethods
		}

		for _, m := range methods {
			if item.GetOperation(m) != nil {
				// Two routes claiming the same path and method cannot happen:
				// the router refuses it. Keep the first all the same.
				continue
			}
			op, err := b.operation(rt, m, path)
			if err != nil {
				return nil, fmt.Errorf("openapi: %s %s: %w", m, rt.Pattern, err)
			}
			item.SetOperation(m, op)
		}
	}

	for name := range owners {
		if name != "" {
			doc.Tags = append(doc.Tags, &openapi3.Tag{Name: name})
		}
	}
	sort.Slice(doc.Tags, func(i, j int) bool { return doc.Tags[i].Name < doc.Tags[j].Name })

	return doc, nil
}

// anyMethods are the operations a route that matches every method is taken to
// describe.
var anyMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

type builder struct {
	schemas  *schemas
	security *openapi3.SecurityRequirements
	ids      map[string]bool
}

func (b *builder) operation(rt router.Route, method, path string) (*openapi3.Operation, error) {
	op := openapi3.NewOperation()
	op.OperationID = b.operationID(rt.Owner, method, path)
	op.Responses = openapi3.NewResponses()
	if rt.Owner != "" {
		op.Tags = []string{rt.Owner}
	}
	// The auth middleware lets every OPTIONS request through, so that a CORS
	// preflight is answered without credentials.
	if !rt.Unprotected && method != http.MethodOptions {
		op.Security = b.security
	}

	if rt.Subtree {
		op.Description = "Everything below this path is served by " + rt.Owner +
			", which addresses the resource by the remainder of the path."
	}

	if rt.Doc == nil {
		// Without a description the operation still says who serves it and
		// what the path names; the rest is simply not known here.
		op.Responses.Set("default", undocumented())
		return op, nil
	}

	op.Summary = rt.Doc.Summary
	if rt.Doc.Description != "" {
		op.Description = rt.Doc.Description
	}

	for _, p := range rt.Doc.Params {
		param := &openapi3.Parameter{
			Name:        p.Name,
			In:          p.In,
			Description: p.Description,
			Required:    p.Required,
			Schema:      openapi3.NewStringSchema().NewRef(),
		}
		if p.Example != "" {
			param.Example = p.Example
		}
		op.AddParameter(param)
	}

	if rt.Doc.Body != nil {
		ref, err := b.schemas.of(rt.Doc.Body)
		if err != nil {
			return nil, err
		}
		op.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithJSONSchemaRef(ref)}
	}

	codes := make([]int, 0, len(rt.Doc.Responses))
	for code := range rt.Doc.Responses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		res := openapi3.NewResponse().WithDescription(http.StatusText(code))
		if body := rt.Doc.Responses[code]; body != nil {
			ref, err := b.schemas.of(body)
			if err != nil {
				return nil, err
			}
			res.Content = openapi3.NewContentWithJSONSchemaRef(ref)
		}
		op.AddResponse(code, res)
	}
	if op.Responses.Len() == 0 {
		op.Responses.Set("default", undocumented())
	}

	return op, nil
}

func undocumented() *openapi3.ResponseRef {
	return &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("Undocumented.")}
}

// operationID names an operation after its owner, method and path. Two paths
// can spell the same id ("/a.b" and "/a/b"), and the second is numbered.
func (b *builder) operationID(owner, method, path string) string {
	id := strings.ToLower(method) + "-" + strings.Trim(path, "/")
	id = strings.NewReplacer("/", "-", "{", "", "}", "", ".", "-").Replace(id)
	if owner != "" {
		id = owner + "-" + id
	}
	unique := id
	for n := 2; b.ids[unique]; n++ {
		unique = id + "-" + strconv.Itoa(n)
	}
	b.ids[unique] = true
	return unique
}

// specPath turns a route's pattern into an OpenAPI path and the parameters it
// names. A mounted subtree becomes the prefix plus a catch-all parameter,
// since everything below it is the resource it addresses.
func specPath(rt router.Route) (string, openapi3.Parameters) {
	pattern := rt.Pattern
	if rt.Subtree {
		if pattern == "/" {
			pattern = "/{path}"
		} else {
			pattern = strings.TrimSuffix(pattern, "/") + "/{path}"
		}
	}

	var params openapi3.Parameters
	var out strings.Builder
	rest := pattern
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out.WriteString(rest)
			break
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			out.WriteString(rest)
			break
		}
		close += open

		name := rest[open+1 : close]
		greedy := strings.HasSuffix(name, "...")
		name = strings.TrimSuffix(name, "...")

		p := openapi3.NewPathParameter(name).WithSchema(openapi3.NewStringSchema())
		if greedy || name == "path" {
			p.Description = "May contain slashes: it is the whole remaining path."
		}
		params = append(params, &openapi3.ParameterRef{Value: p})

		out.WriteString(rest[:open])
		out.WriteString("{" + name + "}")
		rest = rest[close+1:]
	}

	return out.String(), params
}
