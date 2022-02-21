package indexer

import (
	"errors"
	"path"
	"reflect"
	"strconv"
	"strings"
)

func getType(v interface{}) (reflect.Value, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return reflect.Value{}, errors.New("failed to read value via reflection")
	}

	return rv, nil
}

func getTypeFQN(t interface{}) string {
	typ, _ := getType(t)
	typeName := path.Join(typ.Type().PkgPath(), typ.Type().Name())
	typeName = strings.ReplaceAll(typeName, "/", ".")
	return typeName
}

func valueOf(v interface{}, field string) string {
	parts := strings.Split(field, ".")
	for i, part := range parts {
		r := reflect.ValueOf(v)
		if r.Kind() == reflect.Ptr {
			r = r.Elem()
		}
		f := reflect.Indirect(r).FieldByName(part)
		if f.Kind() == reflect.Ptr {
			f = f.Elem()
		}

		switch {
		case f.Kind() == reflect.Struct && i != len(parts)-1:
			v = f.Interface()
		case f.Kind() == reflect.String:
			return f.String()
		case f.IsZero():
			return ""
		default:
			return strconv.Itoa(int(f.Int()))
		}
	}
	return ""
}
