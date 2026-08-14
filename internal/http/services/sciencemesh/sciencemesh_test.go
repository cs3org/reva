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

package sciencemesh

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func requiredScienceMeshConfig() map[string]any {
	return map[string]any{
		"gatewaysvc":         "gateway:9142",
		"mesh_directory_url": "https://mesh.example",
		"provider_domain":    "example.org",
	}
}

func TestConfigOCMClientLimits(t *testing.T) {
	tests := []struct {
		name        string
		extra       map[string]any
		wantLimit   int
		wantTimeout int
	}{
		{
			name: "parses flat keys",
			extra: map[string]any{
				"ocm_client_response_limit": 2097152,
				"ocm_client_timeout":        45,
			},
			wantLimit:   2097152,
			wantTimeout: 45,
		},
		{
			name:        "defaults when unset",
			extra:       map[string]any{},
			wantLimit:   1 << 20,
			wantTimeout: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := requiredScienceMeshConfig()
			for k, v := range tt.extra {
				input[k] = v
			}

			var c config
			if err := cfg.Decode(input, &c); err != nil {
				t.Fatalf("cfg.Decode: %v", err)
			}
			if c.OCMClientResponseLimit != tt.wantLimit {
				t.Errorf("OCMClientResponseLimit = %d, want %d", c.OCMClientResponseLimit, tt.wantLimit)
			}
			if c.OCMClientTimeout != tt.wantTimeout {
				t.Errorf("OCMClientTimeout = %d, want %d", c.OCMClientTimeout, tt.wantTimeout)
			}
		})
	}
}
