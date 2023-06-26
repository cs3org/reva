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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyTemplate(t *testing.T) {
	cfg1 := &Config{
		GRPC: &GRPC{
			Services: map[string]ServicesConfig{
				"authprovider": {
					{
						Address: "localhost:1900",
					},
				},
				"authregistry": {
					{
						Address: "localhost:1901",
						Config: map[string]any{
							"drivers": map[string]any{
								"static": map[string]any{
									"demo": "{{ grpc.services.authprovider.address }}",
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg1.ApplyTemplates(cfg1)
	assert.ErrorIs(t, err, nil)
	assert.Equal(t, cfg1.GRPC.Services["authregistry"][0].Config["drivers"].(map[string]any)["static"].(map[string]any)["demo"], "localhost:1900")

	cfg2 := &Config{
		Vars: Vars{
			"db_username": "root",
			"db_password": "secretpassword",
		},
		GRPC: &GRPC{
			Services: map[string]ServicesConfig{
				"authregistry": {
					{
						Address: "localhost:1901",
						Config: map[string]any{
							"drivers": map[string]any{
								"sql": map[string]any{
									"db_username": "{{ vars.db_username }}",
									"db_password": "{{ vars.db_password }}",
									"key":         "value",
								},
							},
						},
					},
				},
			},
		},
	}

	err = cfg2.ApplyTemplates(cfg2)
	assert.ErrorIs(t, err, nil)
	assert.Equal(t, cfg2.GRPC.Services["authregistry"][0].Config["drivers"].(map[string]any)["sql"],
		map[string]any{
			"db_username": "root",
			"db_password": "secretpassword",
			"key":         "value",
		})

}
