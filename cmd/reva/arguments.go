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
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func (c *Completer) loginArgumentCompleter(args []string) []prompt.Suggest {
	var types []string
	b, err := executeCommand(loginCommand(), "-list")
	if err != nil {
		return []prompt.Suggest{}
	}
	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&types); err != nil {
		return []prompt.Suggest{}
	}

	suggests := make([]prompt.Suggest, len(types))
	for _, t := range types {
		suggests = append(suggests, prompt.Suggest{Text: t})
	}
	return suggests
}

func (c *Completer) lsArgumentCompleter(args []string, onlyDirs bool) []prompt.Suggest {
	var info []*provider.ResourceInfo
	b, err := executeCommand(lsCommand(), "/home")
	if err != nil {
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

func (c *Completer) ocmShareArgumentCompleter(args []string) []prompt.Suggest {
	var info []*ocm.Share
	b, err := executeCommand(ocmShareListCommand())
	if err != nil {
		return []prompt.Suggest{}
	}
	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&info); err != nil {
		return []prompt.Suggest{}
	}

	suggests := make([]prompt.Suggest, len(info))
	for _, r := range info {
		suggests = append(suggests, prompt.Suggest{Text: r.Id.OpaqueId})
	}
	return suggests
}

func (c *Completer) ocmShareReceivedArgumentCompleter(args []string) []prompt.Suggest {
	var info []*ocm.ReceivedShare
	b, err := executeCommand(ocmShareListReceivedCommand())
	if err != nil {
		return []prompt.Suggest{}
	}
	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&info); err != nil {
		return []prompt.Suggest{}
	}

	suggests := make([]prompt.Suggest, len(info))
	for _, r := range info {
		suggests = append(suggests, prompt.Suggest{Text: r.Share.Id.OpaqueId})
	}
	return suggests
}

func (c *Completer) shareArgumentCompleter(args []string) []prompt.Suggest {
	var info []*collaboration.Share
	b, err := executeCommand(shareListCommand())
	if err != nil {
		return []prompt.Suggest{}
	}
	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&info); err != nil {
		return []prompt.Suggest{}
	}

	suggests := make([]prompt.Suggest, len(info))
	for _, r := range info {
		suggests = append(suggests, prompt.Suggest{Text: r.Id.OpaqueId})
	}
	return suggests
}

func (c *Completer) shareReceivedArgumentCompleter(args []string) []prompt.Suggest {
	var info []*collaboration.ReceivedShare
	b, err := executeCommand(shareListReceivedCommand())
	if err != nil {
		return []prompt.Suggest{}
	}
	dec := gob.NewDecoder(&b)
	if err := dec.Decode(&info); err != nil {
		return []prompt.Suggest{}
	}

	suggests := make([]prompt.Suggest, len(info))
	for _, r := range info {
		suggests = append(suggests, prompt.Suggest{Text: r.Share.Id.OpaqueId})
	}
	return suggests
}

func executeCommand(cmd *command, args ...string) (bytes.Buffer, error) {
	var b bytes.Buffer
	if err := cmd.Parse(args); err != nil {
		return b, err
	}
	defer cmd.ResetFlags()
	if err := cmd.Action(&b); err != nil {
		return b, err
	}
	return b, nil
}
