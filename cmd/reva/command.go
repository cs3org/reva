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
	"flag"
	"fmt"
)

// command is the representation to create commands
type command struct {
	*flag.FlagSet
	Name        string
	Action      func() error
	Usage       func() string
	Description func() string
}

// newCommand creates a new command
func newCommand(name string) *command {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	cmd := &command{
		Name: name,
		Usage: func() string {
			return fmt.Sprintf("Usage: %s", name)
		},
		Action: func() error {
			fmt.Println("Hello REVA")
			return nil
		},
		Description: func() string {
			return "TODO description"
		},
		FlagSet: fs,
	}
	return cmd
}
