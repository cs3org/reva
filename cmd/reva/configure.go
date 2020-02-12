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
	"bufio"
	"fmt"
	"os"
)

var configureCommand = func() *command {
	cmd := newCommand("configure")
	cmd.Description = func() string { return "configure the reva client" }
	cmd.Action = func() error {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("host: ")
		text, err := read(reader)
		if err != nil {
			return err
		}

		c := &config{Host: text}
		if err := writeConfig(c); err != nil {
			panic(err)
		}
		fmt.Println("config saved in ", getConfigFile())
		return nil
	}
	return cmd
}
