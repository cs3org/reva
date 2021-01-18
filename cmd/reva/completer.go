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
	"strings"

	"github.com/c-bata/go-prompt"
)

// Completer provides completion command handler
type Completer struct {
	Commands                  []*command
	DisableArgPrompt          bool
	loginArguments            *argumentCompleter
	lsArguments               *argumentCompleter
	lsDirArguments            *argumentCompleter
	ocmShareArguments         *argumentCompleter
	ocmShareReceivedArguments *argumentCompleter
	shareArguments            *argumentCompleter
	shareReceivedArguments    *argumentCompleter
}

func (c *Completer) init() {
	c.loginArguments, c.lsArguments = new(argumentCompleter), new(argumentCompleter)
	c.ocmShareArguments, c.ocmShareReceivedArguments = new(argumentCompleter), new(argumentCompleter)
	c.shareArguments, c.shareReceivedArguments = new(argumentCompleter), new(argumentCompleter)
	c.lsDirArguments = new(argumentCompleter)
}

// Complete provides completion to prompt
func (c *Completer) Complete(d prompt.Document) []prompt.Suggest {
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	args := strings.Split(d.TextBeforeCursor(), " ")

	w := d.GetWordBeforeCursor()

	// If word before the cursor starts with "-", returns CLI flag options.
	if strings.HasPrefix(w, "-") {
		return c.optionCompleter(args...)
	}

	if suggests, ok := c.completeOptionArguments(d); ok {
		return suggests
	}

	commandArgs, skipNext := excludeOptions(args)
	if skipNext {
		return []prompt.Suggest{}
	}

	// TODO(ishank011): check if we can reuse the results from these calls in executor
	return c.argumentCompleter(commandArgs...)
}

func (c *Completer) argumentCompleter(args ...string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(c.getAllSuggests(), args[0], true)
	} else if c.DisableArgPrompt {
		return []prompt.Suggest{}
	}

	var suggests []prompt.Suggest

	switch args[0] {
	case "gen":
		suggests = convertCmdToSuggests([]*command{
			genConfigSubCommand(),
			genUsersSubCommand(),
		})
		return prompt.FilterHasPrefix(suggests, args[1], true)

	case "login":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.loginArgumentCompleter(), args[1], true)
		}

	case "ls", "mkdir":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.lsArgumentCompleter(true), args[1], true)
		}

	case "mv":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.lsArgumentCompleter(false), args[1], true)
		} else if len(args) == 3 {
			return prompt.FilterHasPrefix(c.lsArgumentCompleter(false), args[2], true)
		}

	case "rm", "stat", "share-create", "ocm-share-create", "public-share-create", "open-file-in-app-provider", "download":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.lsArgumentCompleter(false), args[1], true)
		}

	case "upload":
		if len(args) == 3 {
			return prompt.FilterHasPrefix(c.lsArgumentCompleter(false), args[2], true)
		}

	case "ocm-share-remove", "ocm-share-update":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.ocmShareArgumentCompleter(), args[1], true)
		}

	case "ocm-share-update-received":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.ocmShareReceivedArgumentCompleter(), args[1], true)
		}

	case "share-remove", "share-update":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.shareArgumentCompleter(), args[1], true)
		}

	case "share-update-received":
		if len(args) == 2 {
			return prompt.FilterHasPrefix(c.shareReceivedArgumentCompleter(), args[1], true)
		}
	}

	return []prompt.Suggest{}
}

func (c *Completer) optionCompleter(args ...string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(c.getAllSuggests(), args[0], true)
	}

	var suggests []prompt.Suggest
	for _, cmd := range commands {
		if cmd.Name == args[0] {
			cmd.VisitAll(func(fl *flag.Flag) {
				suggests = append(suggests, prompt.Suggest{Text: "-" + fl.Name, Description: fl.Usage})
			})
			return prompt.FilterContains(suggests, strings.TrimLeft(args[len(args)-1], "-"), true)
		}
	}

	return []prompt.Suggest{}
}

func (c *Completer) completeOptionArguments(d prompt.Document) ([]prompt.Suggest, bool) {
	_, option, ok := getPreviousOption(d)
	if !ok {
		return []prompt.Suggest{}, false
	}

	var suggests []prompt.Suggest
	var match bool
	switch option {
	case "-cs":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "basic"}, prompt.Suggest{Text: "oidc"}}
		match = true
	case "-dd":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "local"}, prompt.Suggest{Text: "owncloud"}}
		match = true
	case "-type":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "user"}, prompt.Suggest{Text: "group"}}
		match = true
	case "-rol":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "viewer"}, prompt.Suggest{Text: "editor"}}
		match = true
	case "-state":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "pending"}, prompt.Suggest{Text: "accepted"}, prompt.Suggest{Text: "rejected"}}
		match = true
	case "-viewmode":
		suggests = []prompt.Suggest{prompt.Suggest{Text: "view"}, prompt.Suggest{Text: "read"}, prompt.Suggest{Text: "write"}}
		match = true
	case "-c", "-grantee", "-idp", "-by-resource-id", "-xs", "-token":
		match = true
	}
	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), true), match
}

func getPreviousOption(d prompt.Document) (cmd, option string, ok bool) {
	args := strings.Split(d.TextBeforeCursor(), " ")
	l := len(args)
	if l >= 2 {
		option = args[l-2]
	}
	if strings.HasPrefix(option, "-") {
		return args[0], option, true
	}
	return "", "", false
}

func excludeOptions(args []string) ([]string, bool) {
	l := len(args)
	if l == 0 {
		return nil, false
	}
	filtered := make([]string, 0, l)

	var skipNextArg bool
	for i := 0; i < len(args); i++ {
		if skipNextArg {
			skipNextArg = false
			continue
		}

		for _, s := range []string{
			"-c", "-cs", "-dd", "-dp", "-type", "-grantee", "-idp", "-rol",
			"-by-resource-id", "-state", "-viewmode", "-xs", "-token",
		} {
			if strings.HasPrefix(args[i], s) {
				if strings.Contains(args[i], "=") {
					// we can specify option value like '-o=json'
					skipNextArg = false
				} else {
					skipNextArg = true
				}
				continue
			}
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}

		filtered = append(filtered, args[i])
	}
	return filtered, skipNextArg
}

func (c *Completer) getAllSuggests() []prompt.Suggest {
	return convertCmdToSuggests(commands)
}

func convertCmdToSuggests(cmds []*command) []prompt.Suggest {
	ss := make([]prompt.Suggest, 0, len(cmds))
	for _, cmd := range cmds {
		ss = append(ss, prompt.Suggest{Text: cmd.Name, Description: cmd.Description()})
	}
	return ss
}
