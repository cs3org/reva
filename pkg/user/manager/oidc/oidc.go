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

	"github.com/cs3org/reva/pkg/auth/manager/oidc"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/errtypes"
)

func init() {
	registry.Register("oidc", New)
}

type manager struct {
}

// New returns a new user manager.
func New(m map[string]interface{}) (user.Manager, error) {
	return &manager{}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *typespb.UserId) (*authproviderv0alphapb.User, error) {

	claims, ok := ctx.Value(oidc.ClaimsKey).(oidc.StandardClaims)
	if !ok {
		return nil, errtypes.NotFound(uid.OpaqueId)
	}

	user := &authproviderv0alphapb.User{
		// TODO(jfd) clean up idp = iss, sub = opaque ... is redundant
		Id: &typespb.UserId{
			OpaqueId: claims.Sub, // a stable non reassignable id
			Idp:      claims.Iss, // in the scope of this issuer
		},
		// Subject:     claims.Sub, // TODO(labkode) remove from CS3, is in Id
		// Issuer:      claims.Iss, // TODO(labkode) remove from CS3, is in Id
		Username:    claims.PreferredUsername,
		Groups:      []string{},
		Mail:        claims.Email,
		DisplayName: claims.Name,
	}

	// try kopano konnect specific claims
	if user.Username == "" {
		user.Username = claims.KCIdentity["kc.i.un"]
	}
	if user.DisplayName == "" {
		user.DisplayName = claims.KCIdentity["kc.i.dn"]
	}

	return user, nil
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*authproviderv0alphapb.User, error) {
	return []*authproviderv0alphapb.User{}, nil // FIXME implement FindUsers for oidc user manager
}

func (m *manager) GetUserGroups(ctx context.Context, uid *typespb.UserId) ([]string, error) {
	return []string{}, nil // FIXME implement GetUserGroups for oidc user manager
}

func (m *manager) IsInGroup(ctx context.Context, uid *typespb.UserId, group string) (bool, error) {
	return false, nil // FIXME implement IsInGroup for oidc user manager
}
