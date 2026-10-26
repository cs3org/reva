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
	"strings"
	"testing"

	revacmd "github.com/cs3org/reva/v3/cmd/reva/app"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

// The fake plugins. The registry is global, so every test of this package
// sees all of them.
var preRunRan bool

func fake(use string) func(*cli.Runtime) *cobra.Command {
	return func(*cli.Runtime) *cobra.Command {
		return &cobra.Command{Use: use, RunE: func(*cobra.Command, []string) error { return nil }}
	}
}

func init() {
	cli.RegisterCommand("toolprobe", fake("toolprobe"))
	cli.RegisterCommand("user account", fake("account"))
	cli.RegisterCommand("share probe", fake("probe"))
	cli.RegisterCommand("fleetprobe", func(rt *cli.Runtime) *cobra.Command {
		c := fake("fleetprobe")(rt)
		c.GroupID = "fleet"
		return c
	})
	cli.RegisterCommand("ls", fake("ls"))
	cli.RegisterCommand("ocm users", fake("users"))
	cli.RegisterCommand("ocm users child", fake("child"))
	cli.RegisterCommand("nosuch child", fake("child"))
	cli.RegisterCommand("misnamed", fake("other"))
	cli.RegisterCommand("badgroup", func(rt *cli.Runtime) *cobra.Command {
		c := fake("badgroup")(rt)
		c.GroupID = "nosuch"
		return c
	})
	cli.RegisterCommand("prerun", func(rt *cli.Runtime) *cobra.Command {
		c := fake("prerun")(rt)
		c.PersistentPreRunE = func(*cobra.Command, []string) error {
			preRunRan = true
			return nil
		}
		return c
	})
}

func find(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	c, _, err := root.Find(strings.Fields(path))
	if err != nil || c.CommandPath() != "reva "+path {
		t.Fatalf("no command `reva %s` (found %v, %v)", path, c.CommandPath(), err)
	}
	return c
}

func TestPluginsAreAttached(t *testing.T) {
	root := revacmd.NewRoot()
	if c := find(t, root, "toolprobe"); c.GroupID != "plugins" || !root.ContainsGroup("plugins") {
		t.Errorf("a plugin without a group is in the group %q, want plugins", c.GroupID)
	}
	find(t, root, "share probe")
	if c := find(t, root, "fleetprobe"); c.GroupID != "fleet" {
		t.Errorf("the group of the plugin changed to %q", c.GroupID)
	}
	if c := find(t, root, "ls"); c.Short == "" {
		t.Error("a plugin replaced the core command ls")
	}
}

// The CERN plugin user account (reva-plugins) attaches under the core
// command user.
func TestPluginAttachesUnderUser(t *testing.T) {
	root := revacmd.NewRoot()
	user := find(t, root, "user")
	if user.GroupID != "users" || user.Short == "" {
		t.Errorf("`reva user` is not the core command: group %q", user.GroupID)
	}
	find(t, root, "user account")
	for _, core := range []string{"get", "find", "groups"} {
		find(t, root, "user "+core)
	}
}

func TestCheckPlugins(t *testing.T) {
	root := revacmd.NewRoot()
	err := revacmd.CheckPlugins(root)
	if err == nil {
		t.Fatal("no defects found")
	}
	for _, want := range []string{
		"plugin cli.commands.ls refused: \"ls\" is taken by `reva ls`",
		"plugin cli.commands.ocm.users refused: \"users\" is taken by `reva ocm contacts`",
		"plugin cli.commands.ocm.users.child refused: its parent cli.commands.ocm.users is refused",
		"plugin cli.commands.nosuch.child refused: no parent command `nosuch`",
		"plugin cli.commands.misnamed refused: its command is named \"other\", not \"misnamed\"",
		"plugin cli.commands.badgroup refused: `reva` has no help group \"nosuch\"",
		"plugin cli.commands.prerun: `reva prerun` sets a persistent pre-run hook",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing defect %q in:\n%v", want, err)
		}
	}
	for _, ok := range []string{"cli.commands.toolprobe", "cli.commands.user.account", "cli.commands.share.probe", "cli.commands.fleetprobe"} {
		if strings.Contains(err.Error()+" ", ok) {
			t.Errorf("a good plugin is reported: %s", ok)
		}
	}
	if c, _, _ := root.Find([]string{"nosuch"}); c != root {
		t.Errorf("a plugin without a parent is attached: %s", c.CommandPath())
	}
}

func TestRegisterCommandPanics(t *testing.T) {
	for name, path := range map[string]string{"duplicate": "user account", "dot": "otg.set", "empty": " "} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterCommand(%q) did not panic", path)
				}
			}()
			cli.RegisterCommand(path, fake("x"))
		})
	}
}

// The hook of the root loads the settings and checks them, so a plugin with
// its own hook must not replace it.
func TestPluginPreRunKeepsTheRootHook(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	preRunRan = false
	root := revacmd.NewRoot()
	root.SetArgs([]string{"-o", "bogus", "prerun"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "unknown output") {
		t.Errorf("err = %v: the hook of the root did not run", err)
	}
	if preRunRan {
		t.Error("the hook of the plugin ran after the hook of the root failed")
	}

	root = revacmd.NewRoot()
	root.SetArgs([]string{"prerun"})
	if err := root.Execute(); err != nil || !preRunRan {
		t.Errorf("err = %v, ran = %v: the hook of the plugin did not run", err, preRunRan)
	}
}

func TestVersionListsPlugins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := revacmd.NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "--plugins"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"brand=reva", "cli.commands.user.account", "github.com/cs3org/reva/v3/cmd/reva/app_test"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in:\n%s", want, out.String())
		}
	}
}
