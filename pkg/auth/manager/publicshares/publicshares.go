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

package publicshares

import (
	"context"
	"strings"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	userprovider "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("publicshares", New)
}

type manager struct {
	c *config
}

type config struct {
	GatewayAddr string `mapstructure:"gateway_addr"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{
		c: conf,
	}, nil
}

func (m *manager) Authenticate(ctx context.Context, token string, secret string) (*user.User, error) {
	parts := strings.Split(token, "/")

	gwConn, err := pool.GetGatewayServiceClient(m.c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	publicShareResponse, err := gwConn.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{Token: parts[1]})
	if err != nil {
		return nil, err
	}

	// how can basic auth flow be triggered from here?
	if publicShareResponse.Share.GetPasswordProtected() {
		return nil, errors.New("resource password protected, bearer token not found")
	}

	getUserResponse, err := gwConn.GetUser(ctx, &userprovider.GetUserRequest{
		UserId: publicShareResponse.GetShare().GetCreator(),
	})
	if err != nil {
		return nil, err
	}

	return getUserResponse.GetUser(), nil
}
