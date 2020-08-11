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
	"errors"
	"sync"
	"time"

	"github.com/c-bata/go-prompt"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

type argumentCompleter struct {
	suggestions []prompt.Suggest
	expiration  time.Time
	sync.RWMutex
}

func (c *Completer) loginArgumentCompleter(args []string) []prompt.Suggest {
	if s, ok := checkCache(c.loginArguments); ok {
		return s
	}

	var types []string
	b, err := executeCommand(loginCommand(), "-list")
	if err != nil {
		if err.Error() == "timeout" {
			cacheSuggestions(c.loginArguments, []prompt.Suggest{})
		}
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
	cacheSuggestions(c.loginArguments, suggests)
	return suggests
}

func (c *Completer) lsArgumentCompleter(args []string, onlyDirs bool) []prompt.Suggest {
	if onlyDirs {
		if s, ok := checkCache(c.lsDirArguments); ok {
			return s
		}
	} else {
		if s, ok := checkCache(c.lsArguments); ok {
			return s
		}
	}

	var info []*provider.ResourceInfo
	b, err := executeCommand(lsCommand(), "/home")
	if err != nil {
		if err.Error() == "timeout" {
			if onlyDirs {
				cacheSuggestions(c.lsDirArguments, []prompt.Suggest{})
			} else {
				cacheSuggestions(c.lsArguments, []prompt.Suggest{})
			}
		}
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

	if onlyDirs {
		cacheSuggestions(c.lsDirArguments, suggests)
	} else {
		cacheSuggestions(c.lsArguments, suggests)
	}
	return suggests
}

func (c *Completer) ocmShareArgumentCompleter(args []string) []prompt.Suggest {
	if s, ok := checkCache(c.ocmShareArguments); ok {
		return s
	}

	var info []*ocm.Share
	b, err := executeCommand(ocmShareListCommand())
	if err != nil {
		if err.Error() == "timeout" {
			cacheSuggestions(c.ocmShareArguments, []prompt.Suggest{})
		}
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
	cacheSuggestions(c.ocmShareArguments, suggests)
	return suggests
}

func (c *Completer) ocmShareReceivedArgumentCompleter(args []string) []prompt.Suggest {
	if s, ok := checkCache(c.ocmShareReceivedArguments); ok {
		return s
	}

	var info []*ocm.ReceivedShare
	b, err := executeCommand(ocmShareListReceivedCommand())
	if err != nil {
		if err.Error() == "timeout" {
			cacheSuggestions(c.ocmShareReceivedArguments, []prompt.Suggest{})
		}
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
	cacheSuggestions(c.ocmShareReceivedArguments, suggests)
	return suggests
}

func (c *Completer) shareArgumentCompleter(args []string) []prompt.Suggest {
	if s, ok := checkCache(c.shareArguments); ok {
		return s
	}

	var info []*collaboration.Share
	b, err := executeCommand(shareListCommand())
	if err != nil {
		if err.Error() == "timeout" {
			cacheSuggestions(c.shareArguments, []prompt.Suggest{})
		}
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
	cacheSuggestions(c.shareArguments, suggests)
	return suggests
}

func (c *Completer) shareReceivedArgumentCompleter(args []string) []prompt.Suggest {
	if s, ok := checkCache(c.shareReceivedArguments); ok {
		return s
	}

	var info []*collaboration.ReceivedShare
	b, err := executeCommand(shareListReceivedCommand())
	if err != nil {
		if err.Error() == "timeout" {
			cacheSuggestions(c.shareReceivedArguments, []prompt.Suggest{})
		}
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
	cacheSuggestions(c.shareReceivedArguments, suggests)
	return suggests
}

func executeCommand(cmd *command, args ...string) (bytes.Buffer, error) {
	var b bytes.Buffer
	var err error
	if err = cmd.Parse(args); err != nil {
		return b, err
	}
	defer cmd.ResetFlags()

	c := make(chan error, 1)
	go func() {
		c <- cmd.Action(&b)
	}()

	select {
	case err = <-c:
		if err != nil {
			return b, err
		}
	case <-time.After(500 * time.Millisecond):
		return b, errors.New("timeout")
	}
	return b, nil
}

func checkCache(a *argumentCompleter) ([]prompt.Suggest, bool) {
	a.RLock()
	defer a.RUnlock()
	if time.Now().Before(a.expiration) {
		return a.suggestions, true
	}
	return nil, false
}

func cacheSuggestions(a *argumentCompleter, suggests []prompt.Suggest) {
	a.Lock()
	a.suggestions = suggests
	a.expiration = time.Now().Add(time.Second * 10)
	a.Unlock()
}
