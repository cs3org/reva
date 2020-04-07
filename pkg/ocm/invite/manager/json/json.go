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

package json

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/manager/registry"
	"github.com/cs3org/reva/pkg/ocm/invite/token"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type inviteModel struct {
	File          string
	Invites       map[string]*invitepb.InviteToken `json:"invites"`
	AcceptedUsers map[string][]*userpb.UserId      `json:"accepted_users"`
}

type manager struct {
	config     *config
	sync.Mutex // concurrent access to the file
	model      *inviteModel
}

type config struct {
	File       string `mapstructure:"file"`
	Expiration string `mapstructure:"expiration"`
}

func init() {
	registry.Register("json", New)
}

// New returns a new invite manager object.
func New(m map[string]interface{}) (invite.Manager, error) {

	config, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error parse config for json invite manager")
		return nil, err
	}

	if config.Expiration == "" {
		config.Expiration = token.DefaultExpirationTime
	}

	// if file is not set we use temporary file
	if config.File == "" {
		dir, err := ioutil.TempDir("", "")
		if err != nil {
			err = errors.Wrap(err, "error creating temporary directory for json invite manager")
			return nil, err
		}
		config.File = path.Join(dir, "invites.json")
	}

	// load or create file
	model, err := loadOrCreate(config.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the invites")
		return nil, err
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
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "error opening/creating the invite storage file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening/creating the invite storage file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	model := &inviteModel{}
	if err := json.Unmarshal(data, model); err != nil {
		err = errors.Wrap(err, "error decoding invite data to json")
		return nil, err
	}

	model.File = file
	return model, nil
}

func (model *inviteModel) Save() error {
	data, err := json.Marshal(model)
	if err != nil {
		err = errors.Wrap(err, "error encoding invite data to json")
		return err
	}

	if err := ioutil.WriteFile(model.File, data, 0644); err != nil {
		err = errors.Wrap(err, "error writing invite data to file: "+model.File)
		return err
	}

	return nil
}

func (m *manager) GenerateToken(ctx context.Context) (*invitepb.InviteToken, error) {

	contexUser := user.ContextMustGetUser(ctx)

	// Create mutex lock
	m.Lock()
	defer m.Unlock()

	// Creating a unique token
	var inviteToken *invitepb.InviteToken
	for {
		tmpInviteToken, err := token.CreateToken(m.config.Expiration, contexUser.GetId())
		if err != nil {
			return nil, err
		}
		_, ok := m.model.Invites[tmpInviteToken.GetToken()]
		if !ok {
			inviteToken = tmpInviteToken
			break
		}
	}

	// Store token data
	m.model.Invites[inviteToken.GetToken()] = inviteToken
	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	return inviteToken, nil
}

func (m *manager) ForwardInvite(ctx context.Context, invite *invitepb.InviteToken, originProvider *ocm.ProviderInfo) error {

	contexUser := user.ContextMustGetUser(ctx)
	requestBody := url.Values{
		"token":              {invite.GetToken()},
		"userID":             {contexUser.GetId().GetOpaqueId()},
		"sender_provider":    {originProvider.GetDomain()},
		"recipient_provider": {contexUser.GetId().GetIdp()},
	}

	resp, err := http.PostForm(originProvider.GetApiEndpoint(), requestBody)
	if err != nil {
		err = errors.Wrap(err, "json: error sending post request")
		return err
	}

	resp.Body.Close()
	return nil
}

func (m *manager) AcceptInvite(ctx context.Context, invite *invitepb.InviteToken, userID *userpb.UserId) error {

	// Create mutex lock
	m.Lock()
	defer m.Unlock()

	inviteToken, err := getTokenIfValid(m, invite)
	if err != nil {
		return err
	}

	// Add to the list of accepted users
	userKey := generateKey(inviteToken.GetUserId())
	m.model.AcceptedUsers[userKey] = append(m.model.AcceptedUsers[userKey], userID)
	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "json: error saving model")
		return err
	}
	return nil
}

func getTokenIfValid(m *manager, token *invitepb.InviteToken) (*invitepb.InviteToken, error) {
	inviteToken, ok := m.model.Invites[token.GetToken()]
	if !ok {
		return nil, errors.New("json: invalid token")
	}
	if uint64(time.Now().Unix()) <= inviteToken.Expiration.Seconds {
		return nil, errors.New("json: token expired")
	}
	return inviteToken, nil
}

func generateKey(user *userpb.UserId) string {
	return fmt.Sprintf("%s_%s", user.GetOpaqueId(), user.GetIdp())
}
