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

package plugin

import (
	"reflect"
)

// RegistryFunc is the func a component that is pluggable
// must define to register the new func in its own registry.
// It is responsibility of the component to type assert the
// new func with the expected one and panic if not.
type RegistryFunc func(name string, newFunc any)

var registry = map[string]RegistryFunc{} // key is the namespace

// RegisterNamespace is the function called by a component
// that is pluggable, to register its namespace and a function
// to register the plugins.
func RegisterNamespace(ns string, f RegistryFunc) {
	if ns == "" {
		panic("namespace cannot be empty")
	}
	registry[ns] = f
}

// RegisterPlugin is called to register a new plugin in the
// given namespace. Its called internally by reva, and should
// not be used by external plugins.
func RegisterPlugin(ns, name string, newFunc any) {
	if ns == "" {
		panic("namespace cannot be empty")
	}
	if name == "" {
		panic("name cannot be empty")
	}
	if newFunc == nil {
		panic("new func cannot be nil")
	}
	if reflect.TypeOf(newFunc).Kind() != reflect.Func {
		panic("type must be a function")
	}
	r, ok := registry[ns]
	if !ok {
		panic("namespace does not exist")
	}
	r(name, newFunc)
}
