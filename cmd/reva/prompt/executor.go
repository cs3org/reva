// Copyright 2018-2020 CERN
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

package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/cs3org/reva/cmd/reva/command"
)

// Executor provides exec command handler
type Executor struct {
	Commands []*command.Command
}

// Do provide completion to prompt
func (e *Executor) Do(s string) {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return
	case "exit", "quit":
		os.Exit(0)
	case "help":
		e.helpExecutor()
		return
	}

	args := strings.Split(s, " ")

	action := args[0]
	for _, v := range e.Commands {
		if v.Name == action {
			if err := v.Parse(args[1:]); err != nil {
				fmt.Println(err)
				return
			}
			err := v.Action()
			if err != nil {
				fmt.Println(err)
			}
			return
		}
	}

	fmt.Println("Invalid command")
}

func (e *Executor) helpExecutor() {
	n := 0
	for _, cmd := range e.Commands {
		l := len(cmd.Name)
		if l > n {
			n = l
		}
	}

	usage := "Command line interface to REVA:\n"
	for _, cmd := range e.Commands {
		usage += fmt.Sprintf("%s%s%s\n", cmd.Name, strings.Repeat(" ", 4+(n-len(cmd.Name))), cmd.Description())
	}
	usage += fmt.Sprintf("%s%s%s\n", "help", strings.Repeat(" ", n), "help for using reva CLI")
	usage += "\nThe REVA authors"
	fmt.Println(usage)
}
