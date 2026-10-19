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

package invoke

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// argField is one struct field that holds an argument.
type argField struct {
	spec  ArgSpec
	index int
}

var (
	durationType        = reflect.TypeFor[time.Duration]()
	timeType            = reflect.TypeFor[time.Time]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// argFields reads the argument fields of the struct A: the fields with an `arg`
// tag. It panics on a type it cannot parse, so that a wrong declaration fails
// when the service starts and not at the first call.
func argFields[A any]() []argField {
	t := reflect.TypeFor[A]()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("invoke: arguments must be a struct, not %s", t))
	}
	var fields []argField
	for i := range t.NumField() {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("arg")
		if !ok {
			continue
		}
		name, opt, _ := strings.Cut(tag, ",")
		if !parsable(f.Type) {
			panic(fmt.Sprintf("invoke: argument %q: unsupported type %s", name, f.Type))
		}
		fields = append(fields, argField{
			spec:  ArgSpec{Name: name, Description: f.Tag.Get("help"), Required: opt == "required"},
			index: i,
		})
	}
	return fields
}

// parsable reports whether an argument can have type t. A pointer is nil when
// the argument is absent, and a slice takes comma-separated values.
func parsable(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return scalar(t) || t.Kind() == reflect.Slice && scalar(t.Elem())
}

// scalar reports whether one value of type t can be parsed from a string.
func scalar(t reflect.Type) bool {
	// Check before the kinds: a named type can parse itself, and
	// time.Duration is an int64.
	if t == durationType || t == timeType || reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// specsOf returns the catalog entries of the argument fields.
func specsOf(fields []argField) []ArgSpec {
	specs := make([]ArgSpec, 0, len(fields))
	for _, f := range fields {
		specs = append(specs, f.spec)
	}
	return specs
}

// bind fills an A from the wire arguments: it checks that the required ones
// are present and parses each one to the type of its field.
func bind[A any](fields []argField, raw map[string]any) (A, error) {
	var a A
	v := reflect.ValueOf(&a).Elem()
	for _, f := range fields {
		r, ok := raw[f.spec.Name]
		if !ok {
			if f.spec.Required {
				return a, fmt.Errorf("invoke: missing argument %q", f.spec.Name)
			}
			continue
		}
		fv := v.Field(f.index)
		t := fv.Type()
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		val, err := parseArg(t, r)
		if err != nil {
			return a, fmt.Errorf("invoke: argument %q: %w", f.spec.Name, err)
		}
		if fv.Kind() == reflect.Pointer {
			p := reflect.New(t)
			p.Elem().Set(val)
			val = p
		}
		fv.Set(val)
	}
	return a, nil
}

// parseArg converts a wire value, which is a string, to type t. A value that
// already has type t, from an in-process caller, is kept.
func parseArg(t reflect.Type, raw any) (reflect.Value, error) {
	if raw != nil && reflect.TypeOf(raw).AssignableTo(t) {
		return reflect.ValueOf(raw), nil
	}
	s, ok := raw.(string)
	if !ok {
		return reflect.Value{}, fmt.Errorf("want %s, got %T", t, raw)
	}
	if scalar(t) {
		return parseScalar(t, s)
	}
	out := reflect.MakeSlice(t, 0, 0)
	for e := range strings.SplitSeq(s, ",") {
		if e = strings.TrimSpace(e); e == "" {
			continue
		}
		v, err := parseScalar(t.Elem(), e)
		if err != nil {
			return reflect.Value{}, err
		}
		out = reflect.Append(out, v)
	}
	return out, nil
}

// parseScalar converts one string to type t.
func parseScalar(t reflect.Type, s string) (reflect.Value, error) {
	var v any
	var err error
	switch {
	case t == durationType:
		v, err = time.ParseDuration(s)
	case t == timeType:
		if v, err = parseTime(s); err != nil {
			return reflect.Value{}, err
		}
	case reflect.PointerTo(t).Implements(textUnmarshalerType):
		p := reflect.New(t)
		if err := p.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
			return reflect.Value{}, fmt.Errorf("%q: %w", s, err)
		}
		return p.Elem(), nil
	default:
		switch t.Kind() {
		case reflect.String:
			v = s
		case reflect.Bool:
			v, err = strconv.ParseBool(s)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v, err = strconv.ParseInt(s, 10, t.Bits())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v, err = strconv.ParseUint(s, 10, t.Bits())
		case reflect.Float32, reflect.Float64:
			v, err = strconv.ParseFloat(s, t.Bits())
		}
	}
	if err != nil {
		return reflect.Value{}, fmt.Errorf("%q is not a valid %s", s, t)
	}
	// Convert lets a field have a named type, e.g. a run id over string.
	return reflect.ValueOf(v).Convert(t), nil
}

// parseTime accepts an RFC3339 time, or a duration that means that long ago.
func parseTime(s string) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not an RFC3339 time or a duration (e.g. 5m)", s)
}
