// Copyright 2018-2024 CERN
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
	"bufio"
	"encoding/json"
	"os"
	gouser "os/user"
	"path"
	"strings"

	"golang.org/x/term"
)

const (
	viewerPermission string = "viewer"
	editorPermission string = "editor"
	collabPermission string = "collab"
	denyPermission   string = "denied"
)

type config struct {
	Host string `json:"host"`
	// AdminHost is the address of the admin gRPC endpoint (its own port,
	// separate from the gateway). Used by the `admin` subcommands.
	AdminHost string `json:"admin_host,omitempty"`
}

func getConfigFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva.config")
}

func readConfig() (*config, error) {
	data, err := os.ReadFile(getConfigFile())
	if err != nil {
		return nil, err
	}

	c := &config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}

	return c, nil
}

func writeConfig(c *config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigFile(), data, 0600)
}

func getTokenFile() string {
	if tokenFile != "" {
		return tokenFile
	}
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva-token")
}

func readToken() (string, error) {
	data, err := os.ReadFile(getTokenFile())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeToken(token string) {
	err := os.WriteFile(getTokenFile(), []byte(token), 0600)
	if err != nil {
		panic(err)
	}
}

// The short-TTL admin token is stored separately from the login token, so the
// two coexist (the sudo model): `admin elevate` writes it, every other admin
// subcommand reads it.
func getAdminTokenFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}
	return path.Join(user.HomeDir, ".reva-admin-token")
}

func readAdminToken() (string, error) {
	data, err := os.ReadFile(getAdminTokenFile())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeAdminToken(token string) {
	if err := os.WriteFile(getAdminTokenFile(), []byte(token), 0600); err != nil {
		panic(err)
	}
}

func read(r *bufio.Reader) (string, error) {
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
func readPassword(fd int) (string, error) {
	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePassword)), nil
}
