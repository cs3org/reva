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

package main

import (
	"fmt"
	"os"
	"strings"
)

// Executor provides exec command handler
type Executor struct {
	Commands []*command
}

// Execute provides execute commands
func (e *Executor) Execute(s string) {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return
	case "exit", "quit":
		os.Exit(0)
	}

	args := strings.Split(s, " ")

	action := args[0]
	for _, v := range e.Commands {
		if v.Name == action {
			if err := v.Parse(args[1:]); err != nil {
				fmt.Println(err)
				return
			}
			defer v.Parse([]string{"-list=false"})
			err := v.Action()
			if err != nil {
				fmt.Println(err)
			}
			return
		}
	}

	fmt.Println("Invalid command")
}
