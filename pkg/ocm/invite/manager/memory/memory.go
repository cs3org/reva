// Copyright 2018-2020 CERN
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
	"sync"

	"github.com/cs3org/reva/pkg/user"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/manager/registry"
	"github.com/cs3org/reva/pkg/ocm/invite/token"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("memory", New)
}

// New returns a new invite manager.
func New(m map[string]interface{}) (invite.Manager, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}
	if c.Expiration == "" {
		c.Expiration = token.DefaultExpirationTime
	}

	return &manager{
		invites: sync.Map{},
		config:  c,
	}, nil
}

type manager struct {
	invites sync.Map
	config  *config
}

type config struct {
	Expiration string `mapstructure:"expiration"`
}

func (m *manager) GenerateToken(ctx context.Context) (*invitepb.InviteToken, error) {

	ctxUser := user.ContextMustGetUser(ctx)
	inviteToken, err := token.CreateToken(m.config.Expiration, ctxUser.GetId())
	if err != nil {
		return nil, errors.Wrap(err, "error create token")
	}

	m.invites.Store(inviteToken.GetToken(), inviteToken)

	return inviteToken, nil
}

func (m *manager) ForwardInvite(ctx context.Context, invite *invitepb.InviteToken, originProvider *ocm.ProviderInfo) error {
	return nil
}

func (m *manager) AcceptInvite(ctx context.Context, invite *invitepb.InviteToken, userID *userpb.UserId) error {
	return nil
}
