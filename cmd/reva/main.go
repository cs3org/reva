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
	"context"
	"fmt"
	"os"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/admin"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/status"
)

var gitCommit, buildDate, version, goVersion string

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

func main() {
	os.Exit(run(os.Args[1:]))
}

// run runs the command line args and returns the exit code: 2 for a usage
// error, 1 for any other error.
func run(args []string) int {
	rt := &cli.Runtime{}
	// started is set when the command itself runs. An error before that comes
	// from cobra (unknown command, bad flag, wrong arguments) or from the
	// choice of the plane: a usage error.
	started := false
	root := rootCommand(rt, &started)
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
	if s, ok := status.FromError(err); ok && rt.Debug {
		fmt.Fprintf(os.Stderr, "  grpc code=%s details=%v\n", s.Code(), s.Details())
	}
	if !started {
		fmt.Fprintln(os.Stderr, "Run 'reva --help' for usage.")
		return 2
	}
	return 1
}

func rootCommand(rt *cli.Runtime, started *bool) *cobra.Command {
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
			if rt.Timeout > 0 {
				ctx, cancel := context.WithTimeout(cmd.Context(), rt.Timeout)
				cobra.OnFinalize(cancel)
				cmd.SetContext(ctx)
			}
			if cli.IsMutating(cmd) {
				return rt.ConfirmMutation(cmd.CommandPath())
			}
			return nil
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&rt.GatewayAddr, "gateway", "", "address of the gateway, host:port (env REVA_GATEWAY)")
	rt.TransportFlags(pf)
	pf.StringVar(&rt.AdminHost, "admin-host", "", "address of the Admin API, host:port or unix:///path (env REVA_ADMIN_HOST; default the gateway)")
	pf.BoolVar(&rt.Insecure, "insecure", false, "allow plaintext, without TLS, on the selected transport (development only)")
	pf.BoolVar(&rt.SkipVerify, "skip-verify", false, "do not verify the server certificate (development only)")
	pf.StringVar(&rt.TokenValue, "token", "", "use this token instead of signing in (env REVA_TOKEN)")
	pf.StringVar(&rt.AppTokenFile, "app-token-file", "", "file holding an app token, to sign in with --user")
	pf.StringVar(&rt.User, "user", "", "user name, for app-token and basic sign-in")
	pf.StringVar(&rt.As, "as", "", "run the command as this user (impersonation, needs admin rights)")
	pf.StringVar(&rt.Reason, "reason", "", "why you use admin rights, written to the audit log")
	pf.BoolVarP(&rt.Yes, "yes", "y", false, "do not ask for confirmations")
	pf.BoolVar(&rt.NoElevate, "no-elevate", false, "never get an admin token automatically (env REVA_NO_ELEVATE)")
	pf.DurationVar(&rt.Timeout, "timeout", 0, "overall timeout for the command, for example 30s (0 for none)")
	pf.StringVarP(&rt.Output, "output", "o", "table", "output format: table or json")
	pf.BoolVarP(&rt.Quiet, "quiet", "q", false, "hide progress messages")
	pf.BoolVar(&rt.Debug, "debug", false, "show the underlying error and request details")
	pf.BoolVar(&rt.NoProgress, "no-progress", false, "hide the progress bar")

	root.AddGroup(
		&cobra.Group{ID: "files", Title: "Files:"},
		&cobra.Group{ID: "sharing", Title: "Sharing:"},
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
	add("fleet", admin.Commands(rt)...)
	add("session",
		loginCommand(rt), logoutCommand(rt), statusCommand(rt), whoamiCommand(rt), configCommand(rt),
		admin.ElevateCommand(rt), openCommand(rt), tokenCommand(rt), preferencesCommand(rt), versionCommand(),
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
	return root
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of this tool",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "version=%s commit=%s go_version=%s build_date=%s\n", version, gitCommit, goVersion, buildDate)
		},
	}
}

// formatError returns the error of a CS3 status that is not OK.
func formatError(status *rpc.Status) error {
	return cli.Check(status)
}
