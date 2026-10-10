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

package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/list"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new invite manager.
func New(ctx context.Context, m map[string]any) (invite.Repository, error) {
	return &manager{
		Invites:       sync.Map{},
		acceptedUsers: map[string][]*userpb.User{},
	}, nil
}

type manager struct {
	Invites sync.Map

	// acceptedUsers keeps one entry per initiator opaque id. The mutex makes
	// the duplicate check and the list update one atomic step.
	acceptedUsersMu sync.RWMutex
	acceptedUsers   map[string][]*userpb.User
}

func (m *manager) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	if err := invite.ValidateInviteToken(token); err != nil {
		return err
	}

	m.Invites.Store(token.Token, token)
	return nil
}

func (m *manager) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errtypes.BadRequest("blank token")
	}

	v, ok := m.Invites.Load(token)
	if !ok {
		return nil, invite.ErrTokenNotFound
	}
	stored, ok := v.(*invitepb.InviteToken)
	if !ok || invite.ValidateInviteToken(stored) != nil {
		return nil, errtypes.InternalError("stored invite token is malformed")
	}
	return stored, nil
}

func (m *manager) ListTokens(ctx context.Context, initiator *userpb.UserId) ([]*invitepb.InviteToken, error) {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	tokens := []*invitepb.InviteToken{}
	var warnedMalformedToken bool
	now := uint64(time.Now().Unix())
	m.Invites.Range(func(_, value any) bool {
		token, ok := value.(*invitepb.InviteToken)
		if !ok || invite.ValidateInviteToken(token) != nil {
			if !warnedMalformedToken {
				log.Warn().Msg("skipping malformed stored invite token")
				warnedMalformedToken = true
			}
			return true
		}
		if utils.UserEqual(token.UserId, initiator) && !tokenIsExpired(token, now) {
			tokens = append(tokens, token)
		}
		return true
	})
	return tokens, nil
}

func tokenIsExpired(token *invitepb.InviteToken, now uint64) bool {
	return token.Expiration != nil && token.Expiration.Seconds < now
}

func (m *manager) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return err
	}
	if err := invite.ValidateRemoteUser(remoteUser); err != nil {
		return err
	}

	m.acceptedUsersMu.Lock()
	defer m.acceptedUsersMu.Unlock()

	log := appctx.GetLogger(ctx)
	key := initiator.GetOpaqueId()
	acceptedUsers := m.acceptedUsers[key]
	var warnedMalformedUser bool
	for _, acceptedUser := range acceptedUsers {
		if invite.ValidateRemoteUser(acceptedUser) != nil {
			if !warnedMalformedUser {
				log.Warn().Msg("skipping malformed stored remote user")
				warnedMalformedUser = true
			}
			continue
		}
		if acceptedUser.Id.GetOpaqueId() == remoteUser.Id.OpaqueId && acceptedUser.Id.GetIdp() == remoteUser.Id.Idp {
			return invite.ErrUserAlreadyAccepted
		}
	}

	m.acceptedUsers[key] = append(acceptedUsers, remoteUser)
	return nil
}

func (m *manager) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}
	if err := invite.ValidateUserID(remoteUserID, "remote user id"); err != nil {
		return nil, err
	}

	m.acceptedUsersMu.RLock()
	defer m.acceptedUsersMu.RUnlock()

	log := appctx.GetLogger(ctx)
	var warnedMalformedUser bool
	for _, acceptedUser := range m.acceptedUsers[initiator.GetOpaqueId()] {
		if invite.ValidateRemoteUser(acceptedUser) != nil {
			if !warnedMalformedUser {
				log.Warn().Msg("skipping malformed stored remote user")
				warnedMalformedUser = true
			}
			continue
		}
		if (acceptedUser.Id.GetOpaqueId() == remoteUserID.OpaqueId) && (remoteUserID.Idp == "" || acceptedUser.Id.GetIdp() == remoteUserID.Idp) {
			return acceptedUser, nil
		}
	}
	return nil, errtypes.NotFound(remoteUserID.OpaqueId)
}

func (m *manager) FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, query string) ([]*userpb.User, error) {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}

	m.acceptedUsersMu.RLock()
	defer m.acceptedUsersMu.RUnlock()

	log := appctx.GetLogger(ctx)
	users := []*userpb.User{}
	var warnedMalformedUser bool
	for _, acceptedUser := range m.acceptedUsers[initiator.GetOpaqueId()] {
		if invite.ValidateRemoteUser(acceptedUser) != nil {
			if !warnedMalformedUser {
				log.Warn().Msg("skipping malformed stored remote user")
				warnedMalformedUser = true
			}
			continue
		}
		if query == "" || userContains(acceptedUser, query) {
			users = append(users, acceptedUser)
		}
	}
	return users, nil
}

func userContains(u *userpb.User, query string) bool {
	if u == nil || u.Id == nil {
		return false
	}
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(u.Username), query) || strings.Contains(strings.ToLower(u.DisplayName), query) ||
		strings.Contains(strings.ToLower(u.Mail), query) || strings.Contains(strings.ToLower(u.Id.OpaqueId), query)
}

func (m *manager) DeleteRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.UserId) error {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return err
	}
	if err := invite.ValidateUserID(remoteUser, "remote user id"); err != nil {
		return err
	}

	m.acceptedUsersMu.Lock()
	defer m.acceptedUsersMu.Unlock()

	log := appctx.GetLogger(ctx)
	key := initiator.GetOpaqueId()
	acceptedUsers := m.acceptedUsers[key]
	var warnedMalformedUser bool
	for i, user := range acceptedUsers {
		if invite.ValidateRemoteUser(user) != nil {
			if !warnedMalformedUser {
				log.Warn().Msg("skipping malformed stored remote user")
				warnedMalformedUser = true
			}
			continue
		}
		if (user.Id.GetOpaqueId() == remoteUser.OpaqueId) && (remoteUser.Idp == "" || user.Id.GetIdp() == remoteUser.Idp) {
			m.acceptedUsers[key] = list.Remove(acceptedUsers, i)
			return nil
		}
	}
	return nil
}
