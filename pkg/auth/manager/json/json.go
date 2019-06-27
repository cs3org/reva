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

package json

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// Credentials holds a pair of username and secret
// TOTDO id?
type Credentials struct {
	Username string `mapstructure:"username"`
	Secret   string `mapstructure:"secret"`
}

type manager struct {
	credentials map[string]string
}

type config struct {
	// Users holds a path to a file containing json conforming the Users struct
	Users string `mapstructure:"users"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	manager := &manager{credentials: map[string]string{}}

	credentials := []*Credentials{}
	f, err := ioutil.ReadFile(c.Users)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(f), &credentials)
	if err != nil {
		return nil, err
	}

	for _, c := range credentials {
		manager.credentials[c.Username] = c.Secret
	}

	return manager, nil
}

func (m *manager) Authenticate(ctx context.Context, username string, secret string) (context.Context, error) {
	if s, ok := m.credentials[username]; ok {
		if s == secret {
			return ctx, nil
		}
	}
	return ctx, invalidCredentialsError(username)
}

type invalidCredentialsError string

func (e invalidCredentialsError) Error() string { return string(e) }
