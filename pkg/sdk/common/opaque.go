// Copyright 2018-2021 CERN
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

package common

import (
	"fmt"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// DecodeOpaqueMap decodes a Reva opaque object into a map of strings.
// Only plain decoders are currently supported.
func DecodeOpaqueMap(opaque *types.Opaque) map[string]string {
	entries := make(map[string]string)

	if opaque != nil {
		for k, v := range opaque.GetMap() {
			// Only plain values are currently supported
			if v.Decoder == "plain" {
				entries[k] = string(v.Value)
			}
		}
	}

	return entries
}

// GetValuesFromOpaque extracts the given keys from the opaque object.
// If mandatory is set to true, all specified keys must be available in the opaque object.
func GetValuesFromOpaque(opaque *types.Opaque, keys []string, mandatory bool) (map[string]string, error) {
	values := make(map[string]string)
	entries := DecodeOpaqueMap(opaque)

	for _, key := range keys {
		if value, ok := entries[key]; ok {
			values[key] = value
		} else if mandatory {
			return map[string]string{}, fmt.Errorf("missing opaque entry '%v'", key)
		}
	}

	return values, nil
}
