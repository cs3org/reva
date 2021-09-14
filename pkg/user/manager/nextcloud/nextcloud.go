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
	"io"
	"net/http"
	"strings"

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

type config struct {
	EndPoint string `mapstructure:"end_point"`
	MockHTTP bool   `mapstructure:"mock_http"`
}

func (c *config) init() {
	if c.EndPoint == "" {
		c.EndPoint = "http://localhost/end/point?"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
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

	return &manager{
		client:   &http.Client{},
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
	}, nil
}

func (m *manager) do(a Action) (int, []byte, error) {
	url := m.endPoint + a.verb
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
	_, respBody, err := m.do(Action{"GetUser", uid.Idp})
	u := &userpb.User{
		Username: string(respBody),
	}
	return u, err
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	_, respBody, err := m.do(Action{"FindUsers", query})
	u := &userpb.User{
		Username: string(respBody),
	}
	var us = make([]*userpb.User, 1)
	us[0] = u
	return us, err
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	_, respBody, err := m.do(Action{"GetUserByClaim", value})
	u := &userpb.User{
		Username: string(respBody),
	}
	return u, err
}

// func extractClaim(u *userpb.User, claim string) (string, error) {
// 	_, respBody, err := m.do(Action{"ExtractClaim", claim})
// 	u := &userpb.User{
// 		Username: string(respBody),
// 	}
// 	return u, err
// }

// func userContains(u *userpb.User, query string) bool {
// 	_, respBody, err := m.do(Action{"userContains", query})
// 	u := &userpb.User{
// 		Username: string(respBody),
// 	}
// 	return u, err
// 	return false
// }

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	_, respBody, err := m.do(Action{"GetUserGroups", uid.Idp})
	var gs = make([]string, 1)
	gs[0] = string(respBody)
	return gs, err
}
