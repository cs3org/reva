// Copyright 2018-2023 CERN
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

package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// applyTemplateStruct applies recursively to all its fields all the template
// strings to the struct v.
// It panics if the value is not a struct.
// A field in the struct is skipped for applying all the templates
// if a tag "template" has the value "-".
func applyTemplateStruct(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.Struct {
		panic("called applyTemplateStruct on non struct type")
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		el := v.Field(i)
		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		if f.Tag.Get("template") == "-" {
			// skip this field
			continue
		}

		if err := applyTemplateByType(l, setterStruct{Struct: v, Field: i}, el); err != nil {
			return err
		}
	}
	return nil
}

// applyTemplateByType applies the template string to a generic type.
func applyTemplateByType(l Lookuper, p setter, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		return applyTemplateList(l, p, v)
	case reflect.Struct:
		return applyTemplateStruct(l, p, v)
	case reflect.Map:
		return applyTemplateMap(l, p, v)
	case reflect.Interface:
		return applyTemplateInterface(l, p, v)
	case reflect.String:
		return applyTemplateString(l, p, v)
	case reflect.Pointer:
		return applyTemplateByType(l, p, v.Elem())
	}
	return nil
}

// applyTemplateList recursively applies in all the elements of the list
// the template strings.
// It panics if the given value is not a list.
func applyTemplateList(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		panic("called applyTemplateList on non array/slice type")
	}

	for i := 0; i < v.Len(); i++ {
		el := v.Index(i)
		if err := applyTemplateByType(l, setterList{List: v, Index: i}, el); err != nil {
			return err
		}
	}
	return nil
}

// applyTemplateMap recursively applies in all the elements of the map
// the template strings.
// It panics if the given value is not a map.
func applyTemplateMap(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.Map {
		panic("called applyTemplateMap on non map type")
	}

	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		el := v.MapIndex(k)
		if err := applyTemplateByType(l, setterMap{Map: v, Key: k.Interface()}, el); err != nil {
			return err
		}
	}
	return nil
}

// applyTemplateString applies to the string the template string, if any.
// It panics if the given value is not a string.
func applyTemplateString(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.String {
		panic("called applyTemplateString on non string type")
	}
	s := v.String()
	tmpl, is := isTemplate(s)
	if !is {
		// nothing to do
		return nil
	}

	key := keyFromTemplate(tmpl)
	val, err := l.Lookup(key)
	if err != nil {
		return err
	}

	new, err := replaceTemplate(s, tmpl, val)
	if err != nil {
		return err
	}
	str, ok := convertToString(new)
	if !ok {
		return fmt.Errorf("value %v cannot be converted as string in the template %s", val, new)
	}

	p.SetValue(str)
	return nil
}

// applyTemplateInterface applies to the interface the template string, if any.
// It panics if the given value is not an interface.
func applyTemplateInterface(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.Interface {
		panic("called applyTemplateInterface on non interface value")
	}

	s, ok := v.Interface().(string)
	if !ok {
		return applyTemplateByType(l, p, v.Elem())
	}

	tmpl, is := isTemplate(s)
	if !is {
		// nothing to do
		return nil
	}

	key := keyFromTemplate(tmpl)
	val, err := l.Lookup(key)
	if err != nil {
		return err
	}

	new, err := replaceTemplate(s, tmpl, val)
	if err != nil {
		return err
	}
	p.SetValue(new)
	return nil
}

func replaceTemplate(original, tmpl string, val any) (any, error) {
	if strings.TrimSpace(original) == tmpl {
		// the value was directly a template, i.e. "{{ grpc.services.gateway.address }}"
		return val, nil
	}
	// the value is of something like "something {{ template }} something else"
	// in this case we need to replace the template string with the value, converted
	// as string in the original val
	s, ok := convertToString(val)
	if !ok {
		return nil, fmt.Errorf("value %v cannot be converted as string in the template %s", val, original)
	}
	return strings.Replace(original, tmpl, s, 1), nil
}

func convertToString(val any) (string, bool) {
	switch v := val.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case bool:
		return strconv.FormatBool(v), true
	}
	return "", false
}

var templateRegex = regexp.MustCompile("{{.{1,}}}")

func isTemplate(s string) (string, bool) {
	m := templateRegex.FindString(s)
	return m, m != ""
}

func keyFromTemplate(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{{")
	s = strings.TrimSuffix(s, "}}")
	return "." + strings.TrimSpace(s)
}

type setter interface {
	// SetValue sets the value v in a container.
	SetValue(v any)
}

type setterList struct {
	List  reflect.Value
	Index int
}

type setterMap struct {
	Map reflect.Value
	Key any
}

type setterStruct struct {
	Struct reflect.Value
	Field  int
}

// SetValue sets the value v in the <index> element
// of the list.
func (s setterList) SetValue(v any) {
	el := s.List.Index(s.Index)
	el.Set(reflect.ValueOf(v))
}

// SetValue sets the value v to the <key> element of the map.
func (s setterMap) SetValue(v any) {
	s.Map.SetMapIndex(reflect.ValueOf(s.Key), reflect.ValueOf(v))
}

// SetValue sets the value v to the field in the struct.
func (s setterStruct) SetValue(v any) {
	s.Struct.Field(s.Field).Set(reflect.ValueOf(v))
}
