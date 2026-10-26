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

package revadcmd

import (
	"slices"
	"testing"

	"github.com/cs3org/reva/v3"
)

type cliPlugin struct{}

func (cliPlugin) RevaPlugin() reva.PluginInfo {
	return reva.PluginInfo{ID: "cli.commands.revadtest", New: func() {}}
}

// A CLI plugin linked into revad must not stop it at start-up.
func TestCLIPluginsAreSkipped(t *testing.T) {
	reva.RegisterPlugin(cliPlugin{})
	initPlugins()
	if slices.ContainsFunc(revadPlugins(), func(p reva.PluginInfo) bool { return p.ID == "cli.commands.revadtest" }) {
		t.Error("revad --plugins lists a CLI plugin")
	}
}
