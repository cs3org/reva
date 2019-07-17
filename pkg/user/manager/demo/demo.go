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

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	catalog map[string]*authv0alphapb.User
}

// New returns a new user manager.
func New(m map[string]interface{}) (user.Manager, error) {
	cat := getUsers()
	return &manager{catalog: cat}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *typespb.UserId) (*authv0alphapb.User, error) {
	if user, ok := m.catalog[uid.OpaqueId]; ok {
		return user, nil
	}
	return nil, userNotFoundError(uid.OpaqueId)
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*authv0alphapb.User, error) {
	return []*authv0alphapb.User{}, nil // FIXME implement FindUsers for demo user manager
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

func getUsers() map[string]*authv0alphapb.User {
	return map[string]*authv0alphapb.User{
		// TODO sub
		// TODO iss
		"einstein": &authv0alphapb.User{
			Username:    "einstein",
			Groups:      []string{"sailing-lovers", "violin-haters"},
			Mail:        "einstein@example.org",
			DisplayName: "Albert Einstein",
		},
		"marie": &authv0alphapb.User{
			Username:    "marie",
			Groups:      []string{"radium-lovers", "polonium-lovers"},
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
		},
		"richard": &authv0alphapb.User{
			Username:    "richard",
			Groups:      []string{"quantum-lovers", "philosophy-haters"},
			Mail:        "richard@example.org",
			DisplayName: "Richard Feynman",
		},
	}
}
