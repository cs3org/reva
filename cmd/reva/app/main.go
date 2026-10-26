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

// Package revacmd is the reva CLI: the core commands, the command plugins
// that are compiled in, and Main. cmd/reva and the builds of gaia call Main.
package revacmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/admin"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/status"
)

// The build sets the version in pkg/rversion, the same for reva and revad.
var gitCommit, buildDate, version, goVersion = rversion.GitCommit, rversion.BuildDate, rversion.Version, rversion.GoVersion

// mutating are the commands that change data. Under --as they ask for a
// confirmation. cp asks only for an upload, in the command itself.
var mutating = []string{
	"mkdir", "mv", "rm", "touch", "trash restore", "trash purge", "lock set", "lock remove",
	"share create", "share update", "share remove", "share received update",
	"link create", "link update", "link remove",
	"ocm invite create", "ocm invite accept", "ocm share create", "ocm share update", "ocm share remove",
	"ocm received update", "ocm transfer cancel", "ocm transfer retry",
	"token create", "token revoke", "preferences set",
}

// Option changes the program that Main and NewRoot build.
//
// TODO(T40): WithBrand.
type Option func(*options)

type options struct{}

// Main runs the CLI with the args of the process, and exits.
func Main(opts ...Option) {
	os.Exit(run(os.Args[1:], opts...))
}

// NewRoot returns the command tree: the core commands and the command
// plugins that are compiled in. CheckPlugins tells which plugins it refused.
func NewRoot(opts ...Option) *cobra.Command {
	return rootCommand(&cli.Runtime{}, new(bool), opts...)
}

// run runs the command line args and returns the exit code: 2 for a usage
// error, 1 for any other error.
func run(args []string, opts ...Option) int {
	rt := &cli.Runtime{}
	// started is set when the command itself runs. An error before that comes
	// from cobra (unknown command, bad flag, wrong arguments) or from the
	// choice of the plane: a usage error.
	started := false
	root := rootCommand(rt, &started, opts...)
	for _, line := range refusedPlugins(root) {
		fmt.Fprintf(os.Stderr, "%s: %s\n", root.Name(), line)
	}
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		rt.WarnUnusedReason()
	}
	rt.Close()
	if err == nil {
		return 0
	}
	prefix := "Error:"
	if cli.UseColor(os.Stderr) {
		prefix = errorColor.Sprint(prefix)
	}
	fmt.Fprintln(os.Stderr, prefix, err)
	if s, ok := status.FromError(err); ok && rt.Debug() {
		fmt.Fprintf(os.Stderr, "  grpc code=%s details=%v\n", s.Code(), s.Details())
	}
	if !started {
		fmt.Fprintln(os.Stderr, "Run 'reva --help' for usage.")
		return 2
	}
	return 1
}

func rootCommand(rt *cli.Runtime, started *bool, opts ...Option) *cobra.Command {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	// A plugin with its own PersistentPreRunE must not replace the hook of
	// the root, which loads the settings.
	cobra.EnableTraverseRunHooks = true
	root := &cobra.Command{
		Use:           "reva",
		Short:         "Command line client for the reva gateway",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// cobra checks the required flags only after this hook, so check them
			// here to report them as usage errors.
			if err := cmd.ValidateRequiredFlags(); err != nil {
				return err
			}
			if err := cmd.ValidateFlagGroups(); err != nil {
				return err
			}
			if err := rt.SelectPlane(cmd); err != nil {
				return err
			}
			*started = true
			if err := rt.Load(cmd.Flags()); err != nil {
				return err
			}
			if rt.Timeout() > 0 {
				ctx, cancel := context.WithTimeout(cmd.Context(), rt.Timeout())
				cobra.OnFinalize(cancel)
				cmd.SetContext(ctx)
			}
			if cli.IsMutating(cmd) {
				return rt.ConfirmMutation(cmd.CommandPath())
			}
			return nil
		},
	}
	rt.AddFlags(root.PersistentFlags())

	root.AddGroup(
		&cobra.Group{ID: "files", Title: "Files:"},
		&cobra.Group{ID: "sharing", Title: "Sharing:"},
		&cobra.Group{ID: "storage", Title: "Storage:"},
		&cobra.Group{ID: "fleet", Title: "Fleet:"},
		&cobra.Group{ID: "session", Title: "Session:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("files",
		lsCommand(rt), statCommand(rt), mkdirCommand(rt), mvCommand(rt), rmCommand(rt), touchCommand(rt),
		cpCommand(rt), versionsCommand(rt), trashCommand(rt), lockCommand(rt),
	)
	add("sharing", shareCommand(rt), linkCommand(rt), ocmCommand(rt))
	add("storage", admin.RouteCommand(rt))
	add("fleet", admin.Commands(rt)...)
	add("session",
		loginCommand(rt), logoutCommand(rt), statusCommand(rt), whoamiCommand(rt), configCommand(rt),
		admin.ElevateCommand(rt), openCommand(rt), tokenCommand(rt), preferencesCommand(rt), versionCommand(rt),
		completionCommand(),
	)
	for _, p := range mutating {
		c, _, err := root.Find(strings.Fields(p))
		if err != nil || c.CommandPath() != "reva "+p {
			panic("reva: no command " + p)
		}
		cli.Mutates(c)
	}
	root.SetHelpCommandGroupID("session")
	root.CompletionOptions.DisableDefaultCmd = true
	cli.OperatorHelp(root)
	colorHelp(root)
	attachPlugins(rt, root)
	return root
}

type versionResult struct {
	Version   string      `json:"version"`
	Commit    string      `json:"commit"`
	GoVersion string      `json:"go_version"`
	BuildDate string      `json:"build_date"`
	Brand     string      `json:"brand,omitempty"`
	Plugins   []pluginRow `json:"plugins,omitempty"`
}

func versionCommand(rt *cli.Runtime) *cobra.Command {
	var plugins bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of this tool",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := versionResult{Version: version, Commit: gitCommit, GoVersion: goVersion, BuildDate: buildDate}
			if plugins {
				// TODO(T40): the name of the brand file.
				v.Brand = cmd.Root().Name()
				v.Plugins = compiledPlugins()
			}
			return rt.Print(cmd.OutOrStdout(), v, func(w io.Writer) {
				fmt.Fprintf(w, "version=%s commit=%s go_version=%s build_date=%s\n", v.Version, v.Commit, v.GoVersion, v.BuildDate)
				if !plugins {
					return
				}
				fmt.Fprintf(w, "brand=%s\n", v.Brand)
				if len(v.Plugins) == 0 {
					fmt.Fprintln(w, "no CLI plugins")
					return
				}
				t := cli.Table{Headers: []string{"Plugin", "Package", "Version"}}
				for _, p := range v.Plugins {
					t.Rows = append(t.Rows, []string{p.ID, p.Package, p.Version})
				}
				t.Write(w)
			})
		},
	}
	cmd.Flags().BoolVar(&plugins, "plugins", false, "also list the CLI plugins that are compiled in, and the brand")
	return cmd
}

// formatError returns the error of a CS3 status that is not OK.
func formatError(status *rpc.Status) error {
	return cli.Check(status)
}
