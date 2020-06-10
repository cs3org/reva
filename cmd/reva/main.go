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
	"os"
	"strings"
)

var (
	conf *config

	gitCommit, buildDate, version, goVersion string

	insecure, skipverify bool
)

func init() {
	flag.BoolVar(&insecure, "insecure", false, "disables grpc transport security")
	flag.BoolVar(&skipverify, "skip-verify", false, "whether a client verifies the server's certificate chain and host name.")
	flag.Parse()

}

func main() {

	cmds := []*command{
		versionCommand(),
		configureCommand(),
		loginCommand(),
		whoamiCommand(),
		importCommand(),
		lsCommand(),
		statCommand(),
		uploadCommand(),
		downloadCommand(),
		rmCommand(),
		moveCommand(),
		mkdirCommand(),
		ocmShareCreateCommand(),
		ocmShareListCommand(),
		ocmShareRemoveCommand(),
		ocmShareUpdateCommand(),
		ocmShareListReceivedCommand(),
		ocmShareUpdateReceivedCommand(),
		preferencesCommand(),
		genCommand(),
		recycleListCommand(),
		recycleRestoreCommand(),
		recyclePurgeCommand(),
		shareCreateCommand(),
		shareListCommand(),
		shareRemoveCommand(),
		shareUpdateCommand(),
		shareListReceivedCommand(),
		shareUpdateReceivedCommand(),
	}

	mainUsage := createMainUsage(cmds)

	// Verify that a subcommand has been provided
	if len(flag.Args()) < 1 {
		fmt.Println(mainUsage)
		os.Exit(1)
	}

	// Verify a configuration file exists.
	// If if does not, create one
	c, err := readConfig()
	if err != nil && flag.Args()[0] != "configure" {
		fmt.Println("reva is not initialized, run \"reva configure\"")
		os.Exit(1)
	} else if flag.Args()[0] != "configure" {
		conf = c
	}

	// Run command
	action := flag.Args()[0]
	for _, v := range cmds {
		if v.Name == action {
			if err := v.Parse(flag.Args()[1:]); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			err := v.Action()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	fmt.Println(mainUsage)
	os.Exit(1)
}

func createMainUsage(cmds []*command) string {
	n := 0
	for _, cmd := range cmds {
		l := len(cmd.Name)
		if l > n {
			n = l
		}
	}

	usage := "Command line interface to REVA\n\n"
	for _, cmd := range cmds {
		usage += fmt.Sprintf("%s%s%s\n", cmd.Name, strings.Repeat(" ", 4+(n-len(cmd.Name))), cmd.Description())
	}
	usage += "\nThe REVA authors"
	return usage
}
