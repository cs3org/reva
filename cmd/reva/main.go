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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/cmd/reva/admin"
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/spf13/cobra"
)

var gitCommit, buildDate, version, goVersion string

func main() {
	rt := &cli.Runtime{}
	// started is set when the command itself runs. An error before that comes
	// from cobra (unknown command, bad flag, wrong arguments): a usage error.
	started := false

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
			started = true
			if err := rt.Load(cmd.Flags()); err != nil {
				return err
			}
			if rt.Timeout > 0 {
				ctx, cancel := context.WithTimeout(cmd.Context(), rt.Timeout)
				cobra.OnFinalize(cancel)
				cmd.SetContext(ctx)
			}
			return nil
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&rt.Host, "host", "", "address of the gateway (env REVA_HOST)")
	pf.BoolVar(&rt.Insecure, "insecure", false, "disable transport security for gRPC")
	pf.BoolVar(&rt.SkipVerify, "skip-verify", false, "do not verify the server certificate (gRPC and HTTP)")
	pf.StringVar(&rt.TokenFile, "token-file", "", "file that holds the login token (env REVA_TOKEN_FILE)")
	pf.DurationVar(&rt.Timeout, "timeout", 0, "stop the command after this duration, for example 30s (0 means no limit)")
	pf.StringVarP(&rt.Output, "output", "o", "text", "output format: text or json")

	root.AddGroup(
		&cobra.Group{ID: "files", Title: "Files:"},
		&cobra.Group{ID: "sharing", Title: "Sharing:"},
		&cobra.Group{ID: "other", Title: "Other:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("files",
		lsCommand(rt), statCommand(rt), mkdirCommand(rt), mvCommand(rt), rmCommand(rt), touchCommand(rt),
		uploadCommand(rt), downloadCommand(rt), versionsCommand(rt), recycleCommand(rt), lockCommand(rt),
	)
	add("sharing", shareCommand(rt), publicShareCommand(rt), ocmCommand(rt))
	add("other",
		loginCommand(rt), whoamiCommand(rt), configCommand(rt), openCommand(rt), appPasswordCommand(rt),
		preferencesCommand(rt), transferCommand(rt), admin.Command(rt), versionCommand(),
	)
	root.SetHelpCommandGroupID("other")
	root.SetCompletionCommandGroupID("other")
	colorHelp(root)

	err := root.Execute()
	rt.Close()
	if err != nil {
		prefix := "Error:"
		if cli.UseColor(os.Stderr) {
			prefix = errorColor.Sprint(prefix)
		}
		fmt.Fprintln(os.Stderr, prefix, err)
		if !started {
			fmt.Fprintln(os.Stderr, "Run 'reva --help' for usage.")
			os.Exit(2)
		}
		os.Exit(1)
	}
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

func formatError(status *rpc.Status) error {
	return fmt.Errorf("error: code=%+v msg=%q support_trace=%q", status.Code, status.Message, status.Trace)
}
