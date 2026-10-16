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
	"strings"

	"github.com/invopop/jsonschema"
)

// Schema is a JSON Schema. OpenAPI 3.1 embeds JSON Schema as it is, so the
// schemas reflected out of Go types need no translation.
type Schema = jsonschema.Schema

// componentsRef is where an OpenAPI document keeps its schemas, which is not
// where JSON Schema keeps its definitions.
const componentsRef = "#/components/schemas/"

const defsRef = "#/$defs/"

// reflector turns Go types into schemas, collecting the named types it meets
// so that a type used twice is described once and a type that refers to itself
// terminates.
type reflector struct {
	r    *jsonschema.Reflector
	defs map[string]*Schema
}

func newReflector() *reflector {
	return &reflector{
		r: &jsonschema.Reflector{
			// Without this every schema carries an $id naming invopop.com.
			Anonymous: true,
			// Describe a field as absent when it may be, rather than
			// declaring every field required.
			RequiredFromJSONSchemaTags: false,
			ExpandedStruct:             false,
		},
		defs: map[string]*Schema{},
	}
}

// schemaFor returns the schema describing v's type, or nil for an untyped nil,
// which is how a response without a body is spelled.
func (f *reflector) schemaFor(v any) *Schema {
	if v == nil {
		return nil
	}

	s := f.r.Reflect(v)
	if s == nil {
		return nil
	}

	// The reflected schema holds its definitions; an OpenAPI document holds
	// them centrally, so they are moved over and the references repointed.
	for name, def := range s.Definitions {
		repoint(def)
		f.defs[name] = def
	}
	s.Definitions = nil
	repoint(s)

	// A struct comes back as a reference to its own definition, which is what
	// should be used wherever the type appears.
	if s.Ref != "" {
		return &Schema{Ref: s.Ref}
	}
	return s
}

// repoint rewrites every reference in a schema from where JSON Schema keeps
// its definitions to where an OpenAPI document keeps its schemas.
func repoint(s *Schema) {
	if s == nil {
		return
	}

	s.Ref = strings.Replace(s.Ref, defsRef, componentsRef, 1)

	repoint(s.Items)
	repoint(s.AdditionalProperties)
	repoint(s.Contains)
	repoint(s.Not)
	repoint(s.If)
	repoint(s.Then)
	repoint(s.Else)

	if s.Properties != nil {
		for pair := s.Properties.Oldest(); pair != nil; pair = pair.Next() {
			repoint(pair.Value)
		}
	}
	for _, group := range [][]*Schema{s.AllOf, s.AnyOf, s.OneOf, s.PrefixItems} {
		for _, sub := range group {
			repoint(sub)
		}
	}
	for _, sub := range s.PatternProperties {
		repoint(sub)
	}
	for _, sub := range s.Definitions {
		repoint(sub)
	}
}

// stringSchema describes a parameter, all of which arrive as text.
func stringSchema() *Schema { return &Schema{Type: "string"} }
