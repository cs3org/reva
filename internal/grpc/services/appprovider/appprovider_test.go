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

package appprovider

import (
	"testing"

	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

func Test_parseConfig(t *testing.T) {

	tests := []struct {
		name    string
		m       map[string]interface{}
		want    *config
		wantErr interface{}
	}{
		{
			name: "all configurations set",
			m: map[string]interface{}{
				"Driver":    "demo",
				"Demo":      map[string]interface{}{"a": "b", "c": "d"},
				"IopSecret": "very-secret",
				"WopiURL":   "https://my.wopi:9871",
			},
			want: &config{
				Driver:    "demo",
				Demo:      map[string]interface{}{"a": "b", "c": "d"},
				IopSecret: "very-secret",
				WopiURL:   "https://my.wopi:9871",
			},
			wantErr: nil,
		},
		{
			name: "wrong type of setting",
			m:    map[string]interface{}{"Driver": 123, "IopSecret": 456},
			want: nil,
			wantErr: &mapstructure.Error{
				Errors: []string{
					"'driver' expected type 'string', got unconvertible type 'int', value: '123'",
					"'iopsecret' expected type 'string', got unconvertible type 'int', value: '456'",
				},
			},
		},
		{
			name: "undefined settings type",
			m:    map[string]interface{}{"Not-Defined": 123},
			want: &config{
				Driver:    "",
				Demo:      map[string]interface{}(nil),
				IopSecret: "",
				WopiURL:   "",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig(tt.m)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
