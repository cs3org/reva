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
	"bytes"
	"encoding/gob"

	"github.com/c-bata/go-prompt"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func (c *Completer) loginArgumentCompleter(args []string) []prompt.Suggest {
	var b bytes.Buffer
	var types []string

	cmd := loginCommand()
	if err := cmd.Parse([]string{"-list"}); err != nil {
		return []prompt.Suggest{}
	}
	defer cmd.ResetFlags()

	if err := cmd.Action(&b); err != nil {
		return []prompt.Suggest{}
	}

	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&types); err != nil {
		return []prompt.Suggest{}
	}

	var suggests []prompt.Suggest
	for _, t := range types {
		suggests = append(suggests, prompt.Suggest{Text: t})
	}
	return suggests
}

func (c *Completer) lsArgumentCompleter(args []string, onlyDirs bool) []prompt.Suggest {
	var b bytes.Buffer
	var info []*provider.ResourceInfo

	cmd := lsCommand()
	if err := cmd.Parse([]string{"/home"}); err != nil {
		return []prompt.Suggest{}
	}
	if err := cmd.Action(&b); err != nil {
		return []prompt.Suggest{}
	}

	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&info); err != nil {
		return []prompt.Suggest{}
	}

	suggests := []prompt.Suggest{prompt.Suggest{Text: "/home"}}
	for _, r := range info {
		if !onlyDirs || r.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			suggests = append(suggests, prompt.Suggest{Text: r.Path})
		}
	}
	return suggests
}
