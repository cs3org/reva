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
// reva fleet.
package admin

import (
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/spf13/cobra"
)

// rt is set once by Command. The CLI runs one command per process, so a
// package var is enough.
var rt *cli.Runtime

// Command returns the `admin` command tree.
func Command(r *cli.Runtime) *cobra.Command {
	rt = r
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administer the reva fleet",
		Long: "Administer the reva fleet through the admin endpoint.\n\n" +
			"Without --admin-host, the commands first try the local admin socket\n" +
			"(/run/reva/admin.sock or $XDG_RUNTIME_DIR/reva/admin.sock), then the stored admin host.",
	}
	cmd.PersistentFlags().StringVar(&r.AdminHost, "admin-host", "", "address of the admin gRPC endpoint (env REVA_ADMIN_HOST)")
	cmd.AddCommand(
		elevateCommand(),
		servicesCommand(),
		configCommand(),
		invocationsCommand(),
		invokeCommand(),
		logsCommand(),
		jobsCommand(),
		traceCommand(),
		stackCommand(),
		impersonateCommand(),
	)
	return cmd
}
