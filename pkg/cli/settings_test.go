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

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/spf13/pflag"
)

const acmeBrand = `
[program]
name = "acme"
config_dir = "acme-cli"
[connection]
gateway = "brand:9142"
[env]
gateway = ["ACME_GATEWAY", "ACME_HOST"]
output = ["ACME_OUTPUT"]
config = ["ACME_CONFIG"]
token_cache = ["ACME_TOKEN_CACHE"]
no_elevate = ["ACME_NO_ELEVATE"]
`

// acme returns the runtime of the test brand after the flags args, and
// the dir of the test.
func acme(t *testing.T, args ...string) (*Runtime, error) {
	t.Helper()
	rt := NewRuntime(brand.MustParse([]byte(acmeBrand)))
	fs := pflag.NewFlagSet("acme", pflag.ContinueOnError)
	rt.AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return rt, rt.Load(fs)
}

func writeFile(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func userConfig(dir string) string   { return filepath.Join(dir, ".config", "acme-cli", "config.toml") }
func systemConfig(dir string) string { return filepath.Join(dir, "etc", "acme-cli", "config.toml") }

func wantSetting(t *testing.T, rt *Runtime, key, want, wantFrom string) {
	t.Helper()
	v, from := rt.Setting(key)
	if v != want || !strings.HasPrefix(from, wantFrom) {
		t.Errorf("%s = %q from %q, want %q from %q", key, v, from, want, wantFrom)
	}
}

func TestSettingBuiltIn(t *testing.T) {
	setup(t)
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "transport", "grpc", "built-in")
	wantSetting(t, rt, "output", "table", "built-in")
	wantSetting(t, rt, "admin_host", "", "built-in")
}

func TestSettingBrand(t *testing.T) {
	setup(t)
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "brand:9142", "brand acme")
	if rt.GatewayAddr() != "brand:9142" {
		t.Errorf("gateway = %q", rt.GatewayAddr())
	}
}

func TestSettingSystemConfig(t *testing.T) {
	dir := setup(t)
	writeFile(t, systemConfig(dir), "[connection]\ngateway = \"system:9142\"\n[ui]\noutput = \"json\"\n")
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "system:9142", "system config")
	wantSetting(t, rt, "output", "json", "system config")
}

func TestSettingUserConfig(t *testing.T) {
	dir := setup(t)
	writeFile(t, systemConfig(dir), "[connection]\ngateway = \"system:9142\"\nadmin_host = \"admin:9999\"\n")
	// An empty value is no value: it must not hide the system config.
	writeFile(t, userConfig(dir), "[connection]\ngateway = \"user:9142\"\nadmin_host = \"\"\n")
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "user:9142", "user config")
	wantSetting(t, rt, "admin_host", "admin:9999", "system config")
}

func TestSettingEnv(t *testing.T) {
	dir := setup(t)
	writeFile(t, userConfig(dir), "[connection]\ngateway = \"user:9142\"\n[ui]\noutput = \"table\"\n")
	t.Setenv("ACME_HOST", "alias:9142")
	t.Setenv("ACME_OUTPUT", "json")
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "alias:9142", "env ACME_HOST")
	wantSetting(t, rt, "output", "json", "env ACME_OUTPUT")

	t.Setenv("ACME_GATEWAY", "env:9142")
	rt, _ = acme(t)
	wantSetting(t, rt, "gateway", "env:9142", "env ACME_GATEWAY")
}

func TestSettingFlag(t *testing.T) {
	setup(t)
	t.Setenv("ACME_GATEWAY", "env:9142")
	t.Setenv("ACME_OUTPUT", "json")
	rt, err := acme(t, "--gateway", "flag:9142", "-o", "table")
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "flag:9142", "flag --gateway")
	wantSetting(t, rt, "output", "table", "flag --output")
	if rt.GatewayAddr() != "flag:9142" || rt.Output() != "table" {
		t.Errorf("gateway = %q, output = %q", rt.GatewayAddr(), rt.Output())
	}
}

