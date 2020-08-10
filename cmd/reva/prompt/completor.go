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
	"flag"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/cs3org/reva/cmd/reva/command"
)

// Completer provides completion command handler
type Completer struct {
	Commands []*command.Command
}

// Do provide completion to prompt
func (c *Completer) Do(d prompt.Document) []prompt.Suggest {
	if d.TextBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	args := strings.Split(d.TextBeforeCursor(), " ")

	w := d.GetWordBeforeCursor()

	// If word before the cursor starts with "-", returns CLI flag options.
	if strings.HasPrefix(w, "-") {
		return c.optionCompleter(args...)
	}

	return c.argumentCompleter(args...)
}

func (c *Completer) argumentCompleter(args ...string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(c.getAllSuggests(), args[0], true)
	}

	return []prompt.Suggest{}
}

func (c *Completer) optionCompleter(args ...string) []prompt.Suggest {
	if len(args) <= 1 {
		return prompt.FilterHasPrefix(c.getAllSuggests(), args[0], true)
	}

	var suggests []prompt.Suggest
	for _, cmd := range c.Commands {
		if cmd.Name == args[0] {
			cmd.VisitAll(func(fl *flag.Flag) {
				suggests = append(suggests, prompt.Suggest{Text: "-" + fl.Name, Description: fl.Usage})
			})
			return prompt.FilterContains(suggests, strings.TrimLeft(args[len(args)-1], "-"), true)
		}
	}

	return []prompt.Suggest{}
}

func (c *Completer) getAllSuggests() []prompt.Suggest {
	ss := make([]prompt.Suggest, 0, len(c.Commands))
	for _, cmd := range c.Commands {
		ss = append(ss, prompt.Suggest{Text: cmd.Name, Description: cmd.Description()})
	}
	ss = append(ss, prompt.Suggest{Text: "help", Description: "help for using reva CLI"})
	return ss
}
