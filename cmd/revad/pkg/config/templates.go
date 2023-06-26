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
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

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

func applyTemplateByType(l Lookuper, p setter, v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		return applyTemplateString(l, p, v)
	case reflect.Array, reflect.Slice:
		return applyTemplateList(l, p, v)
	case reflect.Struct:
		return applyTemplateStruct(l, p, v)
	case reflect.Map:
		return applyTemplateMap(l, p, v)
	case reflect.Interface:
		return applyTemplateInterface(l, p, v)
	case reflect.Pointer:
		return applyTemplateByType(l, p, v.Elem())
	}
	return nil
}

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

func applyTemplateInterface(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.Interface {
		panic("called applyTemplateInterface on non interface value")
	}

	s, ok := v.Interface().(string)
	if !ok {
		return applyTemplateByType(l, p, v.Elem())
	}

	if !isTemplate(s) {
		// nothing to do
		return nil
	}

	key := keyFromTemplate(s)
	val, err := l.Lookup(key)
	if err != nil {
		return err
	}

	p.SetValue(val)
	return nil
}

func applyTemplateString(l Lookuper, p setter, v reflect.Value) error {
	if v.Kind() != reflect.String {
		panic("called applyTemplateString on non string type")
	}

	s := v.Interface().(string)
	if !isTemplate(s) {
		// nothing to do
		return nil
	}

	if !v.CanSet() {
		panic("value is not addressable")
	}

	key := keyFromTemplate(s)
	val, err := l.Lookup(key)
	if err != nil {
		return err
	}

	str, ok := val.(string)
	if ok {
		p.SetValue(str)
		return nil
	}

	return errors.New("value cannot be set on a non string type")
}

func isTemplate(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}")
}

func keyFromTemplate(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{{")
	s = strings.TrimSuffix(s, "}}")
	return "." + strings.TrimSpace(s)
}

type setter interface {
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

func (s setterList) SetValue(v any) {
	el := s.List.Index(s.Index)
	el.Set(reflect.ValueOf(v))
}

func (s setterMap) SetValue(v any) {
	s.Map.SetMapIndex(reflect.ValueOf(s.Key), reflect.ValueOf(v))
}

func (s setterStruct) SetValue(v any) {
	s.Struct.Field(s.Field).Set(reflect.ValueOf(v))
}
