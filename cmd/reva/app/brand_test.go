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

package revacmd_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	revacmd "github.com/cs3org/reva/v3/cmd/reva/app"
	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/spf13/cobra"
)

// acme is a test brand. mode is ui.operator_commands.
func acme(mode string) *brand.Brand {
	return brand.MustParse(fmt.Appendf(nil, `
[program]
name = "acme"
short = "Acme Box command line client"
docs_url = "https://docs.acme.example/cli"
config_dir = "acme-cli"
[connection]
gateway = "gw.acme.example:9142"
[auth]
chain = ["token", "kerberos"]
[env]
gateway = ["ACME_GATEWAY"]
admin_host = ["ACME_ADMIN_HOST"]
[ui]
hide = ["sharing"]
operator_commands = %q
`, mode))
}

// isolate gives the test its own config dirs. It returns the user config
// dir.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("XDG_RUNTIME_DIR", dir)
	old := state.SystemDir
	state.SystemDir = func() string { return filepath.Join(dir, "etc") }
	t.Cleanup(func() { state.SystemDir = old })
	return filepath.Join(dir, ".config")
}

func execute(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestBrandChangesTheProgram(t *testing.T) {
	cfg := isolate(t)
	root := revacmd.NewRoot(revacmd.WithBrand(acme("show")))
	help, err := execute(t, root, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Acme Box command line client",
		"Documentation: https://docs.acme.example/cli",
		"acme [command]",
		"(env ACME_GATEWAY) (default \"gw.acme.example:9142\")",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("no %q in the help:\n%s", want, help)
		}
	}
	if strings.Contains(help, "REVA_") {
		t.Errorf("the help names a reva env:\n%s", help)
	}
	if c, _, _ := root.Find([]string{"cp"}); !strings.Contains(c.Example, "acme cp ") {
		t.Errorf("the example of cp is not renamed:\n%s", c.Example)
	}

	t.Setenv("ACME_GATEWAY", "env.acme.example:9142")
	out, err := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme("show"))), "config", "view")
	if err != nil || !strings.Contains(out, "env.acme.example:9142 (env ACME_GATEWAY)") {
		t.Errorf("err = %v, config view:\n%s", err, out)
	}
	if _, err := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme("show"))), "config", "set", "admin-host", "admin:9999"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(cfg, "acme-cli", "config.toml")); err != nil || !strings.Contains(string(b), "admin:9999") {
		t.Errorf("config set did not write to the config dir of the brand: %s, %v", b, err)
	}

	out, err = execute(t, revacmd.NewRoot(revacmd.WithBrand(acme("show"))), "version", "--plugins")
	if err != nil || !strings.Contains(out, "brand=acme sha256=") {
		t.Errorf("err = %v, version:\n%s", err, out)
	}
}

// A group of ui.hide is out of help, with its title, and its commands run.
func TestBrandHidesGroups(t *testing.T) {
	isolate(t)
	root := revacmd.NewRoot(revacmd.WithBrand(acme("show")))
	help, _ := execute(t, root, "--help")
	if strings.Contains(help, "Sharing:") || strings.Contains(help, "  share ") {
		t.Errorf("the sharing group is in the help:\n%s", help)
	}
	if out, _ := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme("show"))), "help", "--all"); !strings.Contains(out, "  share ") {
		t.Errorf("help --all does not show share:\n%s", out)
	}
	if out, err := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme("show"))), "share", "--help"); err != nil || !strings.Contains(out, "acme share") {
		t.Errorf("err = %v, share --help:\n%s", err, out)
	}
}

func TestOperatorCommands(t *testing.T) {
	if _, err := os.Stat("/run/reva/admin.sock"); err == nil {
		t.Skip("this machine has an admin socket")
	}
	const hint = "operator commands hidden: acme help --all"
	for _, tc := range []struct {
		name, mode string
		setup      func(t *testing.T, cfg string)
		shown      bool
		hint       bool
	}{
		{name: "show", mode: "show", shown: true},
		{name: "hide", mode: "hide"},
		{name: "hide with an admin host", mode: "hide", setup: func(t *testing.T, cfg string) { t.Setenv("ACME_ADMIN_HOST", "admin:9999") }},
		{name: "auto", mode: "auto", hint: true},
		{name: "auto with an admin host", mode: "auto", shown: true,
			setup: func(t *testing.T, cfg string) { t.Setenv("ACME_ADMIN_HOST", "admin:9999") }},
		{name: "auto with an elevate token", mode: "auto", shown: true, setup: func(t *testing.T, cfg string) {
			if err := os.MkdirAll(filepath.Join(cfg, "acme-cli"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfg, "acme-cli", "admin-token"), []byte(`{"login":"x","token":"y"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := isolate(t)
			if tc.setup != nil {
				tc.setup(t, cfg)
			}
			help, err := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme(tc.mode))), "--help")
			if err != nil {
				t.Fatal(err)
			}
			if shown := strings.Contains(help, "  service "); shown != tc.shown {
				t.Errorf("service shown = %v, want %v:\n%s", shown, tc.shown, help)
			}
			if tc.shown && !strings.Contains(help, "(admin)") {
				t.Errorf("an operator command is not marked:\n%s", help)
			}
			if got := strings.Contains(help, "8 "+hint); got != tc.hint {
				t.Errorf("hint = %v, want %v:\n%s", got, tc.hint, help)
			}
			if tc.shown {
				return
			}
			all, _ := execute(t, revacmd.NewRoot(revacmd.WithBrand(acme(tc.mode))), "help", "--all")
			if !strings.Contains(all, "  service ") || strings.Contains(all, hint) {
				t.Errorf("help --all:\n%s", all)
			}
			// A hidden command still runs: it fails for the missing login, not
			// as an unknown command.
			_, err = execute(t, revacmd.NewRoot(revacmd.WithBrand(acme(tc.mode))), "service", "list")
			if err == nil || !strings.Contains(err.Error(), "no login token: run `acme login` first") {
				t.Errorf("err = %v, want the missing login", err)
			}
		})
	}
}

func TestCheckBrand(t *testing.T) {
	isolate(t)
	if err := revacmd.CheckBrand(brand.Reva()); err != nil {
		t.Errorf("reva brand: %v", err)
	}
	if err := revacmd.CheckBrand(acme("auto")); err != nil {
		t.Errorf("acme brand: %v", err)
	}
	b := brand.MustParse([]byte("[program]\nname = \"acme\"\n[auth]\nchain = [\"token\", \"cern-sso\"]\n[ui]\nhide = [\"cs3\"]\n"))
	err := revacmd.CheckBrand(b)
	for _, want := range []string{`auth.chain: no sign-in method "cern-sso"`, `ui.hide: no help group "cs3"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}
