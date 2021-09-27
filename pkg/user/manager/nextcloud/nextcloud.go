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

package nextcloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	// "github.com/cs3org/reva/pkg/errtypes"
)

func init() {
	registry.Register("nextcloud", New)
}

type manager struct {
	client   *http.Client
	endPoint string
}

// UserManagerConfig contains config for a Nextcloud-based UserManager
type UserManagerConfig struct {
	EndPoint string `mapstructure:"endpoint" docs:";The Nextcloud backend endpoint for user management"`
}

func (c *UserManagerConfig) init() {
	if c.EndPoint == "" {
		c.EndPoint = "http://localhost/end/point?"
	}
}

func parseConfig(m map[string]interface{}) (*UserManagerConfig, error) {
	c := &UserManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()
	return c, nil
}

// Action describes a REST request to forward to the Nextcloud backend
type Action struct {
	verb string
	argS string
}

// New returns a user manager implementation that reads a json file to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return NewUserManager(c, &http.Client{})
}

// NewUserManager returns a new Nextcloud-based UserManager
func NewUserManager(c *UserManagerConfig, hc *http.Client) (user.Manager, error) {
	return &manager{
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		client:   hc,
	}, nil
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "nextcloud storage driver: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (m *manager) do(ctx context.Context, a Action) (int, []byte, error) {
	user, err := getUser(ctx)
	if err != nil {
		return 0, nil, err
	}
	url := m.endPoint + "~" + user.Username + "/api/user/" + a.verb
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

func (m *manager) Configure(ml map[string]interface{}) error {
	return nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	bodyStr, err := json.Marshal(uid)
	if err != nil {
		return nil, err
	}
	_, respBody, err := m.do(ctx, Action{"GetUser", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	result := &userpb.User{}
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		return nil, err
	}
	return result, err
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	type paramsObj struct {
		Claim string `json:"claim"`
		Value string `json:"value"`
	}
	bodyObj := &paramsObj{
		Claim: claim,
		Value: value,
	}
	bodyStr, _ := json.Marshal(bodyObj)
	_, respBody, err := m.do(ctx, Action{"GetUserByClaim", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	result := &userpb.User{}
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		return nil, err
	}
	return result, err
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	bodyStr, err := json.Marshal(uid)
	if err != nil {
		return nil, err
	}
	_, respBody, err := m.do(ctx, Action{"GetUserGroups", string(bodyStr)})
	if err != nil {
		return nil, err
	}
	var gs []string
	err = json.Unmarshal(respBody, &gs)
	if err != nil {
		return nil, err
	}
	return gs, err
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	_, respBody, err := m.do(ctx, Action{"FindUsers", query})
	if err != nil {
		return nil, err
	}
	var respArr []userpb.User
	err = json.Unmarshal(respBody, &respArr)
	if err != nil {
		return nil, err
	}
	var pointers = make([]*userpb.User, len(respArr))
	for i := 0; i < len(respArr); i++ {
		pointers[i] = &respArr[i]
	}
	return pointers, err
}
