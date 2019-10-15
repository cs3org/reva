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

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/token"
	tokenmgr "github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("authsvc", New)
}

type config struct {
	AuthManager          string                            `mapstructure:"auth_manager"`
	AuthManagers         map[string]map[string]interface{} `mapstructure:"auth_managers"`
	TokenManager         string                            `mapstructure:"token_manager"`
	TokenManagers        map[string]map[string]interface{} `mapstructure:"token_managers"`
	UserProviderEndpoint string                            `mapstructure:"userprovidersvc"`
}

type service struct {
	authmgr  auth.Manager
	tokenmgr token.Manager
	conf     *config
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
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

// New returns a new AuthProviderServiceServer.
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

	svc := &service{conf: c, authmgr: authManager, tokenmgr: tokenManager}
	authproviderv0alphapb.RegisterAuthProviderServiceServer(ss, svc)

	return svc, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) GenerateAccessToken(ctx context.Context, req *authproviderv0alphapb.GenerateAccessTokenRequest) (*authproviderv0alphapb.GenerateAccessTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	username := req.ClientId
	password := req.ClientSecret

	uid, err := s.authmgr.Authenticate(ctx, username, password)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in Authenticate")
		res := &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewUnauthenticated(ctx, err, "error authenticating user"),
		}
		return res, nil
	}

	c, err := pool.GetUserProviderServiceClient(s.conf.UserProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error getting user provider client")
		return &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewInternal(ctx, err, "error getting user provider service client"),
		}, nil
	}

	getUserReq := &userproviderv0alphapb.GetUserRequest{
		UserId: uid,
	}

	getUserRes, err := c.GetUser(ctx, getUserReq)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in GetUser")
		res := &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}
		return res, nil
	}

	if getUserRes.Status.Code != rpcpb.Code_CODE_OK {
		err := status.NewErrorFromCode(getUserRes.Status.Code, "authsvc")
		return &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}, nil
	}

	user := getUserRes.User
	accessToken, err := s.tokenmgr.MintToken(ctx, user)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in MintToken")
		res := &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewUnauthenticated(ctx, err, "error creating access token"),
		}
		return res, nil
	}

	log.Info().Msgf("user %s authenticated", user.Username)
	res := &authproviderv0alphapb.GenerateAccessTokenResponse{
		Status:      status.NewOK(ctx),
		AccessToken: accessToken,
		UserId:      uid,
	}
	return res, nil
}

func (s *service) WhoAmI(ctx context.Context, req *authproviderv0alphapb.WhoAmIRequest) (*authproviderv0alphapb.WhoAmIResponse, error) {
	u, err := s.tokenmgr.DismantleToken(ctx, req.AccessToken)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error getting user from access token")
		return &authproviderv0alphapb.WhoAmIResponse{
			Status: status.NewUnauthenticated(ctx, err, "error dismantling access token"),
		}, nil
	}

	res := &authproviderv0alphapb.WhoAmIResponse{
		Status: status.NewOK(ctx),
		User:   u,
	}
	return res, nil
}
