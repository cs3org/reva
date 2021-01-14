// Copyright 2018-2021 CERN
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
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

var genCommand = func() *command {
	cmd := newCommand("gen")
	cmd.Description = func() string { return "generates files for configuration" }
	cmd.Usage = func() string { return "Usage: gen <subcommand>" }

	subcmds := []*command{
		genConfigSubCommand(),
		genUsersSubCommand(),
	}

	cmd.Action = func(w ...io.Writer) error {
		// Verify that a subcommand has been provided
		// cmd.Args()[0] is the subcommand command
		// cmd.Args()[1] will be the subcommands arguments
		if len(cmd.Args()) < 1 {
			return errors.New("Invalid arguments. " + createGenUsage(subcmds))
		}
		subcommand := cmd.Args()[0]
		for _, v := range subcmds {
			if v.Name == subcommand {
				err := v.Parse(cmd.Args()[1:])
				if err != nil {
					return err
				}
				return v.Action()
			}
		}
		return errors.New("Invalid arguments. " + cmd.Usage())
	}
	return cmd
}

func createGenUsage(cmds []*command) string {
	n := 0
	for _, cmd := range cmds {
		l := len(cmd.Name)
		if l > n {
			n = l
		}
	}

	usage := "Available sub commands:\n\n"
	for _, cmd := range cmds {
		usage += fmt.Sprintf("gen %s%s%s\n", cmd.Name, strings.Repeat(" ", 4+(n-len(cmd.Name))), cmd.Description())
	}
	return usage
}
