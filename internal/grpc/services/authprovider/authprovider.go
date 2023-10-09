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
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("authprovider", New)
}

type config struct {
	AuthManager  string                            `mapstructure:"auth_manager"`
	AuthManagers map[string]map[string]interface{} `mapstructure:"auth_managers"`
	blockedUsers []string
}

func (c *config) ApplyDefaults() {
	if c.AuthManager == "" {
		c.AuthManager = "json"
	}
	c.blockedUsers = sharedconf.GetBlockedUsers()
}

type service struct {
	authmgr      auth.Manager
	conf         *config
	blockedUsers user.BlockedUsers
}

func getAuthManager(ctx context.Context, manager string, m map[string]map[string]interface{}) (auth.Manager, error) {
	if manager == "" {
		return nil, errtypes.InternalError("authsvc: driver not configured for auth manager")
	}
	if f, ok := registry.NewFuncs[manager]; ok {
		authmgr, err := f(ctx, m[manager])
		return authmgr, err
	}
	return nil, errtypes.NotFound(fmt.Sprintf("authsvc: driver %s not found for auth manager", manager))
}

// New returns a new AuthProviderServiceServer.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	authManager, err := getAuthManager(ctx, c.AuthManager, c.AuthManagers)
	if err != nil {
		return nil, err
	}

	svc := &service{
		conf:         &c,
		authmgr:      authManager,
		blockedUsers: user.NewBlockedUsersSet(c.blockedUsers),
	}

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

	if s.blockedUsers.IsBlocked(username) {
		return &provider.AuthenticateResponse{
			Status: status.NewPermissionDenied(ctx, errtypes.PermissionDenied(""), "user is blocked"),
		}, nil
	}

	u, scope, err := s.authmgr.Authenticate(ctx, username, password)
	switch v := err.(type) {
	case nil:
		log.Info().Interface("userId", u.Id).Msg("user authenticated")
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
			Status: status.NewNotFound(ctx, "unknown client id: "+err.Error()),
		}, nil
	default:
		err = errors.Wrap(err, "authsvc: error in Authenticate")
		return &provider.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error authenticating user"),
		}, nil
	}
}
