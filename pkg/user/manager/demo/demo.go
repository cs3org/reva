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

package demo

import (
	"context"
	"strings"

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	catalog map[string]*authproviderv0alphapb.User
}

// New returns a new user manager.
func New(m map[string]interface{}) (user.Manager, error) {
	cat := getUsers()
	return &manager{catalog: cat}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *typespb.UserId) (*authproviderv0alphapb.User, error) {
	if user, ok := m.catalog[uid.OpaqueId]; ok {
		return user, nil
	}
	return nil, errtypes.NotFound(uid.OpaqueId)
}

// TODO(jfd) search Opaque? compare sub?
func userContains(u *authproviderv0alphapb.User, query string) bool {
	return strings.Contains(u.Username, query) || strings.Contains(u.DisplayName, query) || strings.Contains(u.Mail, query)
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*authproviderv0alphapb.User, error) {
	users := []*authproviderv0alphapb.User{}
	for _, u := range m.catalog {
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

func getUsers() map[string]*authproviderv0alphapb.User {
	return map[string]*authproviderv0alphapb.User{
		"4c510ada-c86b-4815-8820-42cdf82c3d51": &authproviderv0alphapb.User{
			Id: &typespb.UserId{
				Idp:      "http://localhost:9998",
				OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
			},
			Username:    "einstein",
			Groups:      []string{"sailing-lovers", "violin-haters", "physics-lovers"},
			Mail:        "einstein@example.org",
			DisplayName: "Albert Einstein",
		},
		"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c": &authproviderv0alphapb.User{
			Id: &typespb.UserId{
				Idp:      "http://localhost:9998",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			Username:    "marie",
			Groups:      []string{"radium-lovers", "polonium-lovers", "physics-lovers"},
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
		},
		"932b4540-8d16-481e-8ef4-588e4b6b151c": &authproviderv0alphapb.User{
			Id: &typespb.UserId{
				Idp:      "http://localhost:9998",
				OpaqueId: "932b4540-8d16-481e-8ef4-588e4b6b151c",
			},
			Username:    "richard",
			Groups:      []string{"quantum-lovers", "philosophy-haters", "physics-lovers"},
			Mail:        "richard@example.org",
			DisplayName: "Richard Feynman",
		},
	}
}
