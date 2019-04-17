// Copyright 2018-2019 CERN
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
	"io/ioutil"
	gouser "os/user"
	"path"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

func getConfigFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva.config")
}

func getTokenFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva-token")
}

func writeToken(token string) {
	ioutil.WriteFile(getTokenFile(), []byte(token), 0600)
}

func readToken() (string, error) {
	data, err := ioutil.ReadFile(getTokenFile())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readConfig() (*config, error) {
	data, err := ioutil.ReadFile(getConfigFile())
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
	if c.AuthHeader == "" {
		c.AuthHeader = "x-access-token"
	}

	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(getConfigFile(), data, 0600)
}

type config struct {
	Host       string `json:"host"`
	AuthHeader string `json:"auth_header"`
}

func read(r *bufio.Reader) (string, error) {
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
func readPassword(fd int) (string, error) {
	bytePassword, err := terminal.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePassword)), nil
}
