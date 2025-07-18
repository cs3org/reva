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

package json

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/user"
	"github.com/cs3org/reva/v3/pkg/user/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func init() {
	registry.Register("json", New)
}

type manager struct {
	users []*userpb.User
}

type config struct {
	// Users holds a path to a file containing json conforming to the Users struct
	Users string `mapstructure:"users"`
}

func (c *config) ApplyDefaults() {
	if c.Users == "" {
		c.Users = "/etc/revad/users.json"
	}
}

// New returns a user manager implementation that reads a json file to provide user metadata.
func New(_ context.Context, m map[string]interface{}) (user.Manager, error) {
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
		return err
	}

	f, err := os.ReadFile(c.Users)
	if err != nil {
		return err
	}

	users := []*userpb.User{}

	err = json.Unmarshal(f, &users)
	if err != nil {
		return err
	}
	m.users = users
	return nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId, skipFetchingGroups bool) (*userpb.User, error) {
	for _, u := range m.users {
		if (u.Id.GetOpaqueId() == uid.OpaqueId || u.Username == uid.OpaqueId) && (uid.Idp == "" || uid.Idp == u.Id.GetIdp()) {
			user := proto.Clone(u).(*userpb.User)
			if skipFetchingGroups {
				user.Groups = nil
			}
			return user, nil
		}
	}
	return nil, errtypes.NotFound(uid.OpaqueId)
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string, skipFetchingGroups bool) (*userpb.User, error) {
	for _, u := range m.users {
		if userClaim, err := extractClaim(u, claim); err == nil && value == userClaim {
			user := proto.Clone(u).(*userpb.User)
			if skipFetchingGroups {
				user.Groups = nil
			}
			return user, nil
		}
	}
	return nil, errtypes.NotFound(value)
}

func extractClaim(u *userpb.User, claim string) (string, error) {
	switch claim {
	case "mail":
		return u.Mail, nil
	case "username":
		return u.Username, nil
	case "uid":
		if u.UidNumber != 0 {
			return strconv.FormatInt(u.UidNumber, 10), nil
		}
	}
	return "", errors.New("json: invalid field")
}

// TODO(jfd) search Opaque? compare sub?
func userContains(u *userpb.User, query string) bool {
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(u.Username), query) || strings.Contains(strings.ToLower(u.DisplayName), query) ||
		strings.Contains(strings.ToLower(u.Mail), query) || strings.Contains(strings.ToLower(u.Id.OpaqueId), query)
}

func (m *manager) FindUsers(ctx context.Context, query string, skipFetchingGroups bool) ([]*userpb.User, error) {
	users := []*userpb.User{}
	for _, u := range m.users {
		if userContains(u, query) {
			user := proto.Clone(u).(*userpb.User)
			if skipFetchingGroups {
				user.Groups = nil
			}
			users = append(users, user)
		}
	}
	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	user, err := m.GetUser(ctx, uid, false)
	if err != nil {
		return nil, err
	}
	return user.Groups, nil
}
