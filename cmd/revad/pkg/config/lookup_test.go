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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type SimpleStruct struct {
	KeyA string `key:"keya"`
	KeyB string `key:"keyb"`
}

type NestedStruct struct {
	Nested SimpleStruct `key:"nested"`
	Value  int          `key:"value"`
}

type StructWithNestedMap struct {
	Map map[string]any `key:"map"`
}

type StructWithNestedList struct {
	List []SimpleStruct `key:"list"`
}

type Squashed struct {
	Squashed SimpleStruct `key:",squash"`
	Simple   SimpleStruct
}

type SquashedMap struct {
	Squashed map[string]any `key:",squash"`
	Simple   SimpleStruct   `key:"simple"`
}

func TestLookupStruct(t *testing.T) {
	tests := []struct {
		in  any
		key string
		val any
		err error
	}{
		{
			in: SimpleStruct{
				KeyA: "val_a",
				KeyB: "val_b",
			},
			key: ".keyb",
			val: "val_b",
		},
		{
			in: NestedStruct{
				Nested: SimpleStruct{
					KeyA: "val_a",
					KeyB: "val_b",
				},
				Value: 10,
			},
			key: ".nested.keyb",
			val: "val_b",
		},
		{
			in: NestedStruct{
				Nested: SimpleStruct{
					KeyA: "val_a",
					KeyB: "val_b",
				},
				Value: 10,
			},
			key: ".value",
			val: 10,
		},
		{
			in: StructWithNestedMap{
				Map: map[string]any{
					"key1": "val1",
					"key2": "val2",
				},
			},
			key: ".map.key1",
			val: "val1",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
					{
						KeyA: "val_a[2]",
						KeyB: "val_b[2]",
					},
				},
			},
			key: ".list[1].keyb",
			val: "val_b[2]",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
				},
			},
			key: ".list.keya",
			val: "val_a[1]",
		},
		{
			in: StructWithNestedList{
				List: []SimpleStruct{
					{
						KeyA: "val_a[1]",
						KeyB: "val_b[1]",
					},
					{
						KeyA: "val_a[2]",
						KeyB: "val_b[2]",
					},
				},
			},
			key: ".list[1]",
			val: SimpleStruct{
				KeyA: "val_a[2]",
				KeyB: "val_b[2]",
			},
		},
		{
			in: Squashed{
				Squashed: SimpleStruct{
					KeyA: "val_a[1]",
					KeyB: "val_b[1]",
				},
				Simple: SimpleStruct{
					KeyA: "val_a[2]",
					KeyB: "val_b[2]",
				},
			},
			key: ".keya",
			val: "val_a[1]",
		},
		{
			in: SquashedMap{
				Squashed: map[string]any{
					"keya": "val_a[1]",
					"keyb": "val_b[1]",
				},
				Simple: SimpleStruct{
					KeyA: "val_a[2]",
					KeyB: "val_b[2]",
				},
			},
			key: ".keya",
			val: "val_a[1]",
		},
	}

	for _, tt := range tests {
		got, err := lookupStruct(tt.key, reflect.ValueOf(tt.in))
		assert.Equal(t, err, tt.err, "got not expected error")
		if tt.err == nil {
			assert.Equal(t, tt.val, got)
		}
	}

}
