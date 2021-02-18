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

package demo

import (
	"context"
	"errors"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	catalog map[string]*userpb.User
}

// New returns a new user manager.
func New(m map[string]interface{}) (user.Manager, error) {
	cat := getUsers()
	return &manager{catalog: cat}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	if user, ok := m.catalog[uid.OpaqueId]; ok {
		if uid.Idp == "" || user.Id.Idp == uid.Idp {
			return user, nil
		}
	}
	return nil, errtypes.NotFound(uid.OpaqueId)
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	for _, u := range m.catalog {
		if userClaim, err := extractClaim(u, claim); err == nil && value == userClaim {
			return u, nil
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
		if u.Opaque != nil && u.Opaque.Map != nil {
			if uidObj, ok := u.Opaque.Map["uid"]; ok {
				if uidObj.Decoder == "plain" {
					return string(uidObj.Value), nil
				}
			}
		}
	}
	return "", errors.New("demo: invalid field")
}

// TODO(jfd) compare sub?
func userContains(u *userpb.User, query string) bool {
	return strings.Contains(u.Username, query) || strings.Contains(u.DisplayName, query) || strings.Contains(u.Mail, query) || strings.Contains(u.Id.OpaqueId, query)
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	users := []*userpb.User{}
	for _, u := range m.catalog {
		if userContains(u, query) {
			users = append(users, u)
		}
	}
	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	user, err := m.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return user.Groups, nil
}

func getUsers() map[string]*userpb.User {
	return map[string]*userpb.User{
		"4c510ada-c86b-4815-8820-42cdf82c3d51": &userpb.User{
			Id: &userpb.UserId{
				Idp:      "http://localhost:9998",
				OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
			},
			Username:    "einstein",
			Groups:      []string{"sailing-lovers", "violin-haters", "physics-lovers"},
			Mail:        "einstein@example.org",
			DisplayName: "Albert Einstein",
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"uid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("123"),
					},
					"gid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("987"),
					},
				},
			},
		},
		"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c": &userpb.User{
			Id: &userpb.UserId{
				Idp:      "http://localhost:9998",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			Username:    "marie",
			Groups:      []string{"radium-lovers", "polonium-lovers", "physics-lovers"},
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"uid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("456"),
					},
					"gid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte("987"),
					},
				},
			},
		},
		"932b4540-8d16-481e-8ef4-588e4b6b151c": &userpb.User{
			Id: &userpb.UserId{
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
