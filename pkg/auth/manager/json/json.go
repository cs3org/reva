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

package json

import (
	"context"
	"encoding/json"
	"io/ioutil"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// Credentials holds a pair of secret and userid
type Credentials struct {
	ID           *user.UserId    `mapstructure:"id" json:"id"`
	Username     string          `mapstructure:"username" json:"username"`
	Mail         string          `mapstructure:"mail" json:"mail"`
	MailVerified bool            `mapstructure:"mail_verified" json:"mail_verified"`
	DisplayName  string          `mapstructure:"display_name" json:"display_name"`
	Secret       string          `mapstructure:"secret" json:"secret"`
	Groups       []string        `mapstructure:"groups" json:"groups"`
	Opaque       *typespb.Opaque `mapstructure:"opaque" json:"opaque"`
}

type manager struct {
	credentials map[string]*Credentials
}

type config struct {
	// Users holds a path to a file containing json conforming the Users struct
	Users string `mapstructure:"users"`
}

func (c *config) init() {
	if c.Users == "" {
		c.Users = "/etc/revad/users.json"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()
	return c, nil
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	manager := &manager{credentials: map[string]*Credentials{}}

	f, err := ioutil.ReadFile(c.Users)
	if err != nil {
		return nil, err
	}

	credentials := []*Credentials{}

	err = json.Unmarshal(f, &credentials)
	if err != nil {
		return nil, err
	}

	for _, c := range credentials {
		manager.credentials[c.Username] = c
	}

	return manager, nil
}

func (m *manager) Authenticate(ctx context.Context, username string, secret string) (*user.User, error) {
	if c, ok := m.credentials[username]; ok {
		if c.Secret == secret {
			return &user.User{
				Id:           c.ID,
				Username:     c.Username,
				Mail:         c.Mail,
				MailVerified: c.MailVerified,
				DisplayName:  c.DisplayName,
				Groups:       c.Groups,
				Opaque:       c.Opaque,
				// TODO add arbitrary keys as opaque data
			}, nil
		}
	}
	return nil, errtypes.InvalidCredentials(username)
}
