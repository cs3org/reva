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

package authprovider

import (
	"context"
	"fmt"

	provider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("authprovider", New)
}

type config struct {
	AuthManager  string                            `mapstructure:"auth_manager"`
	AuthManagers map[string]map[string]interface{} `mapstructure:"auth_managers"`
}

func (c *config) init() {
	if c.AuthManager == "" {
		c.AuthManager = "json"
	}
}

type service struct {
	authmgr auth.Manager
	conf    *config
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

func getAuthManager(manager string, m map[string]map[string]interface{}) (auth.Manager, error) {
	if manager == "" {
		return nil, errtypes.InternalError("authsvc: driver not configured for auth manager")
	}
	if f, ok := registry.NewFuncs[manager]; ok {
		return f(m[manager])
	}
	return nil, errtypes.NotFound(fmt.Sprintf("authsvc: driver %s not found for auth manager", manager))
}

// New returns a new AuthProviderServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	authManager, err := getAuthManager(c.AuthManager, c.AuthManagers)
	if err != nil {
		return nil, err
	}

	svc := &service{conf: c, authmgr: authManager}

	return svc, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.auth.provider.v1beta1.ProviderAPI/Authenticate"}
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func (s *service) Authenticate(ctx context.Context, req *provider.AuthenticateRequest) (*provider.AuthenticateResponse, error) {
	log := appctx.GetLogger(ctx)
	username := req.ClientId
	password := req.ClientSecret

	u, scope, err := s.authmgr.Authenticate(ctx, username, password)
	switch v := err.(type) {
	case nil:
		log.Info().Msgf("user %s authenticated", u.String())
		return &provider.AuthenticateResponse{
			Status:     status.NewOK(ctx),
			User:       u,
			TokenScope: scope,
		}, nil
	case errtypes.InvalidCredentials:
		return &provider.AuthenticateResponse{
			Status: status.NewPermissionDenied(ctx, v, "wrong password"),
		}, nil
	case errtypes.NotFound:
		return &provider.AuthenticateResponse{
			Status: status.NewNotFound(ctx, "unknown client id"),
		}, nil
	default:
		err = errors.Wrap(err, "authsvc: error in Authenticate")
		return &provider.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error authenticating user"),
		}, nil
	}

}
