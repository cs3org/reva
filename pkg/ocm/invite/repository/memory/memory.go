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

package memory

import (
	"context"
	"strings"
	"sync"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/repository/registry"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new invite manager.
func New(m map[string]interface{}) (invite.Repository, error) {
	return &manager{
		Invites:       sync.Map{},
		AcceptedUsers: sync.Map{},
	}, nil
}

type manager struct {
	Invites       sync.Map
	AcceptedUsers sync.Map
}

func (m *manager) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	m.Invites.Store(token.GetToken(), token)
	return nil
}

func (m *manager) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	if v, ok := m.Invites.Load(token); ok {
		return v.(*invitepb.InviteToken), nil
	}
	return nil, invite.ErrTokenNotFound
}

func (m *manager) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	usersList, ok := m.AcceptedUsers.Load(initiator)
	acceptedUsers := usersList.([]*userpb.User)
	if ok {
		for _, acceptedUser := range acceptedUsers {
			if acceptedUser.Id.GetOpaqueId() == remoteUser.Id.OpaqueId && acceptedUser.Id.GetIdp() == remoteUser.Id.Idp {
				return invite.ErrUserAlreadyAccepted
			}
		}

		acceptedUsers = append(acceptedUsers, remoteUser)
		m.AcceptedUsers.Store(initiator.GetOpaqueId(), acceptedUsers)
	} else {
		acceptedUsers := []*userpb.User{remoteUser}
		m.AcceptedUsers.Store(initiator.GetOpaqueId(), acceptedUsers)
	}
	return nil
}

func (m *manager) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	usersList, ok := m.AcceptedUsers.Load(initiator)
	if !ok {
		return nil, errtypes.NotFound(remoteUserID.OpaqueId)
	}

	acceptedUsers := usersList.([]*userpb.User)
	for _, acceptedUser := range acceptedUsers {
		if (acceptedUser.Id.GetOpaqueId() == remoteUserID.OpaqueId) && (remoteUserID.Idp == "" || acceptedUser.Id.GetIdp() == remoteUserID.Idp) {
			return acceptedUser, nil
		}
	}
	return nil, errtypes.NotFound(remoteUserID.OpaqueId)
}

func (m *manager) FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, query string) ([]*userpb.User, error) {
	usersList, ok := m.AcceptedUsers.Load(initiator)
	if !ok {
		return []*userpb.User{}, nil
	}

	users := []*userpb.User{}
	acceptedUsers := usersList.([]*userpb.User)
	for _, acceptedUser := range acceptedUsers {
		if query == "" || userContains(acceptedUser, query) {
			users = append(users, acceptedUser)
		}
	}
	return users, nil
}

func userContains(u *userpb.User, query string) bool {
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(u.Username), query) || strings.Contains(strings.ToLower(u.DisplayName), query) ||
		strings.Contains(strings.ToLower(u.Mail), query) || strings.Contains(strings.ToLower(u.Id.OpaqueId), query)
}
