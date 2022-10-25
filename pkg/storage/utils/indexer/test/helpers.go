// Copyright 2018-2022 CERN
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

package test

import (
	"errors"
	"os"
	"path"
	"reflect"
	"strings"
)

// CreateTmpDir creates a temporary dir for tests data.
func CreateTmpDir() (string, error) {
	name, err := os.MkdirTemp("/tmp", "testfiles-")
	if err != nil {
		return "", err
	}

	return name, nil
}

// ValueOf gets the value of a type v on a given field <field>.
func ValueOf(v interface{}, field string) string {
	r := reflect.ValueOf(v)
	f := reflect.Indirect(r).FieldByName(field)

	return f.String()
}

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

// GetTypeFQN formats a valid name from a type <t>. This is a duplication of the already existing function in the
// indexer package, but since there is a circular dependency we chose to duplicate it.
func GetTypeFQN(t interface{}) string {
	typ, _ := getType(t)
	typeName := path.Join(typ.Type().PkgPath(), typ.Type().Name())
	typeName = strings.ReplaceAll(typeName, "/", ".")
	return typeName
}
