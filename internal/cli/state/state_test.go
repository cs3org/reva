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
	"strings"
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

func TestWriteConfigKeepsPluginSections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := Write("config.toml", []byte("[plugins.otg]\nnumber = 42\n")); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Connection.Gateway = "gw:9142"
	if err := WriteConfig(c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "[plugins.otg]") || !strings.Contains(string(b), "number = 42") {
		t.Fatalf("the plugin section is lost:\n%s", b)
	}
}

func TestReadConfigCopiesCernboxYAMLOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	Use(Layout{Dir: "cernbox"})
	t.Cleanup(func() { Use(Layout{Dir: "reva"}) })
	yml := filepath.Join(home, ".config", "cernbox", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(yml), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yml, []byte(`endpoint: https://cernbox.example
insecure: true
auth:
  method: kerberos
  token_cache: /tmp/cache
  kerberos:
    service_principal: HTTP/cernbox.example
transfer:
  chunk_size: 16M
  archive: false
edit:
  folder: notes
outbox:
  - local: ~/scans
    remote: /eos/scans
`), 0o600); err != nil {
		t.Fatal(err)
	}

	c, from, err := PeekConfig()
	if err != nil || from != yml {
		t.Fatalf("from = %q, err = %v", from, err)
	}
	if p, _ := ConfigPath(); fileExists(p) {
		t.Fatal("PeekConfig wrote the new file")
	}
	c, err = ReadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Connection.Endpoint != "https://cernbox.example" || !*c.Connection.Insecure || c.Auth.TokenCache != "/tmp/cache" ||
		c.Auth.Kerberos.SPN != "HTTP/cernbox.example" || c.Transfer.ChunkSize != "16M" || *c.Transfer.Archive {
		t.Errorf("config.yaml not read: %+v", c)
	}
	if c.Plugins["edit"]["folder"] != "notes" || c.Plugins["outbox"]["folders"] == nil {
		t.Errorf("plugin settings not moved: %v", c.Plugins)
	}
	p, _ := ConfigPath()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `endpoint = "https://cernbox.example"`) || !strings.Contains(string(b), "[plugins.outbox]") {
		t.Errorf("config.toml:\n%s", b)
	}
	if _, from, _ := PeekConfig(); from != "" {
		t.Errorf("config.yaml read again, from %s", from)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestConfigIsStrict(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	old := SystemDir
	SystemDir = func() string { return filepath.Join(dir, "etc") }
	t.Cleanup(func() { SystemDir = old })
	if err := Write("config.toml", []byte("[connection]\ngatway = \"x\"\n[plugins.any]\nkey = 1\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(); err == nil || !strings.Contains(err.Error(), "connection.gatway") {
		t.Errorf("err = %v, want the unknown key", err)
	}
	if err := Write("config.toml", []byte("[plugins.any]\nkey = 1\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(); err != nil {
		t.Errorf("a plugin section is refused: %v", err)
	}
	if c, err := ReadSystemConfig(); err != nil || c.Connection.Gateway != "" {
		t.Errorf("a missing system config: %+v, %v", c, err)
	}
	if p := SystemConfigPath(); p != filepath.Join(dir, "etc", "reva", "config.toml") {
		t.Errorf("system config = %s", p)
	}
}