// The env names come from the brand only: acme does not read the names of
// reva, and warns once that it ignores them.
func TestSettingForeignEnv(t *testing.T) {
	setup(t)
	t.Setenv("REVA_GATEWAY", "reva:9142")
	var out bytes.Buffer
	old := stderr
	stderr, foreignEnvOnce = &out, sync.Once{}
	t.Cleanup(func() { stderr = old })

	rt, _ := acme(t)
	wantSetting(t, rt, "gateway", "brand:9142", "brand")
	acme(t)
	if got := out.String(); strings.Count(got, "ignoring REVA_GATEWAY") != 1 {
		t.Errorf("warnings: %q, want one", got)
	}

	out.Reset()
	foreignEnvOnce = sync.Once{}
	rt = NewRuntime(nil)
	if rt.GatewayAddr() != "reva:9142" || out.Len() > 0 {
		t.Errorf("reva: gateway = %q, warning %q", rt.GatewayAddr(), out.String())
	}
	t.Setenv("REVA_GATEWAY", "")
	t.Setenv("REVA_HOST", "legacy:9142")
	if rt := NewRuntime(nil); rt.GatewayAddr() != "legacy:9142" {
		t.Errorf("REVA_HOST is not an alias of the gateway: %q", rt.GatewayAddr())
	}
}

func TestSettingFiles(t *testing.T) {
	dir := setup(t)
	explicit := filepath.Join(dir, "elsewhere.toml")
	writeFile(t, explicit, "[connection]\ngateway = \"explicit:9142\"\n")
	writeFile(t, userConfig(dir), "[connection]\ngateway = \"user:9142\"\n")
	t.Setenv("ACME_CONFIG", explicit)
	t.Setenv("ACME_TOKEN_CACHE", filepath.Join(dir, "tok"))
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	wantSetting(t, rt, "gateway", "explicit:9142", "user config "+explicit)
	if p, _ := state.TokenPath(); p != filepath.Join(dir, "tok") {
		t.Errorf("token path = %s", p)
	}
	t.Setenv("ACME_CONFIG", "")
	t.Setenv("ACME_TOKEN_CACHE", "")
	acme(t)
	if p, _ := state.TokenPath(); p != filepath.Join(dir, ".config", "acme-cli", "token") {
		t.Errorf("token path = %s, want it in the config dir of the brand", p)
	}
}

func TestSettingErrorsWaitForLoad(t *testing.T) {
	dir := setup(t)
	writeFile(t, userConfig(dir), "[connection]\ngatway = \"typo\"\n")
	rt := NewRuntime(brand.MustParse([]byte(acmeBrand)))
	fs := pflag.NewFlagSet("acme", pflag.ContinueOnError)
	rt.AddFlags(fs)
	if err := rt.Load(fs); err == nil || !strings.Contains(err.Error(), "gatway") {
		t.Errorf("err = %v, want the unknown key", err)
	}

	writeFile(t, userConfig(dir), "")
	t.Setenv("ACME_NO_ELEVATE", "maybe")
	if _, err := acme(t); err == nil || !strings.Contains(err.Error(), "ACME_NO_ELEVATE") {
		t.Errorf("err = %v, want the bad boolean", err)
	}
	t.Setenv("ACME_NO_ELEVATE", "1")
	if rt, err := acme(t); err != nil || !rt.noElevate {
		t.Errorf("no_elevate = %v, err = %v", rt.noElevate, err)
	}
}

func TestPluginConfigMergesFiles(t *testing.T) {
	dir := setup(t)
	writeFile(t, systemConfig(dir), "[plugins.otg]\nnumber = 1\nmessage = \"from puppet\"\n")
	writeFile(t, userConfig(dir), "[plugins.otg]\nnumber = 2\n")
	rt, err := acme(t)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Number  int
		Message string
	}
	if err := PluginConfig(rt, "otg", &c); err != nil || c.Number != 2 || c.Message != "from puppet" {
		t.Errorf("otg = %+v, err = %v", c, err)
	}
}
