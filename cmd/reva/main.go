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
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/cs3org/reva/pkg/rhttp"
)

var (
	conf                                   *config
	host                                   string
	insecure, skipverify, disableargprompt bool
	timeout                                int

	helpCommandOutput string

	gitCommit, buildDate, version, goVersion string

	client *http.Client

	commands = []*command{
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
		ocmFindAcceptedUsersCommand(),
		ocmInviteGenerateCommand(),
		ocmInviteForwardCommand(),
		ocmShareCreateCommand(),
		ocmShareListCommand(),
		ocmShareRemoveCommand(),
		ocmShareUpdateCommand(),
		ocmShareListReceivedCommand(),
		ocmShareUpdateReceivedCommand(),
		preferencesCommand(),
		genCommand(),
		publicShareCreateCommand(),
		publicShareListCommand(),
		publicShareRemoveCommand(),
		publicShareUpdateCommand(),
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
		transferCreateCommand(),
		transferGetStatusCommand(),
		transferCancelCommand(),
		helpCommand(),
	}
)

func init() {
	flag.StringVar(&host, "host", "", "address of the GRPC gateway host")
	flag.BoolVar(&insecure, "insecure", false, "disables grpc transport security")
	flag.BoolVar(&skipverify, "skip-verify", false, "whether to skip verifying the server's certificate chain and host name")
	flag.BoolVar(&disableargprompt, "disable-arg-prompt", false, "whether to disable prompts for command arguments")
	flag.IntVar(&timeout, "timout", -1, "the timeout in seconds for executing the commands, -1 means no timeout")
	flag.Parse()
}

func main() {

	if host != "" {
		conf = &config{host}
		if err := writeConfig(conf); err != nil {
			fmt.Println("error writing to config file")
			os.Exit(1)
		}
	}

	client = rhttp.GetHTTPClient(
		// TODO make insecure configurable
		rhttp.Insecure(true),
		// TODO make timeout configurable
		rhttp.Timeout(time.Duration(24*int64(time.Hour))),
	)

	generateMainUsage()
	executor := Executor{Timeout: timeout}
	completer := Completer{DisableArgPrompt: disableargprompt}
	completer.init()

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

func generateMainUsage() {
	n := 0
	for _, cmd := range commands {
		l := len(cmd.Name)
		if l > n {
			n = l
		}
	}

	helpCommandOutput = "Command line interface to REVA:\n"
	for _, cmd := range commands {
		helpCommandOutput += fmt.Sprintf("%s%s%s\n", cmd.Name, strings.Repeat(" ", 4+(n-len(cmd.Name))), cmd.Description())
	}
}
