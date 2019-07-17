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
	"strings"

	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
)

func init() {
	registry.Register("json", New)
}

type manager struct {
	users []*authv0alphapb.User
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

// New returns a user manager implementation that reads a json file to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	f, err := ioutil.ReadFile(c.Users)
	if err != nil {
		return nil, err
	}

	users := []*authv0alphapb.User{}

	err = json.Unmarshal([]byte(f), &users)
	if err != nil {
		return nil, err
	}

	return &manager{
		users: users,
	}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *typespb.UserId) (*authv0alphapb.User, error) {
	for _, u := range m.users {
		if u.Username == uid.OpaqueId {
			return u, nil
		}
	}
	return nil, userNotFoundError(uid.OpaqueId)
}

// TODO search Opaque? compare sub?
func userContains(u *authv0alphapb.User, query string) bool {
	return strings.Contains(u.Username, query) || strings.Contains(u.DisplayName, query) || strings.Contains(u.Mail, query)
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*authv0alphapb.User, error) {
	users := []*authv0alphapb.User{}
	for _, u := range m.users {
		if userContains(u, query) {
			users = append(users, u)
		}
	}
	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *typespb.UserId) ([]string, error) {
	user, err := m.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return user.Groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, uid *typespb.UserId, group string) (bool, error) {
	user, err := m.GetUser(ctx, uid)
	if err != nil {
		return false, err
	}

	for _, g := range user.Groups {
		if group == g {
			return true, nil
		}
	}
	return false, nil
}

type userNotFoundError string

func (e userNotFoundError) Error() string { return string(e) }
