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

package openapi

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// Merge adds what src describes to dst, and keeps what dst already has.
// A conflict is possible only in a fleet that runs mixed releases, where two
// versions of a type share a schema name: the first one merged is kept.
func Merge(dst, src *openapi3.T) {
	if src.Paths != nil {
		if dst.Paths == nil {
			dst.Paths = openapi3.NewPaths()
		}
		for path, item := range src.Paths.Map() {
			have := dst.Paths.Value(path)
			if have == nil {
				dst.Paths.Set(path, item)
				continue
			}
			for method, op := range item.Operations() {
				if have.GetOperation(method) == nil {
					have.SetOperation(method, op)
				}
			}
		}
	}

	if src.Components != nil {
		if dst.Components == nil {
			dst.Components = &openapi3.Components{}
		}
		for name, s := range src.Components.Schemas {
			if dst.Components.Schemas == nil {
				dst.Components.Schemas = openapi3.Schemas{}
			}
			if _, ok := dst.Components.Schemas[name]; !ok {
				dst.Components.Schemas[name] = s
			}
		}
		for name, s := range src.Components.SecuritySchemes {
			if dst.Components.SecuritySchemes == nil {
				dst.Components.SecuritySchemes = openapi3.SecuritySchemes{}
			}
			if _, ok := dst.Components.SecuritySchemes[name]; !ok {
				dst.Components.SecuritySchemes[name] = s
			}
		}
	}

	for _, tag := range src.Tags {
		if dst.Tags.Get(tag.Name) == nil {
			dst.Tags = append(dst.Tags, tag)
		}
	}
	sort.Slice(dst.Tags, func(i, j int) bool { return dst.Tags[i].Name < dst.Tags[j].Name })
}
