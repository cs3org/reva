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
	"io"
	"reflect"

	"github.com/pkg/errors"
)

// ErrKeyNotFound is the error returned when a key does not exist
// in the configuration.
type ErrKeyNotFound struct {
	Key string
}

// Error returns a string representation of the ErrKeyNotFound error.
func (e ErrKeyNotFound) Error() string {
	return "key '" + e.Key + "' not found in the configuration"
}

// lookupStruct recursively looks up the key in the struct v.
// It panics if the value in v is not a struct.
// Only fields are allowed to be accessed. It bails out if
// an user wants to access by index.
// The struct is traversed considering the field tags. If the tag
// "key" is not specified for a field, the field is skipped in
// the lookup. If the tag specifies "squash", the field is treated
// as squashed.
func lookupStruct(key string, v reflect.Value) (any, error) {
	if v.Kind() != reflect.Struct {
		panic("called lookupStruct on non struct type")
	}

	cmd, next, err := parseNext(key)
	if errors.Is(err, io.EOF) {
		return v.Interface(), nil
	}
	if err != nil {
		return nil, err
	}

	c, ok := cmd.(FieldByKey)
	if !ok {
		return nil, errors.New("call of index on struct type")
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		val := v.Field(i)
		field := t.Field(i)

		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("key")
		if tag == "" {
			continue
		}

		if tag[1:] == "squash" {
			if val.Kind() == reflect.Pointer {
				val = val.Elem()
			}

			var (
				v   any
				err error
			)
			switch val.Kind() {
			case reflect.Struct:
				v, err = lookupStruct(key, val)
			case reflect.Map:
				v, err = lookupMap(key, val)
			default:
				panic("squash not allowed on non map/struct types")
			}
			var e ErrKeyNotFound
			if errors.As(err, &e) {
				continue
			}
			if err != nil {
				return nil, err
			}
			return v, nil
		}

		if tag != c.Key {
			continue
		}

		return lookupByType(next, val)
	}
	return nil, ErrKeyNotFound{Key: key}
}

var typeLookuper = reflect.TypeOf((*Lookuper)(nil)).Elem()

// lookupByType recursively looks up the given key in v.
func lookupByType(key string, v reflect.Value) (any, error) {
	if v.Type().Implements(typeLookuper) {
		if v, err := lookupFromLookuper(key, v); err == nil && v != nil {
			return v, nil
		}
	}
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return lookupPrimitive(key, v)
	case reflect.Array, reflect.Slice:
		return lookupList(key, v)
	case reflect.Struct:
		return lookupStruct(key, v)
	case reflect.Map:
		return lookupMap(key, v)
	case reflect.Interface, reflect.Pointer:
		return lookupByType(key, v.Elem())
	}
	panic("type not supported: " + v.Kind().String())
}

// lookupFromLookuper looks up the key in a Lookup value.
func lookupFromLookuper(key string, v reflect.Value) (any, error) {
	g, ok := v.Interface().(Lookuper)
	if !ok {
		panic("called lookupFromLookuper on type not implementing Lookup interface")
	}

	cmd, _, err := parseNext(key)
	if errors.Is(err, io.EOF) {
		return v.Interface(), nil
	}
	if err != nil {
		return nil, err
	}

	c, ok := cmd.(FieldByKey)
	if !ok {
		return nil, errors.New("call of index on getter type")
	}

	return g.Lookup(c.Key)
}

// lookupMap recursively looks up the given key in the map v.
// It panics if the value in v is not a map.
// Works similarly to lookupStruct.
func lookupMap(key string, v reflect.Value) (any, error) {
	if v.Kind() != reflect.Map {
		panic("called lookupMap on non map type")
	}

	cmd, next, err := parseNext(key)
	if errors.Is(err, io.EOF) {
		return v.Interface(), nil
	}
	if err != nil {
		return nil, err
	}

	c, ok := cmd.(FieldByKey)
	if !ok {
		return nil, errors.New("call of index on map type")
	}

	// lookup elemen in the map
	el := v.MapIndex(reflect.ValueOf(c.Key))
	if !el.IsValid() {
		return nil, ErrKeyNotFound{Key: key}
	}

	return lookupByType(next, el)
}

// lookupList recursively looks up the given key in the list v,
// in all the elements contained in the list.
// It panics if the value v is not a list.
// The elements can be addressed in general by index, but
// access by key is only allowed if the list contains exactly
// one element.
func lookupList(key string, v reflect.Value) (any, error) {
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		panic("called lookupList on non array/slice type")
	}

	cmd, next, err := parseNext(key)
	if errors.Is(err, io.EOF) {
		return v.Interface(), nil
	}
	if err != nil {
		return nil, err
	}

	var el reflect.Value
	switch c := cmd.(type) {
	case FieldByIndex:
		if c.Index < 0 || c.Index >= v.Len() {
			return nil, errors.New("list index out of range")
		}
		el = v.Index(c.Index)
	case FieldByKey:
		// only allowed if the list contains only one element
		if v.Len() != 1 {
			return nil, errors.New("cannot access field by key on a non 1-elem list")
		}
		el = v.Index(0)
		e, err := lookupByType("."+c.Key, el)
		if err != nil {
			return nil, err
		}
		el = reflect.ValueOf(e)
	}

	return lookupByType(next, el)
}

// lookupPrimitive gets the value from v.
// If the key tries to access by field or by index the value,
// an error is returned.
func lookupPrimitive(key string, v reflect.Value) (any, error) {
	if v.Kind() != reflect.Bool && v.Kind() != reflect.Int && v.Kind() != reflect.Int8 &&
		v.Kind() != reflect.Int16 && v.Kind() != reflect.Int32 && v.Kind() != reflect.Int64 &&
		v.Kind() != reflect.Uint && v.Kind() != reflect.Uint8 && v.Kind() != reflect.Uint16 &&
		v.Kind() != reflect.Uint32 && v.Kind() != reflect.Uint64 && v.Kind() != reflect.Float32 &&
		v.Kind() != reflect.Float64 && v.Kind() != reflect.String {
		panic("called lookupPrimitive on non primitive type: " + v.Kind().String())
	}

	_, _, err := parseNext(key)
	if errors.Is(err, io.EOF) {
		return v.Interface(), nil
	}
	if err != nil {
		return nil, err
	}

	return nil, errors.New("cannot address a value of type " + v.Kind().String())
}
