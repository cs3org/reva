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

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrandcheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.toml", "[program]\nname = \"acme\"\n[auth]\nchain = [\"token\", \"kerberos\"]\n[ui]\nhide = [\"fleet\"]\n")
	missing := write("missing.toml", "[program]\nname = \"acme\"\n[auth]\nchain = [\"token\", \"cern-sso\"]\n")
	typo := write("typo.toml", "[program]\nname = \"acme\"\n[ui]\nhid = [\"fleet\"]\n")

	var out bytes.Buffer
	if code := run([]string{good, filepath.Join("..", "..", "..", "pkg", "cli", "brand", "reva.toml")}, &out); code != 0 {
		t.Errorf("code = %d: %s", code, out.String())
	}
	for p, want := range map[string]string{missing: `no sign-in method "cern-sso"`, typo: "unknown key ui.hid"} {
		out.Reset()
		if code := run([]string{p}, &out); code != 1 || !strings.Contains(out.String(), want) {
			t.Errorf("%s: code = %d, want 1 and %q in:\n%s", filepath.Base(p), code, want, out.String())
		}
	}
}
