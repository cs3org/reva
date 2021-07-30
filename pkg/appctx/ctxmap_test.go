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

package appctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetKeyValues(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		m    map[interface{}]interface{}
	}{
		{
			"Background context",
			context.Background(),
			map[interface{}]interface{}{},
		},
		{
			"Context with Values",
			context.WithValue(context.Background(), "key", "value"), //nolint
			map[interface{}]interface{}{
				"key": "value",
			},
		},
		{
			"Nested Context with Values",
			context.WithValue(context.WithValue(context.Background(), "key", "value"), "key2", "value2"), //nolint
			map[interface{}]interface{}{
				"key":  "value",
				"key2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kvMap := GetKeyValues(tt.ctx)
			assert.Equal(t, tt.m, kvMap)
		})
	}
}

func TestPutKeyValues(t *testing.T) {
	tests := []struct {
		name string
		m    map[interface{}]interface{}
		ctx  context.Context
	}{
		{
			"empty context",
			map[interface{}]interface{}{},
			context.Background(),
		},
		{
			"single kv pair",
			map[interface{}]interface{}{
				"key": "value",
			},
			context.WithValue(context.Background(), "key", "value"), //nolint
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := PutKeyValues(tt.m)
			assert.Equal(t, tt.ctx, ctx)
		})
	}
}
