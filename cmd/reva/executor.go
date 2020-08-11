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
	"time"
)

// Executor provides exec command handler
type Executor struct {
	Timeout int
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
	for _, v := range commands {
		if v.Name == action {
			var err error
			if err = v.Parse(args[1:]); err != nil {
				fmt.Println(err)
				return
			}
			defer v.ResetFlags()

			// Provide a longer timeout for login as it requires user input
			timeout := e.Timeout
			if action == "login" {
				timeout = 12
			}

			c := make(chan error, 1)
			go func() {
				c <- v.Action()
			}()

			select {
			case err = <-c:
				if err != nil {
					fmt.Println(err)
				}
			case <-time.After(time.Duration(timeout * int(time.Second))):
				fmt.Println("Error: executing the command timed out.")
			}
			return
		}
	}

	fmt.Println("Invalid command. Use \"help\" to list the available commands.")
}
