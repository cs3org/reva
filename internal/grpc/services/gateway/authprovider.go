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

package gateway

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) Authenticate(ctx context.Context, req *gateway.AuthenticateRequest) (*gateway.AuthenticateResponse, error) {
	log := appctx.GetLogger(ctx)

	// find auth provider
	c, err := s.findAuthProvider(ctx, req.Type)
	if err != nil {
		err = errors.New("gateway: error finding auth provider for type: " + req.Type)
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider client"),
		}, nil
	}

	authProviderReq := &provider.AuthenticateRequest{
		ClientId:     req.ClientId,
		ClientSecret: req.ClientSecret,
	}
	res, err := c.Authenticate(ctx, authProviderReq)
	if err != nil {
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting user provider service client"),
		}, nil
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		log.Err(err).Msgf("error authenticating credentials to auth provider for type: %s", req.Type)
		return &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, ""),
		}, nil
	}

	// validate valid userId
	if res.User == nil {
		err := errors.New("gateway: user after Authenticate is nil")
		log.Err(err).Msg("user is nil")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "user is nil"),
		}, nil
	}

	uid := res.User.Id
	if uid == nil {
		err := errors.New("gateway: uid after Authenticate is nil")
		log.Err(err).Msg("user id is nil")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "user id is nil"),
		}, nil
	}

	userClient, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error getting user provider client")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting user provider service client"),
		}, nil
	}

	getUserReq := &user.GetUserRequest{
		UserId: uid,
	}

	getUserRes, err := userClient.GetUser(ctx, getUserReq)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in GetUser")
		res := &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}
		return res, nil
	}

	if getUserRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(getUserRes.Status.Code, "authsvc")
		return &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}, nil
	}

	user := res.User

	token, err := s.tokenmgr.MintToken(ctx, user)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in MintToken")
		res := &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error creating access token"),
		}
		return res, nil
	}

	gwRes := &gateway.AuthenticateResponse{
		Status: status.NewOK(ctx),
		User:   res.User,
		Token:  token,
	}
	return gwRes, nil
}

func (s *svc) WhoAmI(ctx context.Context, req *gateway.WhoAmIRequest) (*gateway.WhoAmIResponse, error) {
	u, err := s.tokenmgr.DismantleToken(ctx, req.Token)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting user from token")
		return &gateway.WhoAmIResponse{
			Status: status.NewUnauthenticated(ctx, err, "error dismantling token"),
		}, nil
	}

	res := &gateway.WhoAmIResponse{
		Status: status.NewOK(ctx),
		User:   u,
	}
	return res, nil
}

func (s *svc) findAuthProvider(ctx context.Context, authType string) (provider.ProviderAPIClient, error) {
	c, err := pool.GetAuthRegistryServiceClient(s.c.AuthRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting auth registry client")
		return nil, err
	}

	res, err := c.GetAuthProvider(ctx, &registry.GetAuthProviderRequest{
		Type: authType,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAuthProvider")
		return nil, err
	}

	if res.Status.Code == rpc.Code_CODE_OK && res.Provider != nil {
		// TODO(labkode): check for capabilities here
		c, err := pool.GetAuthProviderServiceClient(res.Provider.Address)
		if err != nil {
			err = errors.Wrap(err, "gateway: error getting an auth provider client")
			return nil, err
		}

		return c, nil
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gateway: auth provider not found for type:" + authType)
	}

	return nil, errors.New("gateway: error finding an auth provider for type: " + authType)
}
