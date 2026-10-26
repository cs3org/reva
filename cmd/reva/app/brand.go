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
	"regexp"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/brand"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	// hiddenAnnotation marks what help hides because of the brand: "operator"
	// or "group". help --all shows it again.
	hiddenAnnotation = "reva.hidden"
	helpAnnotation   = "reva.help"
	// brandCheckArg makes Main check the brand and the plugins of the build
	// and exit, so that gaia can check a build with plugins before it ships:
	// go run . __brandcheck.
	brandCheckArg = "__brandcheck"
)

// WithBrand builds the program of the brand b: its name, its defaults, its
// env names and its texts. A build embeds the brand file:
//
//	//go:embed brand.toml
//	var embedded []byte
//
//	func main() { revacmd.Main(revacmd.WithBrand(brand.MustParse(embedded))) }
func WithBrand(b *brand.Brand) Option {
	return func(o *options) { o.brand = b }
}

// CheckBrand checks b against the build it runs in: every provider of
// auth.chain is built in or a cli.auth.* plugin, and every group of ui.hide
// is a help group of the root.
func CheckBrand(b *brand.Brand) error {
	var errs []error
	providers := cli.AuthProviders()
	for _, p := range b.Auth.Chain {
		if !slices.Contains(providers, p) {
			errs = append(errs, fmt.Errorf("auth.chain: no sign-in method %q in this build (it has %s)", p, strings.Join(providers, ", ")))
		}
	}
	if t := b.Connection.Transport; t != "" && !slices.Contains(brand.Transports, t) {
		errs = append(errs, fmt.Errorf("connection.transport: unknown transport %q", t))
	}
	root := NewRoot(WithBrand(b))
	var groups []string
	for _, g := range root.Groups() {
		groups = append(groups, g.ID)
	}
	for _, g := range b.UI.Hide {
		if !slices.Contains(groups, g) {
			errs = append(errs, fmt.Errorf("ui.hide: no help group %q (the groups are %s)", g, strings.Join(groups, ", ")))
		}
	}
	return errors.Join(errs...)
}

// commandName matches the program name where a text shows a command line.
var commandName = regexp.MustCompile("(^|[\\s'\"`(])reva ")

// applyBrand gives the tree of the core and the plugins the name and the
// texts of the brand, and hides what the brand keeps out of help.
func applyBrand(rt *cli.Runtime, root *cobra.Command) {
	b := rt.Brand()
	root.Short = b.Program.Short
	root.Long = b.Program.Short
	if b.Program.DocsURL != "" {
		root.Long += "\n\nDocumentation: " + b.Program.DocsURL
	}
	if b.Program.SupportURL != "" {
		root.Long += "\nSupport:       " + b.Program.SupportURL
	}
	name := b.Name()
	walk(root, func(c *cobra.Command) {
		if name != "reva" && c != root {
			for _, s := range []*string{&c.Short, &c.Long, &c.Example} {
				*s = commandName.ReplaceAllString(*s, "${1}"+name+" ")
			}
		}
		if c.Parent() == root && slices.Contains(b.UI.Hide, c.GroupID) {
			hide(c, "group")
		}
	})
	if !rt.ShowOperators() {
		// Only auto tells how many it hides: hide is for a build whose users
		// never need them.
		why := "operator"
		if mode, _ := rt.Setting("operator_commands"); mode == "hide" {
			why = "operator-quiet"
		}
		walk(root, func(c *cobra.Command) {
			if cli.IsOperator(c) {
				hide(c, why)
			}
			c.Flags().VisitAll(func(f *pflag.Flag) {
				if cli.IsOperatorFlag(f) && !f.Hidden {
					f.Hidden = true
					if f.Annotations == nil {
						f.Annotations = map[string][]string{}
					}
					f.Annotations[hiddenAnnotation] = []string{"operator"}
				}
			})
		})
	}
	root.SetHelpCommand(helpCommand())
}

func walk(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walk(sub, fn)
	}
}

func hide(c *cobra.Command, why string) {
	if c.Hidden {
		return
	}
	c.Hidden = true
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations[hiddenAnnotation] = why
}

// showAll shows again what applyBrand hid, for help --all.
func showAll(root *cobra.Command) {
	walk(root, func(c *cobra.Command) {
		if c.Annotations[hiddenAnnotation] != "" {
			c.Hidden = false
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if len(f.Annotations[hiddenAnnotation]) > 0 {
				f.Hidden = false
			}
		})
	})
}

// helpCommand replaces the help command of cobra, to add --all.
func helpCommand() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long: "Help about any command. --all also shows what help hides: the operator\n" +
			"commands and the hidden groups. Hidden commands run all the same.",
		GroupID:     "session",
		Annotations: map[string]string{helpAnnotation: "true"},
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			parent, _, err := c.Root().Find(args)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out []cobra.Completion
			for _, sub := range parent.Commands() {
				if sub.IsAvailableCommand() && strings.HasPrefix(sub.Name(), toComplete) {
					out = append(out, cobra.CompletionWithDesc(sub.Name(), sub.Short))
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			if all {
				showAll(c.Root())
			}
			target, _, err := c.Root().Find(args)
			if err != nil {
				return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "also show the operator commands and the hidden groups")
	return cmd
}

// helpTemplate edits the usage template of root: a help group with no
// command to show has no title, and a command with hidden operator
// subcommands says how many there are.
func helpTemplate(root *cobra.Command) {
	cobra.AddTemplateFunc("groupHasCommands", func(c *cobra.Command, id string) bool {
		return slices.ContainsFunc(c.Commands(), func(s *cobra.Command) bool {
			return s.GroupID == id && (s.IsAvailableCommand() || s.Name() == "help")
		})
	})
	cobra.AddTemplateFunc("operatorHint", operatorHint)
	const group = `{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}`
	const groupIfAny = `{{range $group := .Groups}}{{if groupHasCommands $ $group.ID}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}`
	t := strings.Replace(root.UsageTemplate(), group, groupIfAny, 1)
	t = strings.TrimSuffix(t, "\n") + "{{with operatorHint .}}\n\n{{.}}{{end}}\n"
	root.SetUsageTemplate(t)
}

func operatorHint(c *cobra.Command) string {
	n := 0
	for _, sub := range c.Commands() {
		if sub.Hidden && sub.Annotations[hiddenAnnotation] == "operator" {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	what := "operator commands"
	if n == 1 {
		what = "operator command"
	}
	help := c.Root().Name() + " help --all"
	if c.HasParent() {
		help += " " + strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")
	}
	return fmt.Sprintf("%d %s hidden: %s", n, what, help)
}
