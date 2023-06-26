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

import "reflect"

func dumpStruct(v reflect.Value) map[string]any {
	if v.Kind() != reflect.Struct {
		panic("called dumpStruct on non struct type")
	}

	n := v.NumField()
	m := make(map[string]any, n)

	t := v.Type()
	for i := 0; i < n; i++ {
		e := v.Field(i)
		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		if isFieldSquashed(f) {
			if e.Kind() == reflect.Pointer {
				e = e.Elem()
			}

			var mm map[string]any
			if e.Kind() == reflect.Struct {
				mm = dumpStruct(e)
			} else if e.Kind() == reflect.Map {
				mm = dumpMap(e)
			} else {
				panic("squash not allowed on non map/struct types")
			}
			for k, v := range mm {
				m[k] = v
			}
			continue
		}

		m[fieldName(f)] = dumpByType(e)
	}
	return m
}

func fieldName(f reflect.StructField) string {
	fromtag := f.Tag.Get("key")
	if fromtag != "" {
		return fromtag
	}
	return f.Name
}

func isFieldSquashed(f reflect.StructField) bool {
	tag := f.Tag.Get("key")
	return tag != "" && tag[1:] == "squash"
}

func dumpMap(v reflect.Value) map[string]any {
	if v.Kind() != reflect.Map {
		panic("called dumpMap on non map type")
	}

	m := make(map[string]any, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		e := iter.Value()

		key, ok := k.Interface().(string)
		if !ok {
			panic("key map must be a string")
		}

		m[key] = dumpByType(e)
	}
	return m
}

func dumpList(v reflect.Value) []any {
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		panic("called dumpList on non array/slice type")
	}

	n := v.Len()
	l := make([]any, 0, n)

	for i := 0; i < n; i++ {
		e := v.Index(i)
		l = append(l, dumpByType(e))
	}
	return l
}

func dumpPrimitive(v reflect.Value) any {
	if v.Kind() != reflect.Bool && v.Kind() != reflect.Int && v.Kind() != reflect.Int8 &&
		v.Kind() != reflect.Int16 && v.Kind() != reflect.Int32 && v.Kind() != reflect.Int64 &&
		v.Kind() != reflect.Uint && v.Kind() != reflect.Uint8 && v.Kind() != reflect.Uint16 &&
		v.Kind() != reflect.Uint32 && v.Kind() != reflect.Uint64 && v.Kind() != reflect.Float32 &&
		v.Kind() != reflect.Float64 && v.Kind() != reflect.String {
		panic("called dumpPrimitive on non primitive type: " + v.Kind().String())
	}
	return v.Interface()
}

func dumpByType(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return dumpPrimitive(v)
	case reflect.Array, reflect.Slice:
		return dumpList(v)
	case reflect.Struct:
		return dumpStruct(v)
	case reflect.Map:
		return dumpMap(v)
	case reflect.Interface, reflect.Pointer:
		return dumpByType(v.Elem())
	}
	panic("type not supported: " + v.Kind().String())
}
