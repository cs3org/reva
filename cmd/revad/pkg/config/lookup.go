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

func (e ErrKeyNotFound) Error() string {
	return "key '" + e.Key + "' not found in the configuration"
}

type Getter interface {
	Get(k string) (any, error)
}

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

var typeGetter = reflect.TypeOf((*Getter)(nil)).Elem()

func lookupByType(key string, v reflect.Value) (any, error) {
	if v.Type().Implements(typeGetter) {
		return lookupGetter(key, v)
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

func lookupGetter(key string, v reflect.Value) (any, error) {
	g, ok := v.Interface().(Getter)
	if !ok {
		panic("called lookupGetter on type not implementing Getter interface")
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

	return g.Get(c.Key)
}

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
