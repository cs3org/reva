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
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/cs3org/reva/pkg/user"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/manager/registry"
	"github.com/cs3org/reva/pkg/ocm/invite/token"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const acceptInviteEndpoint = "invites/accept"

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
		Invites:       sync.Map{},
		AcceptedUsers: sync.Map{},
		Config:        c,
	}, nil
}

type manager struct {
	Invites       sync.Map
	AcceptedUsers sync.Map
	Config        *config
}

type config struct {
	Expiration string `mapstructure:"expiration"`
}

func (m *manager) GenerateToken(ctx context.Context) (*invitepb.InviteToken, error) {

	ctxUser := user.ContextMustGetUser(ctx)
	inviteToken, err := token.CreateToken(m.Config.Expiration, ctxUser.GetId())
	if err != nil {
		return nil, errors.Wrap(err, "memory: error creating token")
	}

	m.Invites.Store(inviteToken.GetToken(), inviteToken)
	return inviteToken, nil
}

func (m *manager) ForwardInvite(ctx context.Context, invite *invitepb.InviteToken, originProvider *ocmprovider.ProviderInfo) error {

	contextUser := user.ContextMustGetUser(ctx)
	requestBody := url.Values{
		"token":             {invite.GetToken()},
		"userID":            {contextUser.GetId().GetOpaqueId()},
		"recipientProvider": {contextUser.GetId().GetIdp()},
	}

	resp, err := http.PostForm(fmt.Sprintf("%s%s", originProvider.GetApiEndpoint(), acceptInviteEndpoint), requestBody)
	if err != nil {
		err = errors.Wrap(err, "memory: error sending post request, URL: "+path.Join(originProvider.GetApiEndpoint(), acceptInviteEndpoint))
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = errors.Wrap(errors.New(resp.Status), "memory: error sending accept post request")
		return err
	}

	return nil
}

func (m *manager) AcceptInvite(ctx context.Context, invite *invitepb.InviteToken, userID *userpb.UserId) error {
	inviteToken, err := getTokenIfValid(m, invite)
	if err != nil {
		return err
	}

	currUser := inviteToken.GetUserId()
	usersList, ok := m.AcceptedUsers.Load(currUser)
	if ok {
		acceptedUsers := usersList.([]*userpb.UserId)
		for _, acceptedUser := range acceptedUsers {
			if userID.GetOpaqueId() == acceptedUser.OpaqueId && userID.GetIdp() == acceptedUser.Idp {
				return errors.New("memory: user already added to accepted users")
			}
		}

		acceptedUsers = append(acceptedUsers, userID)
		m.AcceptedUsers.Store(currUser, acceptedUsers)
	} else {
		acceptedUsers := []*userpb.UserId{userID}
		m.AcceptedUsers.Store(currUser, acceptedUsers)
	}
	return nil
}

func getTokenIfValid(m *manager, token *invitepb.InviteToken) (*invitepb.InviteToken, error) {
	tokenInterface, ok := m.Invites.Load(token.GetToken())
	if !ok {
		return nil, errors.New("memory: invalid token")
	}

	inviteToken := tokenInterface.(*invitepb.InviteToken)
	if uint64(time.Now().Unix()) > inviteToken.Expiration.Seconds {
		return nil, errors.New("memory: token expired")
	}
	return inviteToken, nil
}
