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
	"os"
	"strings"

	"github.com/cs3org/reva/cmd/reva/gen"
)

var genUsersSubCommand = func() *command {
	cmd := newCommand("users")
	cmd.Description = func() string { return "will create a users.json file with demo users" }
	cmd.Usage = func() string { return "Usage: gen users [-flags]" }

	forceFlag := cmd.Bool("f", false, "force")
	usersFlag := cmd.String("c", "./users.json", "path to the usersfile")

	cmd.ResetFlags = func() {
		*forceFlag, *usersFlag = false, "./users.json"
	}

	cmd.Action = func(w ...io.Writer) error {
		if !*forceFlag {
			if _, err := os.Stat(*usersFlag); err == nil {
				// file exists, overwrite?
				fmt.Fprintf(os.Stdout, "%s exists, overwrite (y/N)? ", *usersFlag)
				var r string
				_, err := fmt.Scanln(&r)
				if err != nil || "y" != strings.ToLower(r[:1]) {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if _, err := os.Stat(*usersFlag); os.IsNotExist(err) {
			gen.WriteUsers(*usersFlag, nil)
		}
		return nil
	}
	return cmd
}
