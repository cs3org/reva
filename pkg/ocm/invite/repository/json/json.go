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

package json

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/cs3org/reva/v3/pkg/utils/list"
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

func (c *config) ApplyDefaults() {
	if c.File == "" {
		c.File = "/var/tmp/reva/ocm-invites.json"
	}
}

// New returns a new invite manager object.
func New(ctx context.Context, m map[string]any) (invite.Repository, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		return nil, errors.Wrap(err, "error loading the file containing the invites")
	}

	manager := &manager{
		config: &c,
		model:  model,
	}

	return manager, nil
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

func (model *inviteModel) clone() *inviteModel {
	return &inviteModel{
		File:          model.File,
		Invites:       maps.Clone(model.Invites),
		AcceptedUsers: maps.Clone(model.AcceptedUsers),
	}
}

func (m *manager) commitModel(candidate *inviteModel) error {
	if err := candidate.save(); err != nil {
		return errors.Wrap(err, "json: error saving model")
	}
	m.model = candidate
	return nil
}

func (model *inviteModel) save() error {
	data, err := json.Marshal(model)
	if err != nil {
		return errors.Wrap(err, "error encoding invite data to json")
	}

	absPath, err := filepath.Abs(model.File)
	if err != nil {
		return errors.Wrap(err, "error resolving invite storage path")
	}
	targetPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return errors.Wrap(err, "error resolving invite storage symlinks")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return errors.Wrap(err, "error stating invite storage file")
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("invite storage path is not a regular file: %s", targetPath)
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".ocm-invites-*.tmp")
	if err != nil {
		return errors.Wrap(err, "error creating temporary invite storage file")
	}
	tmpPath := tmp.Name()
	removeTmp := func() {
		_ = os.Remove(tmpPath)
	}

	n, err := tmp.Write(data)
	if err != nil {
		_ = tmp.Close()
		removeTmp()
		return errors.Wrap(err, "error writing temporary invite storage file")
	}
	if n != len(data) {
		_ = tmp.Close()
		removeTmp()
		return errors.Wrap(io.ErrShortWrite, "error writing temporary invite storage file")
	}

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		removeTmp()
		return errors.Wrap(err, "error setting permissions on temporary invite storage file")
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		removeTmp()
		return errors.Wrap(err, "error syncing temporary invite storage file")
	}
	if err := tmp.Close(); err != nil {
		removeTmp()
		return errors.Wrap(err, "error closing temporary invite storage file")
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		removeTmp()
		return errors.Wrap(err, "error replacing invite storage file: "+targetPath)
	}

	return nil
}

func (m *manager) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	if err := invite.ValidateInviteToken(token); err != nil {
		return err
	}

	m.Lock()
	defer m.Unlock()

	candidate := m.model.clone()
	candidate.Invites[token.GetToken()] = token
	return m.commitModel(candidate)
}

func (m *manager) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errtypes.BadRequest("blank token")
	}

	m.RLock()
	defer m.RUnlock()

	if tkn, ok := m.model.Invites[token]; ok {
		if invite.ValidateInviteToken(tkn) != nil {
			return nil, errtypes.InternalError("stored invite token is malformed")
		}
		return tkn, nil
	}
	return nil, invite.ErrTokenNotFound
}

func (m *manager) ListTokens(ctx context.Context, initiator *userpb.UserId) ([]*invitepb.InviteToken, error) {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}

	m.RLock()
	defer m.RUnlock()

	log := appctx.GetLogger(ctx)
	tokens := []*invitepb.InviteToken{}
	var warnedMalformedToken bool
	for _, token := range m.model.Invites {
		if invite.ValidateInviteToken(token) != nil {
			if !warnedMalformedToken {
				log.Warn().Msg("skipping malformed stored invite token")
				warnedMalformedToken = true
			}
			continue
		}
		if utils.UserEqual(token.UserId, initiator) && !tokenIsExpired(token) {
			tokens = append(tokens, token)
		}
	}
	return tokens, nil
}

func tokenIsExpired(token *invitepb.InviteToken) bool {
	return token.Expiration != nil && token.Expiration.Seconds < uint64(time.Now().Unix())
}

func (m *manager) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return err
	}
	if err := invite.ValidateRemoteUser(remoteUser); err != nil {
		return err
	}

	m.Lock()
	defer m.Unlock()

	log := appctx.GetLogger(ctx)
	var warnedMalformedUser bool
	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
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

	candidate := m.model.clone()
	key := initiator.GetOpaqueId()
	candidate.AcceptedUsers[key] = append(slices.Clone(m.model.AcceptedUsers[key]), remoteUser)
	return m.commitModel(candidate)
}

func (m *manager) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}
	if err := invite.ValidateUserID(remoteUserID, "remote user id"); err != nil {
		return nil, err
	}

	m.RLock()
	defer m.RUnlock()

	log := appctx.GetLogger(ctx)
	var warnedMalformedUser bool
	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
		if invite.ValidateRemoteUser(acceptedUser) != nil {
			if !warnedMalformedUser {
				log.Warn().Msg("skipping malformed stored remote user")
				warnedMalformedUser = true
			}
			continue
		}
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
	if err := invite.ValidateUserID(initiator, "initiator id"); err != nil {
		return nil, err
	}

	m.RLock()
	defer m.RUnlock()

	log := appctx.GetLogger(ctx)
	users := []*userpb.User{}
	var warnedMalformedUser bool
	for _, acceptedUser := range m.model.AcceptedUsers[initiator.GetOpaqueId()] {
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

	m.Lock()
	defer m.Unlock()

	acceptedUsers, ok := m.model.AcceptedUsers[initiator.GetOpaqueId()]
	if !ok {
		return nil
	}

	log := appctx.GetLogger(ctx)
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
			candidate := m.model.clone()
			candidate.AcceptedUsers[initiator.GetOpaqueId()] = list.Remove(slices.Clone(acceptedUsers), i)
			return m.commitModel(candidate)
		}
	}
	return nil
}
