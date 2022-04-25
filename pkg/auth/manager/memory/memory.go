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

package memory

import (
	"context"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v2/pkg/auth"
	"github.com/cs3org/reva/v2/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v2/pkg/auth/scope"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("memory", New)
}

type config struct {
	// Users holds a map with userid and secret
	Users map[string]*Credentials `mapstructure:"users"`
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
	UIDNumber    int64           `mapstructure:"uid_number" json:"uid_number"`
	GIDNumber    int64           `mapstructure:"gid_number" json:"gid_number"`
	Opaque       *typespb.Opaque `mapstructure:"opaque" json:"opaque"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

type manager struct {
	credentials map[string]*Credentials
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	return mgr, err
}

func (m *manager) Configure(ml map[string]interface{}) error {
	c, err := parseConfig(ml)
	if err != nil {
		return err
	}
	m.credentials = c.Users
	return nil
}

func (m *manager) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error) {
	if u, ok := m.credentials[clientID]; ok {
		if u.Secret == clientSecret {
			var scopes map[string]*authpb.Scope
			var err error
			if u.ID != nil && (u.ID.Type == user.UserType_USER_TYPE_LIGHTWEIGHT || u.ID.Type == user.UserType_USER_TYPE_FEDERATED) {
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
				Id:           u.ID,
				Username:     u.Username,
				Mail:         u.Mail,
				MailVerified: u.MailVerified,
				DisplayName:  u.DisplayName,
				Groups:       u.Groups,
				UidNumber:    u.UIDNumber,
				GidNumber:    u.GIDNumber,
				Opaque:       u.Opaque,
				// TODO add arbitrary keys as opaque data
			}, scopes, nil
		}
	}
	return nil, nil, errtypes.InvalidCredentials(clientID)
}
