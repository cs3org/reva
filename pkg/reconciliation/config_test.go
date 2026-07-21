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

package reconciliation

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestConfigDecodeAndDefaults(t *testing.T) {
	in := map[string]any{
		"dry_run": true,
		"path_prefix": []map[string]any{
			{
				"prefix":     "/eos/user",
				"space_type": "personal",
				"default_acl": []map[string]any{
					{"type": "u", "qualifier": "{owner}", "permissions": "rwx", "enforcement": "must"},
					{"type": "egroup", "qualifier": "cbackeosro", "permissions": "rx", "enforcement": "may"},
				},
			},
		},
	}

	var c Config
	if err := cfg.Decode(in, &c); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !c.DryRun {
		t.Errorf("DryRun = false, want true")
	}
	if c.Scanner != DefaultScanner {
		t.Errorf("Scanner = %q, want default %q", c.Scanner, DefaultScanner)
	}
	if len(c.PathPrefixes) != 1 {
		t.Fatalf("PathPrefixes = %d, want 1", len(c.PathPrefixes))
	}
	rule := c.PathPrefixes[0]
	if rule.Prefix != "/eos/user" || rule.SpaceType != SpaceTypePersonal {
		t.Errorf("rule = %+v, unexpected prefix/space_type", rule)
	}
	if len(rule.DefaultACLs) != 2 {
		t.Fatalf("DefaultACLs = %d, want 2", len(rule.DefaultACLs))
	}
	if got := rule.DefaultACLs[0]; got.Qualifier != "{owner}" || got.Enforcement != EnforcementMust {
		t.Errorf("DefaultACLs[0] = %+v, unexpected", got)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestConfigScannerOverride(t *testing.T) {
	var c Config
	if err := cfg.Decode(map[string]any{"scanner": "custom"}, &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.Scanner != "custom" {
		t.Errorf("Scanner = %q, want %q", c.Scanner, "custom")
	}
}

func TestConfigValidate(t *testing.T) {
	valid := DefaultACLRule{Type: "u", Qualifier: "{owner}", Permissions: "rwx", Enforcement: EnforcementMust}

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "ok",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos/user", SpaceType: SpaceTypePersonal, DefaultACLs: []DefaultACLRule{valid}},
			}},
		},
		{
			name: "global space type ok",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos", SpaceType: SpaceTypeAny, DefaultACLs: []DefaultACLRule{valid}},
			}},
		},
		{
			name:    "missing prefix",
			cfg:     Config{PathPrefixes: []PathPrefixRule{{SpaceType: SpaceTypePersonal}}},
			wantErr: true,
		},
		{
			name:    "empty space type",
			cfg:     Config{PathPrefixes: []PathPrefixRule{{Prefix: "/eos/user"}}},
			wantErr: true,
		},
		{
			name:    "invalid space type",
			cfg:     Config{PathPrefixes: []PathPrefixRule{{Prefix: "/eos/user", SpaceType: "bogus"}}},
			wantErr: true,
		},
		{
			name: "invalid acl type",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos/user", SpaceType: SpaceTypePersonal, DefaultACLs: []DefaultACLRule{
					{Type: "x", Qualifier: "q", Permissions: "rx", Enforcement: EnforcementMay},
				}},
			}},
			wantErr: true,
		},
		{
			name: "missing qualifier",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos/user", SpaceType: SpaceTypePersonal, DefaultACLs: []DefaultACLRule{
					{Type: "u", Permissions: "rx", Enforcement: EnforcementMay},
				}},
			}},
			wantErr: true,
		},
		{
			name: "missing permissions",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos/user", SpaceType: SpaceTypePersonal, DefaultACLs: []DefaultACLRule{
					{Type: "u", Qualifier: "q", Enforcement: EnforcementMay},
				}},
			}},
			wantErr: true,
		},
		{
			name: "invalid enforcement",
			cfg: Config{PathPrefixes: []PathPrefixRule{
				{Prefix: "/eos/user", SpaceType: SpaceTypePersonal, DefaultACLs: []DefaultACLRule{
					{Type: "u", Qualifier: "q", Permissions: "rx", Enforcement: "sometimes"},
				}},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
