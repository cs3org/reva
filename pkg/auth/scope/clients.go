// Copyright 2018-2026 CERN
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

package scope

import (
	"path"
	"slices"
	"strings"
)

// Kinds of client an app password is issued for.
const (
	ClientSync   = "sync"
	ClientCLI    = "cli"
	ClientWebDAV = "webdav"
	ClientMCP    = "mcp"
)

// Presentation is how a client sends its app password.
type Presentation string

// Ways of presenting an app password.
const (
	PresentationBasic  Presentation = "basic"
	PresentationBearer Presentation = "bearer"
)

// ClientRule says how an app password of a kind of client may be used.
type ClientRule struct {
	Presentations []Presentation
	// UserAgents are substrings of the user agents expected from the client;
	// empty means any client.
	UserAgents []string
	// Strict rejects a user agent mismatch instead of only logging it.
	Strict bool
	// Paths are the HTTP path prefixes the app password may reach; nil means
	// any path.
	Paths []string
}

var clientRules = map[string]ClientRule{
	ClientSync: {
		Presentations: []Presentation{PresentationBasic},
		UserAgents: []string{
			"mirall/", "Nextcloud-Desktop/",
			"ownCloudApp/", "ownCloud-android/", "ownCloud-iOS/", "Nextcloud-android/", "Nextcloud-iOS/",
		},
		Strict: true,
	},
	ClientCLI: {
		Presentations: []Presentation{PresentationBearer},
		UserAgents:    []string{"cernbox-cli/"},
		Strict:        true,
	},
	ClientWebDAV: {
		Presentations: []Presentation{PresentationBasic, PresentationBearer},
	},
	ClientMCP: {
		Presentations: []Presentation{PresentationBearer},
		// The MCP service downloads and uploads through the data gateway,
		// which forwards the app password's token to the data provider.
		Paths: []string{"/mcp", "/data"},
	},
}

// ClientKinds returns the kinds of client app passwords can be issued for.
func ClientKinds() []string {
	return []string{ClientSync, ClientCLI, ClientWebDAV, ClientMCP}
}

// GetClientRule returns the rule of a kind of client.
func GetClientRule(kind string) (ClientRule, bool) {
	r, ok := clientRules[kind]
	return r, ok
}

// PresentationAllowed reports whether the app password may be sent as p.
func (c ClientRule) PresentationAllowed(p Presentation) bool {
	return slices.Contains(c.Presentations, p)
}

// UserAgentMatches reports whether ua is one the client is expected to send.
func (c ClientRule) UserAgentMatches(ua string) bool {
	if len(c.UserAgents) == 0 {
		return true
	}
	return slices.ContainsFunc(c.UserAgents, func(s string) bool { return strings.Contains(ua, s) })
}

// PathAllowed reports whether the app password may reach the HTTP path p.
func (c ClientRule) PathAllowed(p string) bool {
	if c.Paths == nil {
		return true
	}
	p = path.Clean("/" + p)
	return slices.ContainsFunc(c.Paths, func(prefix string) bool {
		prefix = path.Clean("/" + prefix)
		return prefix == "/" || p == prefix || strings.HasPrefix(p, prefix+"/")
	})
}
