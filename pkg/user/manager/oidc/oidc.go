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

func (m *manager) GetUser(ctx context.Context, username string) (*user.User, error) {

	claims, ok := ctx.Value(oidc.ClaimsKey).(oidc.Claims)
	if !ok {
		return nil, userNotFoundError(username)
	}

	return &user.User{
		Subject:     claims.Subject, // a stable non reassignable id
		Issuer:      claims.Issuer,  // in the scope of this issuer
		Username:    claims.KCIdentity["kc.i.un"],
		Groups:      []string{},
		Mail:        claims.Email,
		DisplayName: claims.KCIdentity["kc.i.dn"],
	}, nil
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*user.User, error) {
	return []*user.User{}, nil // FIXME implement IsInGroup for oidc user manager
}

func (m *manager) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	return []string{}, nil // FIXME implement GetUserGroups for oidc user manager
}

func (m *manager) IsInGroup(ctx context.Context, username, group string) (bool, error) {
	return false, nil // FIXME implement IsInGroup for oidc user manager
}

type userNotFoundError string

func (e userNotFoundError) Error() string { return string(e) }
