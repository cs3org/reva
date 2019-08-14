// Copyright 2018-2019 CERN
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

package authsvc

import (
	"context"
	"fmt"
	"io"

	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	tokenmgr "github.com/cs3org/reva/pkg/token/manager/registry"
	usermgr "github.com/cs3org/reva/pkg/user/manager/registry"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"

	typespb "github.com/cs3org/go-cs3apis/cs3/types"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	grpcserver.Register("authsvc", New)
}

type config struct {
	AuthManager   string                            `mapstructure:"auth_manager"`
	AuthManagers  map[string]map[string]interface{} `mapstructure:"auth_managers"`
	TokenManager  string                            `mapstructure:"token_manager"`
	TokenManagers map[string]map[string]interface{} `mapstructure:"token_managers"`
	UserManager   string                            `mapstructure:"user_manager"`
	UserManagers  map[string]map[string]interface{} `mapstructure:"user_managers"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func getUserManager(manager string, m map[string]map[string]interface{}) (user.Manager, error) {
	if f, ok := usermgr.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for user manager", manager)
}

func getAuthManager(manager string, m map[string]map[string]interface{}) (auth.Manager, error) {
	if f, ok := registry.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for auth manager", manager)
}

func getTokenManager(manager string, m map[string]map[string]interface{}) (token.Manager, error) {
	if f, ok := tokenmgr.NewFuncs[manager]; ok {
		return f(m[manager])
	}

	return nil, fmt.Errorf("driver %s not found for token manager", manager)
}

// New returns a new AuthServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	authManager, err := getAuthManager(c.AuthManager, c.AuthManagers)
	if err != nil {
		return nil, err
	}

	tokenManager, err := getTokenManager(c.TokenManager, c.TokenManagers)
	if err != nil {
		return nil, err
	}

	userManager, err := getUserManager(c.UserManager, c.UserManagers)
	if err != nil {
		return nil, err
	}

	svc := &service{authmgr: authManager, tokenmgr: tokenManager, usermgr: userManager}
	authv0alphapb.RegisterAuthServiceServer(ss, svc)

	return svc, nil
}

type service struct {
	authmgr  auth.Manager
	tokenmgr token.Manager
	usermgr  user.Manager
}

func (s *service) Close() error {
	return nil
}

func (s *service) GenerateAccessToken(ctx context.Context, req *authv0alphapb.GenerateAccessTokenRequest) (*authv0alphapb.GenerateAccessTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	username := req.ClientId
	password := req.ClientSecret
	uid := &typespb.UserId{OpaqueId: username}

	ctx, err := s.authmgr.Authenticate(ctx, username, password)
	if err != nil {
		log.Error().Err(err).Msg("error authentication user")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	user, err := s.usermgr.GetUser(ctx, uid)
	if err != nil {
		log.Error().Err(err).Msg("error getting user information")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	accessToken, err := s.tokenmgr.MintToken(ctx, user)
	if err != nil {
		err = errors.Wrap(err, "error creating access token")
		log.Error().Err(err).Msg("error creating access token")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.GenerateAccessTokenResponse{Status: status}
		return res, nil
	}

	log.Info().Msgf("user %s authenticated", user.Username)
	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &authv0alphapb.GenerateAccessTokenResponse{Status: status, AccessToken: accessToken}
	return res, nil

}

func (s *service) WhoAmI(ctx context.Context, req *authv0alphapb.WhoAmIRequest) (*authv0alphapb.WhoAmIResponse, error) {
	log := appctx.GetLogger(ctx)
	token := req.AccessToken
	u, err := s.tokenmgr.DismantleToken(ctx, token)
	if err != nil {
		err = errors.Wrap(err, "error getting user from access token")
		log.Error().Err(err).Msg("error dismantling access token")
		status := &rpcpb.Status{Code: rpcpb.Code_CODE_UNAUTHENTICATED}
		res := &authv0alphapb.WhoAmIResponse{Status: status}
		return res, nil
	}

	status := &rpcpb.Status{Code: rpcpb.Code_CODE_OK}
	res := &authv0alphapb.WhoAmIResponse{Status: status, User: u}
	return res, nil
}
