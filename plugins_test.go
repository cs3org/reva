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

package reva

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockPlugin struct {
	id string
}

func (p mockPlugin) RevaPlugin() PluginInfo {
	return PluginInfo{ID: PluginID(p.id)}
}

func TestGetPlugins(t *testing.T) {
	registry = map[string]Plugin{
		"a":      mockPlugin{id: "a"},
		"a.b":    mockPlugin{id: "a.b"},
		"a.b.c":  mockPlugin{id: "a.b.c"},
		"a.b.cd": mockPlugin{id: "a.b.cd"},
		"a.c":    mockPlugin{id: "a.c"},
		"a.d":    mockPlugin{id: "a.d"},
		"b":      mockPlugin{id: "b"},
		"b.a":    mockPlugin{id: "b.a"},
		"b.b":    mockPlugin{id: "b.b"},
		"b.a.c":  mockPlugin{id: "b.a.c"},
		"c":      mockPlugin{id: "c"},
	}

	tests := []struct {
		scope string
		exp   []PluginInfo
	}{
		{
			scope: "",
			exp: []PluginInfo{
				{ID: "a"},
				{ID: "a.b"},
				{ID: "a.b.c"},
				{ID: "a.b.cd"},
				{ID: "a.c"},
				{ID: "a.d"},
				{ID: "b"},
				{ID: "b.a"},
				{ID: "b.a.c"},
				{ID: "b.b"},
				{ID: "c"},
			},
		},
		{
			scope: "a",
			exp: []PluginInfo{
				{ID: "a"},
				{ID: "a.b"},
				{ID: "a.b.c"},
				{ID: "a.b.cd"},
				{ID: "a.c"},
				{ID: "a.d"},
			},
		},
		{
			scope: "a.b.c",
			exp: []PluginInfo{
				{ID: "a.b.c"},
			},
		},
		{
			scope: "b.a",
			exp: []PluginInfo{
				{ID: "b.a"},
				{ID: "b.a.c"},
			},
		},
	}

	for i, tt := range tests {
		got := GetPlugins(tt.scope)
		if !reflect.DeepEqual(got, tt.exp) {
			t.Fatalf("test %d: expected %v got %v", i+1, tt.exp, got)
		}
	}
}

func TestNameNamespace(t *testing.T) {
	tests := []struct {
		id    PluginID
		name  string
		ns    string
		valid bool
	}{
		{
			id:    "",
			valid: false,
		},
		{
			id:    "a",
			valid: false,
		},
		{
			id:    "a.b",
			name:  "b",
			ns:    "a",
			valid: true,
		},
		{
			id:    "aa.bb.cc",
			name:  "cc",
			ns:    "aa.bb",
			valid: true,
		},
	}

	for i, tt := range tests {
		if !tt.valid {
			assert.Panics(t, func() {
				tt.id.Name()
			}, "test %d should have paniced", i)
			assert.Panics(t, func() {
				tt.id.Namespace()
			}, "test %d should have paniced", i)
		} else {
			name := tt.id.Name()
			ns := tt.id.Namespace()
			assert.Equal(t, tt.name, name, "test %d", i)
			assert.Equal(t, tt.ns, ns, "test %d", i)
		}
	}
}
