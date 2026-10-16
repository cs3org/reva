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
//
// WebDAV is the one part that does not fit. OpenAPI names its operations in a
// fixed set of fields, and PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, REPORT,
// LOCK and UNLOCK are not among them, so those are reported on the path item
// as an extension rather than silently dropped.
package openapi

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// Version of the OpenAPI specification the document conforms to. 3.1 embeds
// JSON Schema as-is, which is what the reflected schemas already are.
const Version = "3.1.0"

// Document is an OpenAPI document.
type Document struct {
	OpenAPI    string               `json:"openapi"`
	Info       Info                 `json:"info"`
	Servers    []Server             `json:"servers,omitempty"`
	Tags       []Tag                `json:"tags,omitempty"`
	Paths      map[string]*PathItem `json:"paths"`
	Components Components           `json:"components"`
}

type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type        string `json:"type"`
	Scheme      string `json:"scheme,omitempty"`
	In          string `json:"in,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// PathItem holds the operations of one path.
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Trace   *Operation `json:"trace,omitempty"`
	// Parameters shared by every operation of the path.
	Parameters []*Parameter `json:"parameters,omitempty"`
	// WebDAVMethods are the methods the path answers that OpenAPI has no field
	// for. They are reported so that nothing a service declared is lost.
	WebDAVMethods []string `json:"x-webdav-methods,omitempty"`
}

type Operation struct {
	OperationID string                `json:"operationId,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Parameters  []*Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
	Example     string  `json:"example,omitempty"`
}

type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

const jsonMedia = "application/json"

// operationField maps a method to the PathItem field OpenAPI names it with.
var operationField = map[string]func(*PathItem) **Operation{
	http.MethodGet:     func(p *PathItem) **Operation { return &p.Get },
	http.MethodPut:     func(p *PathItem) **Operation { return &p.Put },
	http.MethodPost:    func(p *PathItem) **Operation { return &p.Post },
	http.MethodDelete:  func(p *PathItem) **Operation { return &p.Delete },
	http.MethodOptions: func(p *PathItem) **Operation { return &p.Options },
	http.MethodHead:    func(p *PathItem) **Operation { return &p.Head },
	http.MethodPatch:   func(p *PathItem) **Operation { return &p.Patch },
	http.MethodTrace:   func(p *PathItem) **Operation { return &p.Trace },
}

// From builds the document describing the given routes, authenticated by the
// given schemes.
//
// The schemes are the deployment's own, resolved from how its authentication
// is configured, so that what the document tells a client to send is what the
// server reads. Passing none leaves the document silent about authentication
// rather than guessing at it.
func From(routes []router.Route, info Info, servers []Server, schemes []auth.Scheme) *Document {
	f := newReflector()
	doc := &Document{
		OpenAPI:    Version,
		Info:       info,
		Servers:    servers,
		Paths:      map[string]*PathItem{},
		Components: Components{SecuritySchemes: securitySchemes(schemes)},
	}
	security := requirement(schemes)

	owners := map[string]bool{}
	for _, rt := range routes {
		owners[rt.Owner] = true
		path, params := specPath(rt)

		item, ok := doc.Paths[path]
		if !ok {
			item = &PathItem{Parameters: params}
			doc.Paths[path] = item
		}

		// A route matching every method describes every operation the path
		// has; OpenAPI has no way to say that, so it is reported as such.
		methods := []string{rt.Method}
		if rt.Method == "" {
			methods = anyMethods
		}

		for _, m := range methods {
			field, known := operationField[m]
			if !known {
				if !slices.Contains(item.WebDAVMethods, m) {
					item.WebDAVMethods = append(item.WebDAVMethods, m)
				}
				continue
			}
			if *field(item) != nil {
				// Two routes claiming the same path and method cannot happen:
				// the router refuses it. Keep the first all the same.
				continue
			}
			*field(item) = operationFor(f, rt, m, path, security)
		}
	}

	for _, item := range doc.Paths {
		sort.Strings(item.WebDAVMethods)
	}

	doc.Components.Schemas = f.defs
	for name := range owners {
		if name != "" {
			doc.Tags = append(doc.Tags, Tag{Name: name})
		}
	}
	sort.Slice(doc.Tags, func(i, j int) bool { return doc.Tags[i].Name < doc.Tags[j].Name })

	return doc
}

// anyMethods are the operations a route that matches every method is taken to
// describe.
var anyMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

func operationFor(f *reflector, rt router.Route, method, path string, security []map[string][]string) *Operation {
	op := &Operation{
		OperationID: operationID(rt.Owner, method, path),
		Responses:   map[string]*Response{},
	}
	if rt.Owner != "" {
		op.Tags = []string{rt.Owner}
	}
	if !rt.Unprotected {
		op.Security = security
	}

	if rt.Subtree {
		op.Description = "Everything below this path is served by " + rt.Owner +
			", which addresses the resource by the remainder of the path."
	}

	if rt.Doc == nil {
		// Without a description the operation still says who serves it and
		// what the path names; the rest is simply not known here.
		op.Responses["default"] = &Response{Description: "Undocumented."}
		return op
	}

	op.Summary = rt.Doc.Summary
	if rt.Doc.Description != "" {
		op.Description = rt.Doc.Description
	}

	for _, p := range rt.Doc.Params {
		op.Parameters = append(op.Parameters, &Parameter{
			Name:        p.Name,
			In:          p.In,
			Description: p.Description,
			Required:    p.Required,
			Example:     p.Example,
			Schema:      stringSchema(),
		})
	}

	if rt.Doc.Body != nil {
		op.RequestBody = &RequestBody{
			Required: true,
			Content:  map[string]*MediaType{jsonMedia: {Schema: f.schemaFor(rt.Doc.Body)}},
		}
	}

	codes := make([]int, 0, len(rt.Doc.Responses))
	for code := range rt.Doc.Responses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		res := &Response{Description: http.StatusText(code)}
		if schema := f.schemaFor(rt.Doc.Responses[code]); schema != nil {
			res.Content = map[string]*MediaType{jsonMedia: {Schema: schema}}
		}
		op.Responses[strconv.Itoa(code)] = res
	}
	if len(op.Responses) == 0 {
		op.Responses["default"] = &Response{Description: "Undocumented."}
	}

	return op
}

// specPath turns a route's pattern into an OpenAPI path and the parameters it
// names. A mounted subtree becomes the prefix plus a catch-all parameter,
// since everything below it is the resource it addresses.
func specPath(rt router.Route) (string, []*Parameter) {
	pattern := rt.Pattern
	if rt.Subtree {
		if pattern == "/" {
			pattern = "/{path}"
		} else {
			pattern = strings.TrimSuffix(pattern, "/") + "/{path}"
		}
	}

	var params []*Parameter
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

		p := &Parameter{Name: name, In: "path", Required: true, Schema: stringSchema()}
		if greedy || name == "path" {
			p.Description = "May contain slashes: it is the whole remaining path."
		}
		params = append(params, p)

		out.WriteString(rest[:open])
		out.WriteString("{" + name + "}")
		rest = rest[close+1:]
	}

	return out.String(), params
}

func operationID(owner, method, path string) string {
	id := strings.ToLower(method) + "-" + strings.Trim(path, "/")
	id = strings.NewReplacer("/", "-", "{", "", "}", "", ".", "-").Replace(id)
	if owner == "" {
		return id
	}
	return owner + "-" + id
}
