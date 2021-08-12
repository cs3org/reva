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
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

// RevaPlugin represents the runtime plugin
type RevaPlugin struct {
	Plugin interface{}
	Client *plugin.Client
}

const dirname = "/var/tmp/reva"

var isAlphaNum = regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString
var forcedRegexp = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)

// Kill kills the plugin process
func (plug *RevaPlugin) Kill() {
	plug.Client.Kill()
}

var handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "reva",
}

func compile(pluginType string, path string) (string, error) {
	var errb bytes.Buffer
	binaryPath := filepath.Join(dirname, "bin", pluginType, filepath.Base(path))
	command := fmt.Sprintf("go build -o %s %s", binaryPath, path)
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = path
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %w", errb.String(), err)
	}
	return binaryPath, nil
}

// checkDirAndCompile checks and compiles plugin if the configuration points to a directory.
func checkDirAndCompile(pluginType, driver string) (string, error) {
	bin := driver
	file, err := os.Stat(driver)
	if err != nil {
		return "", err
	}
	// compile if we point to a package
	if file.IsDir() {
		bin, err = compile(pluginType, driver)
		if err != nil {
			return "", err
		}
	}
	return bin, nil
}

// downloadPlugin downloads the plugin and stores it into local filesystem
func downloadAndCompilePlugin(pluginType, driver string) (string, error) {
	destination := fmt.Sprintf("%s/ext/%s/%s", dirname, pluginType, filepath.Base(driver))
	client := &getter.Client{
		Ctx:  context.Background(),
		Dst:  destination,
		Src:  driver,
		Mode: getter.ClientModeDir,
	}
	if err := client.Get(); err != nil {
		return "", err
	}
	bin, err := compile(pluginType, destination)
	if err != nil {
		return "", nil
	}
	return bin, nil
}

// isValidURL tests a string to determine if it is a well-structure URL
func isValidURL(driver string) bool {
	if driverURL := forcedRegexp.FindStringSubmatch(driver); driverURL != nil {
		driver = driverURL[2]
	}
	_, err := url.ParseRequestURI(driver)
	if err != nil {
		return false
	}

	u, err := url.Parse(driver)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

func fetchBinary(pluginType, driver string) (string, error) {
	var bin string
	var err error
	if isAlphaNum(driver) {
		return "", errtypes.NotFound(driver)
	}

	if isValidURL(driver) {
		bin, err = downloadAndCompilePlugin(pluginType, driver)
		if err != nil {
			return "", err
		}
	} else {
		bin, err = checkDirAndCompile(pluginType, driver)
		if err != nil {
			return "", err
		}
	}
	return bin, nil
}

// Load loads the plugin using the hashicorp go-plugin system
func Load(pluginType, driver string) (*RevaPlugin, error) {
	bin, err := fetchBinary(pluginType, driver)
	if err != nil {
		return nil, err
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Trace,
	})

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshake,
		Plugins:         PluginMap,
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

	revaPlugin := &RevaPlugin{
		Plugin: raw,
		Client: client,
	}

	return revaPlugin, nil
}
