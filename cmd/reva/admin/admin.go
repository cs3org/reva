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

// Package admin holds the fleet commands of the reva CLI: the operator
// surface of a running reva fleet, through the Admin API.
package admin

import (
	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/spf13/cobra"
)

// rt is set once by Commands. The CLI runs one command per process, so a
// package var is enough.
var rt *cli.Runtime

// Commands returns the fleet commands. They are operator commands: they
// always use the admin plane.
func Commands(r *cli.Runtime) []*cobra.Command {
	rt = r
	cmds := []*cobra.Command{
		serviceCommand(),
		invocationCommand(),
		invokeCommand(),
		logsCommand(),
		traceCommand(),
		stackCommand(),
		jobCommand(),
		impersonateCommand(),
	}
	for _, c := range cmds {
		cli.AdminCommand(c)
	}
	return cmds
}
