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

package registry

import (
	"reflect"
	"testing"
)

/*
config example:

---
services:
  authprovider:
    basic:
      name: auth-basic
      nodes:
      - address: 0.0.0.0:1234
        metadata:
          version: v0.1.0
    bearer:
      name: auth-bearer
      nodes:
      - address: 0.0.0.0:5678
        metadata:
          version: v0.1.0

*/
func TestParseConfig(t *testing.T) {
	type args struct {
		m map[string]interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{name: "parse config", args: args{map[string]interface{}{
			"services": map[string]map[string]interface{}{
				"authprovider": map[string]interface{}{
					"basic": map[string]interface{}{
						"name": "auth-basic",
						"nodes": []map[string]interface{}{
							{
								"address":  "0.0.0.0:1234",
								"metadata": map[string]string{"version": "v0.1.0"},
							},
						},
					},
					"bearer": map[string]interface{}{
						"name": "auth-bearer",
						"nodes": []map[string]interface{}{
							{
								"address":  "0.0.0.0:5678",
								"metadata": map[string]string{"version": "v0.1.0"},
							},
						},
					},
				},
			},
		}}, want: &Config{
			Services: map[string]map[string]*service{
				"authprovider": map[string]*service{
					"basic": &service{
						Name: "auth-basic",
						Nodes: []node{{
							Address:  "0.0.0.0:1234",
							Metadata: map[string]string{"version": "v0.1.0"},
						}},
					},
					"bearer": &service{
						Name: "auth-bearer",
						Nodes: []node{{
							Address:  "0.0.0.0:5678",
							Metadata: map[string]string{"version": "v0.1.0"},
						}},
					},
				},
			},
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.args.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}
