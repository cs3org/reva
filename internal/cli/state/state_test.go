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

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigCopiesLegacyOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	legacy := filepath.Join(home, ".reva.config")
	if err := os.WriteFile(legacy, []byte(`{"host":"gw:9142","admin_host":"admin:9999"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Connection.Gateway != "gw:9142" || c.Connection.AdminHost != "admin:9999" {
		t.Fatalf("legacy config not read: %+v", c)
	}

	// The new file now exists, so a change to the legacy file has no effect.
	if err := os.WriteFile(legacy, []byte(`{"host":"other:1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c.Connection.Gateway = "new:9142"
	if err := WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	c, err = ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Connection.Gateway != "new:9142" || c.Connection.AdminHost != "admin:9999" {
		t.Fatalf("config.toml not used: %+v", c)
	}
}
