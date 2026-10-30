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

package revacmd

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

const (
	pluginAnnotation = "reva.plugin"
	// refusedAnnotation on the root holds one line per refused plugin.
	refusedAnnotation = "reva.plugins.refused"
	pluginsGroup      = "plugins"
)

// attachPlugins needs parents before children: GetPlugins sorts by id. A
// refused plugin is a defect of the build. The CLI tells it and goes on, and
// CheckPlugins fails the build.
func attachPlugins(rt *cli.Runtime, root *cobra.Command) {
	var lines, refused []string
	for _, p := range reva.GetPlugins(cli.CommandsNamespace) {
		if err := attachPlugin(rt, root, p, refused); err != nil {
			lines = append(lines, fmt.Sprintf("plugin %s refused: %v", p.ID, err))
			refused = append(refused, string(p.ID))
		}
	}
	if len(lines) > 0 {
		if root.Annotations == nil {
			root.Annotations = map[string]string{}
		}
		root.Annotations[refusedAnnotation] = strings.Join(lines, "\n")
	}
}

func attachPlugin(rt *cli.Runtime, root *cobra.Command, p reva.PluginInfo, refused []string) error {
	for _, id := range refused {
		if strings.HasPrefix(string(p.ID), id+".") {
			return fmt.Errorf("its parent %s is refused", id)
		}
	}
	newCmd, ok := p.New.(func(*cli.Runtime) *cobra.Command)
	if !ok {
		return fmt.Errorf("its New is a %T, not a func(*cli.Runtime) *cobra.Command", p.New)
	}
	labels := strings.Split(strings.TrimPrefix(string(p.ID), cli.CommandsNamespace+"."), ".")
	parent := root
	for i, l := range labels[:len(labels)-1] {
		// Only the name counts, not an alias, so that the path of a plugin
		// names one command.
		next := subcommand(parent, l, false)
		if next == nil {
			if c := subcommand(parent, l, true); c != nil {
				return fmt.Errorf("%q is an alias: use the name %q", l, c.Name())
			}
			return fmt.Errorf("no parent command `%s`", strings.Join(labels[:i+1], " "))
		}
		parent = next
	}
	name := labels[len(labels)-1]
	cmd := newCmd(rt)
	switch {
	case cmd == nil:
		return errors.New("its New returned no command")
	case cmd.Name() != name:
		return fmt.Errorf("its command is named %q, not %q", cmd.Name(), name)
	}
	for _, n := range append([]string{cmd.Name()}, cmd.Aliases...) {
		if c := subcommand(parent, n, true); c != nil {
			return fmt.Errorf("%q is taken by `%s`", n, c.CommandPath())
		}
	}
	switch {
	case cmd.GroupID != "" && !parent.ContainsGroup(cmd.GroupID):
		return fmt.Errorf("`%s` has no help group %q", parent.CommandPath(), cmd.GroupID)
	case cmd.GroupID == "" && len(parent.Groups()) > 0:
		if !parent.ContainsGroup(pluginsGroup) {
			parent.AddGroup(&cobra.Group{ID: pluginsGroup, Title: "Plugins:"})
		}
		cmd.GroupID = pluginsGroup
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[pluginAnnotation] = string(p.ID)
	parent.AddCommand(cmd)
	return nil
}

func subcommand(parent *cobra.Command, n string, alias bool) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == n || (alias && slices.Contains(c.Aliases, n)) {
			return c
		}
	}
	return nil
}

func refusedPlugins(root *cobra.Command) []string {
	if s := root.Annotations[refusedAnnotation]; s != "" {
		return strings.Split(s, "\n")
	}
	return nil
}

// CheckPlugins returns the defects of the command plugins in root, a tree
// from NewRoot. The CI of a plugin module calls it, so that a defect fails the
// build and not the users.
func CheckPlugins(root *cobra.Command) error {
	var errs []error
	for _, l := range refusedPlugins(root) {
		errs = append(errs, errors.New(l))
	}
	var walk func(c *cobra.Command, id string)
	walk = func(c *cobra.Command, id string) {
		if v := c.Annotations[pluginAnnotation]; v != "" {
			id = v
		}
		if id != "" {
			if c.PersistentPreRun != nil || c.PersistentPreRunE != nil {
				errs = append(errs, fmt.Errorf("plugin %s: `%s` sets a persistent pre-run hook: use PreRunE", id, c.CommandPath()))
			}
			seen := map[string]bool{}
			for _, sub := range c.Commands() {
				for _, n := range append([]string{sub.Name()}, sub.Aliases...) {
					if seen[n] {
						errs = append(errs, fmt.Errorf("plugin %s: two commands of `%s` are named %q", id, c.CommandPath(), n))
					}
					seen[n] = true
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub, id)
		}
	}
	walk(root, "")
	return errors.Join(errs...)
}

type pluginRow struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Version string `json:"version"`
}

func compiledPlugins() []pluginRow {
	bi, _ := debug.ReadBuildInfo()
	var rows []pluginRow
	for _, p := range reva.GetPlugins("cli") {
		pkg := packageOf(p.New)
		rows = append(rows, pluginRow{ID: string(p.ID), Package: pkg, Version: moduleVersion(bi, pkg)})
	}
	return rows
}

// packageOf cuts the name of f at the first dot after the last slash.
func packageOf(f any) string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	slash := strings.LastIndex(name, "/")
	if i := strings.Index(name[slash+1:], "."); i >= 0 {
		return name[:slash+1+i]
	}
	return name
}

func moduleVersion(bi *debug.BuildInfo, pkg string) string {
	if bi == nil {
		return "unknown"
	}
	var best *debug.Module
	for _, m := range append([]*debug.Module{&bi.Main}, bi.Deps...) {
		if (pkg == m.Path || strings.HasPrefix(pkg, m.Path+"/")) && (best == nil || len(m.Path) > len(best.Path)) {
			best = m
		}
	}
	switch {
	case best == nil:
		return "unknown"
	case best.Replace != nil:
		return strings.TrimSpace(best.Replace.Path + " " + best.Replace.Version)
	}
	return best.Version
}
