// Copyright 2018-2024 CERN
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

	"github.com/cs3org/reva/v3/pkg/utils/cfg"
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
			name: "all configurations set for demo driver",
			m: map[string]interface{}{
				"Driver":  "demo",
				"Drivers": map[string]map[string]interface{}{"demo": {"a": "b", "c": "d"}},
			},
			want: &config{
				Driver:  "demo",
				Drivers: map[string]map[string]interface{}{"demo": {"a": "b", "c": "d"}},
			},
			wantErr: nil,
		},
		{
			name: "all configurations set for wopi driver",
			// Note that the wopi driver is not loaded by this unit test, therefore those properties are just a non-validated example
			m: map[string]interface{}{
				"Driver":  "wopi",
				"Drivers": map[string]map[string]interface{}{"wopi": {"iop_secret": "very-secret", "wopi_url": "https://my.wopi:9871"}},
			},
			want: &config{
				Driver:  "wopi",
				Drivers: map[string]map[string]interface{}{"wopi": {"iop_secret": "very-secret", "wopi_url": "https://my.wopi:9871"}},
			},
			wantErr: nil,
		},
		{
			name: "wrong type of setting",
			m:    map[string]interface{}{"Driver": 123, "NonExistentField": 456},
			want: nil,
			wantErr: &mapstructure.Error{
				Errors: []string{
					"'driver' expected type 'string', got unconvertible type 'int', value: '123'",
				},
			},
		},
		{
			name: "undefined settings type",
			m:    map[string]interface{}{"Not-Defined": 123},
			want: &config{
				Driver:  "demo",
				Drivers: map[string]map[string]interface{}(nil),
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got config
			err := cfg.Decode(tt.m, &got)
			assert.Equal(t, tt.wantErr, err)
			if tt.wantErr == nil {
				assert.Equal(t, tt.want, &got)
			}
		})
	}
}
