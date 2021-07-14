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

package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cs3org/reva/pkg/pluginregistry"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

var handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

func compile(path string, pluginType string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not find current directory: %v", err)
	}
	pluginsDir := filepath.Join(wd, "bin", pluginType, filepath.Base(path))
	command := fmt.Sprintf("GO111MODULE=off go build -o %s %s", pluginsDir, path)
	cmd := exec.Command("bash", "-c", command)
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	return pluginsDir, nil
}

// Load loads the plugin using the hashicorp go-plugin system
func Load(driver string, pluginType string) (interface{}, error) {
	bin := driver
	file, err := os.Stat(driver)
	if err != nil {
		return nil, err
	}
	// compile if we point to a package.
	if file.IsDir() {
		bin, err = compile(driver, pluginType)
		if err != nil {
			return nil, err
		}
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Trace,
	})

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshake,
		Plugins:         pluginregistry.PluginMap,
		Cmd:             exec.Command(bin),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolNetRPC,
		},
		Logger: logger,
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := rpcClient.Dispense(pluginType)
	if err != nil {
		return nil, err
	}

	return raw, nil
}
