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

	"github.com/c-bata/go-prompt"
)

var (
	conf                 *config
	host                 string
	insecure, skipverify bool

	gitCommit, buildDate, version, goVersion string

	commands = []*command{
		versionCommand(),
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
		openFileInAppProviderCommand(),
	}
)

func init() {
	flag.StringVar(&host, "host", "", "address of the GRPC gateway host")
	flag.BoolVar(&insecure, "insecure", false, "disables grpc transport security")
	flag.BoolVar(&skipverify, "skip-verify", false, "whether a client verifies the server's certificate chain and host name.")
	flag.Parse()
}

func main() {

	if host == "" {
		c, err := readConfig()
		if err != nil {
			fmt.Println("reva is not configured, please pass the \"-host\" flag")
			os.Exit(1)
		}
		conf = c
	} else {
		conf = &config{host}
		if err := writeConfig(conf); err != nil {
			fmt.Println("error writing to config file")
			os.Exit(1)
		}
	}

	executor := Executor{Commands: commands}
	completer := Completer{Commands: commands}

	if len(flag.Args()) > 0 {
		executor.Execute(strings.Join(flag.Args(), " "))
		return
	}

	fmt.Printf("reva-cli %s (rev-%s)\n", version, gitCommit)
	fmt.Println("Please use `exit` or `Ctrl-D` to exit this program.")

	p := prompt.New(
		executor.Execute,
		completer.Complete,
		prompt.OptionTitle("reva-cli"),
		prompt.OptionPrefix(">> "),
	)
	p.Run()
}
