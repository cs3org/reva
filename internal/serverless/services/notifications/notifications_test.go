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

package notifications

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestConfigDecodesAccumulationPolicy(t *testing.T) {
	raw := map[string]any{
		"events": map[string]any{
			"upload": map[string]any{
				"type":               "accumulated",
				"dedup_key_template": "{{ index .TemplateData \"share_id\" }}",
				"handlers":           map[string]any{"email": map[string]any{"template_name": "sharedfolder-upload-mail"}},
				"accumulation":       map[string]any{"window_seconds": 60, "max_items": 100},
			},
		},
	}

	var c config
	if err := cfg.Decode(raw, &c); err != nil {
		t.Fatal(err)
	}

	rule := c.Events["upload"]
	if rule.Accumulation.WindowSeconds != 60 {
		t.Fatalf("window seconds = %d, want 60", rule.Accumulation.WindowSeconds)
	}
	if rule.Accumulation.MaxItems != 100 {
		t.Fatalf("max items = %d, want 100", rule.Accumulation.MaxItems)
	}
}
