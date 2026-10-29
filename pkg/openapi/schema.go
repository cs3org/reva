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
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
)

// schemas turns Go types into schemas, keeping every named struct in the
// document's components, so a type used twice is described once and a type
// that refers to itself terminates.
type schemas struct {
	gen        *openapi3gen.Generator
	components openapi3.Schemas
}

func newSchemas(components openapi3.Schemas) *schemas {
	return &schemas{
		components: components,
		gen: openapi3gen.NewGenerator(
			openapi3gen.CreateComponentSchemas(openapi3gen.ExportComponentSchemasOptions{
				ExportComponentSchemas: true,
				ExportTopLevelSchema:   true,
			}),
			openapi3gen.CreateTypeNameGenerator(schemaName),
			openapi3gen.SchemaCustomizer(absentIsNotNull),
		),
	}
}

// of returns the schema describing v's type.
func (s *schemas) of(v any) (*openapi3.SchemaRef, error) {
	ref, err := s.gen.NewSchemaRefForValue(v, s.components)
	if err != nil {
		return nil, err
	}
	s.addOpaque(ref)
	return ref, nil
}

const componentsPrefix = "#/components/schemas/"

// addOpaque adds a component for each struct that is referred to but has no
// JSON fields, such as libregraph's NullableTime. Such a type writes its own
// JSON, which reflection cannot see, and the generator leaves its reference
// dangling.
func (s *schemas) addOpaque(root *openapi3.SchemaRef) {
	missing := map[string]bool{}
	seen := map[*openapi3.Schema]bool{}
	var walk func(*openapi3.SchemaRef)
	walk = func(r *openapi3.SchemaRef) {
		if r == nil {
			return
		}
		if name, ok := strings.CutPrefix(r.Ref, componentsPrefix); ok {
			if _, have := s.components[name]; !have {
				missing[name] = true
			}
			// A component is walked from the components themselves.
			return
		}
		v := r.Value
		if v == nil || seen[v] {
			return
		}
		seen[v] = true
		for _, p := range v.Properties {
			walk(p)
		}
		walk(v.Items)
		walk(v.AdditionalProperties.Schema)
		walk(v.Not)
		for _, group := range []openapi3.SchemaRefs{v.AllOf, v.AnyOf, v.OneOf} {
			for _, sub := range group {
				walk(sub)
			}
		}
	}

	walk(root)
	for _, c := range s.components {
		walk(c)
	}
	for name := range missing {
		s.components[name] = &openapi3.SchemaRef{Value: &openapi3.Schema{
			Description: "Not described: the type writes its own JSON.",
		}}
	}
}

// schemaName names a struct's component after its full import path. The
// documents of several services are merged into one, each reflected on its
// own, so a name has to mean the same type whichever service produced it: a
// name made unique within one document only is not enough.
func schemaName(t reflect.Type) string {
	if t.Name() == "" {
		// Anonymous structs are inlined, which an empty name asks for.
		return ""
	}
	if t.PkgPath() == "" {
		return t.Name()
	}
	return strings.ReplaceAll(t.PkgPath(), "/", ".") + "." + t.Name()
}

// absentIsNotNull drops the nullable flag the generator puts on every pointer.
// A pointer field is how a Go type says a value may be left out, which its
// omitempty tag already tells, not that it may be sent as null. The flag is
// also an OpenAPI 3.0 keyword that 3.1 and later no longer know.
func absentIsNotNull(_ string, _ reflect.Type, _ reflect.StructTag, schema *openapi3.Schema) error {
	schema.Nullable = false
	return nil
}
