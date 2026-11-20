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
	"reflect"
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

// newReflector returns a reflector for values, which are all the values the
// document describes the types of.
//
// It needs every one of them up front to name the schemas. A type is named
// after itself, unless a type of another package has the same name: then both
// are also named after their package, so that neither replaces the other in
// the components and every reference still finds the type it means. Deciding
// this from the whole set, not from the order types are met in, keeps the
// names the same from one start to the next.
func newReflector(values []any) *reflector {
	byName := map[string]map[reflect.Type]bool{}
	survey := jsonReflector(func(t reflect.Type) string {
		if byName[t.Name()] == nil {
			byName[t.Name()] = map[reflect.Type]bool{}
		}
		byName[t.Name()][t] = true
		// Distinct names, so that the survey reaches into every type rather
		// than stopping at one it takes for a type it has already seen.
		return qualifiedName(t)
	})
	for _, v := range values {
		if v != nil {
			survey.Reflect(v)
		}
	}

	return &reflector{
		r: jsonReflector(func(t reflect.Type) string {
			if len(byName[t.Name()]) > 1 {
				return qualifiedName(t)
			}
			return t.Name()
		}),
		defs: map[string]*Schema{},
	}
}

func jsonReflector(namer func(reflect.Type) string) *jsonschema.Reflector {
	// A field is required unless it is tagged omitempty, since encoding/json
	// then always writes it. That is the library's default.
	return &jsonschema.Reflector{
		// Without this every schema carries an $id naming invopop.com.
		Anonymous: true,
		// A server that adds a field to a reply has not broken its clients,
		// so a client that validates the reply should not reject it.
		AllowAdditionalProperties: true,
		Mapper:                    nullable,
		Namer:                     namer,
	}
}

// qualifiedName names a type after its package path as well, in the
// characters a component name may hold.
func qualifiedName(t reflect.Type) string {
	if t.Name() == "" {
		return ""
	}
	return strings.ReplaceAll(t.PkgPath(), "/", ".") + "." + t.Name()
}

var jsonMarshaler = reflect.TypeFor[json.Marshaler]()

// nullable describes the wrappers openapi-generator emits for a field that may
// be null, such as libregraph's NullableTime. They marshal as the value they
// hold or as null, but keep that value in unexported fields, so reflecting
// their struct would describe an empty object.
func nullable(t reflect.Type) *Schema {
	if !strings.HasPrefix(t.Name(), "Nullable") || !t.Implements(jsonMarshaler) {
		return nil
	}
	get, ok := t.MethodByName("Get")
	if !ok || get.Type.NumIn() != 1 || get.Type.NumOut() != 1 || get.Type.Out(0).Kind() != reflect.Pointer {
		return nil
	}
	if _, ok := t.MethodByName("IsSet"); !ok {
		return nil
	}

	value := (&jsonschema.Reflector{
		Anonymous:                 true,
		DoNotReference:            true,
		AllowAdditionalProperties: true,
		Mapper:                    nullable,
	}).ReflectFromType(get.Type.Out(0).Elem())
	value.Version = ""
	return &Schema{AnyOf: []*Schema{value, {Type: "null"}}}
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
	// A schema inside the document takes the dialect the document declares.
	s.Version = ""

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
