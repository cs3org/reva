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

// Package admin is the `reva admin` CLI: the operator surface for a running
// reva fleet. It is a self-contained sub-CLI — the top-level reva command hands
// it the transport flags and access to the persisted admin/login state through
// Options, and calls Dispatch.
package admin

import (
	"fmt"
	"strings"
)

// Options carries what the admin CLI needs from the top-level reva command: the
// transport flags and access to the persisted admin host and login token.
type Options struct {
	Insecure         bool
	SkipVerify       bool
	AdminHost        func() string          // the stored admin host, "" if unset
	PersistAdminHost func(string)           // persist a new admin host
	LoginToken       func() (string, error) // the caller's login token, for elevate
}

// cliOpts is set once per Dispatch. The CLI runs one command per process, so a
// package var mirrors the top-level command's own flag globals.
var cliOpts Options

// adminCommands is the sub-table dispatched by Dispatch.
var adminCommands = []*command{
	adminElevateCommand(),
	adminServicesCommand(),
	adminConfigCommand(),
	adminInvocationsCommand(),
	adminInvokeCommand(),
	adminLogsCommand(),
	adminJobsCommand(),
	adminTraceCommand(),
	adminStackCommand(),
	adminImpersonateCommand(),
}

// Dispatch runs `admin <subcommand> [flags] [args]`; args is everything after
// "admin". It is the single entry point the top-level reva command calls.
func Dispatch(args []string, o Options) error {
	cliOpts = o
	if len(args) == 0 {
		printUsage()
		return nil
	}
	name := args[0]
	for _, sub := range adminCommands {
		if sub.Name == name {
			if err := sub.Parse(args[1:]); err != nil {
				return err
			}
			defer sub.ResetFlags()
			return sub.Action()
		}
	}
	return fmt.Errorf("unknown admin subcommand %q; run `admin` to list them", name)
}

func printUsage() {
	fmt.Println("Usage: admin <subcommand> [flags] [args]")
	fmt.Println("Subcommands:")
	n := 0
	for _, sub := range adminCommands {
		if len(sub.Name) > n {
			n = len(sub.Name)
		}
	}
	for _, sub := range adminCommands {
		fmt.Printf("  %s%s%s\n", sub.Name, strings.Repeat(" ", 2+(n-len(sub.Name))), sub.Description())
	}
}

// Subcommands returns the admin subcommands' names and descriptions, for the
// top-level shell completer.
func Subcommands() [][2]string {
	out := make([][2]string, 0, len(adminCommands))
	for _, c := range adminCommands {
		out = append(out, [2]string{c.Name, c.Description()})
	}
	return out
}
