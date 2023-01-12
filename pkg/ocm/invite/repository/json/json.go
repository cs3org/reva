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

package json

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/repository/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type inviteModel struct {
	File          string
	Invites       map[string]*invitepb.InviteToken `json:"invites"`
	AcceptedUsers map[string][]*userpb.User        `json:"accepted_users"`
}

type manager struct {
	config       *config
	sync.RWMutex // concurrent access to the file
	model        *inviteModel
}

type config struct {
	File string `mapstructure:"file"`
}

func init() {
	registry.Register("json", New)
}

func (c *config) init() error {
	if c.File == "" {
		c.File = "/var/tmp/reva/ocm-invites.json"
	}

	return nil
}

// New returns a new invite manager object.
func New(m map[string]interface{}) (invite.Repository, error) {
	config, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing config for json invite repository")
	}
	err = config.init()
	if err != nil {
		return nil, errors.Wrap(err, "error setting config defaults for json invite repository")
	}

	// load or create file
	model, err := loadOrCreate(config.File)
	if err != nil {
		return nil, errors.Wrap(err, "error loading the file containing the invites")
	}

	manager := &manager{
		config: config,
		model:  model,
	}

	return manager, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func loadOrCreate(file string) (*inviteModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := os.WriteFile(file, []byte("{}"), 0700); err != nil {
			return nil, errors.Wrap(err, "error creating the invite storage file: "+file)
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "error opening the invite storage file: "+file)
	}
	defer fd.Close()

	data, err := io.ReadAll(fd)
	if err != nil {
		return nil, errors.Wrap(err, "error reading the data")
	}

	model := &inviteModel{}
	if err := json.Unmarshal(data, model); err != nil {
		return nil, errors.Wrap(err, "error decoding invite data to json")
	}

	if model.Invites == nil {
		model.Invites = make(map[string]*invitepb.InviteToken)
	}
	if model.AcceptedUsers == nil {
		model.AcceptedUsers = make(map[string][]*userpb.User)
	}

	model.File = file
	return model, nil
}

func (model *inviteModel) save() error {
	data, err := json.Marshal(model)
	if err != nil {
		return errors.Wrap(err, "error encoding invite data to json")
	}

	if err := os.WriteFile(model.File, data, 0644); err != nil {
		return errors.Wrap(err, "error writing invite data to file: "+model.File)
	}

	return nil
}

func (m *manager) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	m.Lock()
	defer m.Unlock()

	m.model.Invites[token.GetToken()] = token
	if err := m.model.save(); err != nil {
		return errors.Wrap(err, "json: error saving model")
	}
	return nil
}

func (m *manager) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	m.RLock()
	defer m.RUnlock()

	if tkn, ok := m.model.Invites[token]; ok {
		return tkn, nil
	}
	return nil, invite.ErrTokenNotFound
}

func (m *manager) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	m.Lock()
	defer m.Unlock()

	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
		if acceptedUser.Id.GetOpaqueId() == remoteUser.Id.OpaqueId && acceptedUser.Id.GetIdp() == remoteUser.Id.Idp {
			return invite.ErrUserAlreadyAccepted
		}
	}

	m.model.AcceptedUsers[initiator.GetOpaqueId()] = append(m.model.AcceptedUsers[initiator.GetOpaqueId()], remoteUser)
	if err := m.model.save(); err != nil {
		return errors.Wrap(err, "json: error saving model")
	}
	return nil
}

func (m *manager) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	m.RLock()
	defer m.RUnlock()

	log := appctx.GetLogger(ctx)
	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
		log.Info().Msgf("looking for '%s' at '%s' - considering '%s' at '%s'",
			remoteUserID.OpaqueId,
			remoteUserID.Idp,
			acceptedUser.Id.GetOpaqueId(),
			acceptedUser.Id.GetIdp(),
		)
		if (acceptedUser.Id.GetOpaqueId() == remoteUserID.OpaqueId) && (remoteUserID.Idp == "" || acceptedUser.Id.GetIdp() == remoteUserID.Idp) {
			return acceptedUser, nil
		}
	}
	return nil, errtypes.NotFound(remoteUserID.OpaqueId)
}

func (m *manager) FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, query string) ([]*userpb.User, error) {
	m.RLock()
	defer m.RUnlock()

	users := []*userpb.User{}
	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
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
