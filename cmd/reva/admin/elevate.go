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

package admin

import (
	"time"

	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
)

// ElevateCommand returns `elevate`. It is in the session group: it changes
// how the next commands get their admin rights.
func ElevateCommand(r *cli.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "elevate",
		Short: "Get an admin token for the next commands",
		Long: "Exchange the login token for a short-lived admin token, and store it.\n\n" +
			"Without elevate, a command that needs admin rights asks for an admin token\n" +
			"itself, and keeps it in memory for that command only. The stored token is\n" +
			"used only while the same login is current. On a host with the local admin\n" +
			"socket, no token is needed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := r.Elevate(cmd.Context())
			if err != nil {
				return adminErr(err)
			}
			if exp.IsZero() {
				r.Msg(cmd.ErrOrStderr(), "Elevated.")
				return nil
			}
			r.Msg(cmd.ErrOrStderr(), "Elevated: the admin token is valid until %s (%s).",
				exp.Local().Format(time.RFC3339), cli.Left(exp))
			return nil
		},
	}
}
