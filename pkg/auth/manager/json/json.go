// Copyright 2018-2023 CERN
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
	"os"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("json", New)
}

// Credentials holds a pair of secret and userid.
type Credentials struct {
	ID           *user.UserId    `json:"id"            mapstructure:"id"`
	Username     string          `json:"username"      mapstructure:"username"`
	Mail         string          `json:"mail"          mapstructure:"mail"`
	MailVerified bool            `json:"mail_verified" mapstructure:"mail_verified"`
	DisplayName  string          `json:"display_name"  mapstructure:"display_name"`
	Secret       string          `json:"secret"        mapstructure:"secret"`
	Groups       []string        `json:"groups"        mapstructure:"groups"`
	UIDNumber    int64           `json:"uid_number"    mapstructure:"uid_number"`
	GIDNumber    int64           `json:"gid_number"    mapstructure:"gid_number"`
	Opaque       *typespb.Opaque `json:"opaque"        mapstructure:"opaque"`
}

type manager struct {
	credentials map[string]*Credentials
}

type config struct {
	// Users holds a path to a file containing json conforming the Users struct
	Users string `mapstructure:"users"`
}

func (c *config) ApplyDefaults() {
	if c.Users == "" {
		c.Users = "/etc/revad/users.json"
	}
}

// New returns a new auth Manager.
func New(ctx context.Context, m map[string]interface{}) (auth.Manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *manager) Configure(ml map[string]interface{}) error {
	var c config
	if err := cfg.Decode(ml, &c); err != nil {
		return errors.Wrap(err, "json: error decoding config")
	}

	m.credentials = map[string]*Credentials{}
	f, err := os.ReadFile(c.Users)
	if err != nil {
		return err
	}

	credentials := []*Credentials{}

	err = json.Unmarshal(f, &credentials)
	if err != nil {
		return err
	}

	for _, c := range credentials {
		m.credentials[c.Username] = c
	}
	return nil
}

func (m *manager) Authenticate(ctx context.Context, username string, secret string) (*user.User, map[string]*authpb.Scope, error) {
	if c, ok := m.credentials[username]; ok {
		if c.Secret == secret {
			var scopes map[string]*authpb.Scope
			var err error
			if c.ID != nil && (c.ID.Type == user.UserType_USER_TYPE_LIGHTWEIGHT || c.ID.Type == user.UserType_USER_TYPE_FEDERATED) {
				scopes, err = scope.AddLightweightAccountScope(authpb.Role_ROLE_OWNER, nil)
				if err != nil {
					return nil, nil, err
				}
			} else {
				scopes, err = scope.AddOwnerScope(nil)
				if err != nil {
					return nil, nil, err
				}
			}
			return &user.User{
				Id:           c.ID,
				Username:     c.Username,
				Mail:         c.Mail,
				MailVerified: c.MailVerified,
				DisplayName:  c.DisplayName,
				Groups:       c.Groups,
				UidNumber:    c.UIDNumber,
				GidNumber:    c.GIDNumber,
				Opaque:       c.Opaque,
				// TODO add arbitrary keys as opaque data
			}, scopes, nil
		}
	}
	return nil, nil, errtypes.InvalidCredentials(username)
}
