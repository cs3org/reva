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

// Package nextcloud verifies a clientID and clientSecret against a Nextcloud backend.
package nextcloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("nextcloud", New)
}

type mgr struct {
	client   *http.Client
	endPoint string
}

type config struct {
	EndPoint string `mapstructure:"endpoint" docs:";The Nextcloud backend endpoint for user check"`
}

// Action describes a REST request to forward to the Nextcloud backend
type Action struct {
	verb     string
	username string
	argS     string
}

func (c *config) init() {
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an auth manager implementation that verifies against a Nextcloud backend.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	return &mgr{
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		client:   &http.Client{},
	}, nil
}

func (am *mgr) Configure(ml map[string]interface{}) error {
	return nil
}

func (am *mgr) do(ctx context.Context, a Action) (int, []byte, error) {
	log := appctx.GetLogger(ctx)
	// user, err := getUser(ctx)
	// if err != nil {
	// 	return 0, nil, err
	// }
	// url := am.endPoint + "~" + a.username + "/api/" + a.verb
	url := "http://localhost/apps/sciencemesh/~" + a.username + "/api/" + a.verb
	log.Info().Msgf("am.do %s %s", url, a.argS)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := am.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	log.Info().Msgf("am.do response %d %s", resp.StatusCode, body)
	return resp.StatusCode, body, nil
}

// Authenticate method as defined in https://github.com/cs3org/reva/blob/28500a8/pkg/auth/auth.go#L31-L33
func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, map[string]*authpb.Scope, error) {
	var params = map[string]string{
		"password": clientSecret,
		// "username": clientID,
	}
	bodyStr, err := json.Marshal(params)
	if err != nil {
		return nil, nil, err
	}
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Authenticate %s %s", clientID, bodyStr)

	statusCode, _, err := am.do(ctx, Action{"Authenticate", clientID, string(bodyStr)})

	if err != nil {
		return nil, nil, err
	}

	if statusCode != 200 {
		return nil, nil, errors.New("Username/password not recognized by Nextcloud backend")
	}
	user := &user.User{
		Username: clientID,
		Id: &user.UserId{
			OpaqueId: clientID,
			Idp:      "localhost",
			Type:     1,
		},
		Mail:        string(clientID),
		DisplayName: string(clientID),
		Groups:      nil,
	}
	return user, nil, nil
}
