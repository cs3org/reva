package config

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

type parent interface {
	SetValue(v any)
}

type parentList struct {
	List  reflect.Value
	Index int
}

type parentMap struct {
	Map reflect.Value
	Key any
}

type parentStruct struct {
	Struct reflect.Value
	Field  int
}

func (p parentList) SetValue(v any) {
	el := p.List.Index(p.Index)
	el.Set(reflect.ValueOf(v))
}

func (p parentMap) SetValue(v any) {
	p.Map.SetMapIndex(reflect.ValueOf(p.Key), reflect.ValueOf(v))
}

func (p parentStruct) SetValue(v any) {
	p.Struct.Field(p.Field).Set(reflect.ValueOf(v))
}

func (c *Config) applyTemplateStruct(p parent, v reflect.Value) error {
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

		if err := c.applyTemplateByType(parentStruct{Struct: v, Field: i}, el); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) applyTemplateByType(p parent, v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		return c.applyTemplateString(p, v)
	case reflect.Array, reflect.Slice:
		return c.applyTemplateList(p, v)
	case reflect.Struct:
		return c.applyTemplateStruct(p, v)
	case reflect.Map:
		return c.applyTemplateMap(p, v)
	case reflect.Interface:
		return c.applyTemplateInterface(p, v)
	case reflect.Pointer:
		return c.applyTemplateByType(p, v.Elem())
	}
	return nil
}

func (c *Config) applyTemplateList(p parent, v reflect.Value) error {
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		panic("called applyTemplateList on non array/slice type")
	}

	for i := 0; i < v.Len(); i++ {
		el := v.Index(i)
		if err := c.applyTemplateByType(parentList{List: v, Index: i}, el); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) applyTemplateMap(p parent, v reflect.Value) error {
	if v.Kind() != reflect.Map {
		panic("called applyTemplateMap on non map type")
	}

	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		el := v.MapIndex(k)
		if err := c.applyTemplateByType(parentMap{Map: v, Key: k.Interface()}, el); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) applyTemplateInterface(p parent, v reflect.Value) error {
	if v.Kind() != reflect.Interface {
		panic("called applyTemplateInterface on non interface value")
	}

	s, ok := v.Interface().(string)
	if !ok {
		return c.applyTemplateByType(p, v.Elem())
	}

	if !isTemplate(s) {
		// nothing to do
		return nil
	}

	key := keyFromTemplate(s)
	val, err := c.Lookup(key)
	if err != nil {
		return err
	}

	p.SetValue(val)
	return nil
}

func (c *Config) applyTemplateString(p parent, v reflect.Value) error {
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
	val, err := c.Lookup(key)
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
