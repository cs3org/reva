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

package json

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"

	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appauth"
	"github.com/cs3org/reva/pkg/appauth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/sethvargo/go-password/password"
)

func init() {
	registry.Register("json", New)
}

type config struct {
	File          string `mapstructure:"file"`
	TokenStrength int    `mapstructure:"token_strength"`
}

type jsonManager struct {
	config *config
	model  *jsonModel
}

type jsonModel struct {
	file  string
	State map[string]*apppb.AppPassword `json:"state"`
}

// New returns a new mgr.
func New(m map[string]interface{}) (appauth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	c.init()

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the shares")
		return nil, err
	}

	return &jsonManager{config: c, model: model}, nil
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/etc/revad/appauth.json"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func loadOrCreate(file string) (*jsonModel, error) {
	stat, err := os.Stat(file)
	if os.IsNotExist(err) || stat.Size() == 0 {
		if err = ioutil.WriteFile(file, []byte("{}"), 0644); err != nil {
			return nil, errors.Wrapf(err, "error creating the file %s", file)
		}
	}

	fd, err := os.OpenFile(file, os.O_RDONLY, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening the file %s", file)
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading the file %s", file)
	}

	m := &jsonModel{file: file}
	if err = json.Unmarshal(data, &m.State); err != nil {
		return nil, errors.Wrapf(err, "error parsing the file %s", file)
	}

	if m.State == nil {
		m.State = make(map[string]*apppb.AppPassword)
	}

	return m, nil
}

func (mgr *jsonManager) GenerateAppPassword(ctx context.Context, scope map[string]*authpb.Scope, label string) (*apppb.AppPassword, error) {
	token, err := password.Generate(mgr.config.TokenStrength, 10, 10, false, false)
	if err != nil {
		return nil, errtypes.InternalError("error creating new token")
	}
	user := user.ContextMustGetUser(ctx)

	appPass := &apppb.AppPassword{
		Password:   token,
		TokenScope: scope,
		Label:      label,
		User:       user.GetId(),
	}
	mgr.model.State[token] = appPass

	err = mgr.model.save()
	if err != nil {
		return nil, errtypes.InternalError("error saving new token")
	}

	return appPass, nil
}

func (mgr *jsonManager) ListAppPasswords(ctx context.Context) ([]*apppb.AppPassword, error) {
	userId := user.ContextMustGetUser(ctx).GetId()
	var userPasswords []*apppb.AppPassword

	for _, appPassword := range mgr.model.State {
		if appPassword.User == userId {
			userPasswords = append(userPasswords, appPassword)
		}
	}

	return userPasswords, nil
}

func (mgr *jsonManager) InvalidateAppPassword(ctx context.Context, password string) error {
	if _, ok := mgr.model.State[password]; !ok {
		return errtypes.BadRequest("password does not exist")
	}
	delete(mgr.model.State, password)
	return mgr.model.save()
}

func (mgr *jsonManager) GetAppPassword(ctx context.Context, user *userpb.UserId, password string) (*apppb.AppPassword, error) {
	appPassword, ok := mgr.model.State[password]
	if !ok {
		return nil, errtypes.BadRequest("password does not exist")
	}
	return appPassword, nil
}

func (m *jsonModel) save() error {
	data, err := json.Marshal(m.State)
	if err != nil {
		return errors.Wrap(err, "error encoding json file")
	}

	if err = ioutil.WriteFile(m.file, data, 0644); err != nil {
		return errors.Wrapf(err, "error writing to file %s", m.file)
	}

	return nil
}
