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

package gatewaysvc

import (
	"context"

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	authregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/authregistry/v0alpha"
	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
)

func (s *svc) Authenticate(ctx context.Context, req *gatewayv0alphapb.AuthenticateRequest) (*gatewayv0alphapb.AuthenticateResponse, error) {
	log := appctx.GetLogger(ctx)

	// find auth provider
	c, err := s.findAuthProvider(ctx, req.Type)
	if err != nil {
		err = errors.New("gatewaysvc: error finding auth provider for type: " + req.Type)
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider client"),
		}, nil
	}

	authProviderReq := &authproviderv0alphapb.AuthenticateRequest{
		ClientId:     req.ClientId,
		ClientSecret: req.ClientSecret,
	}
	res, err := c.Authenticate(ctx, authProviderReq)
	if err != nil {
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting user provider service client"),
		}, nil
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		err := status.NewErrorFromCode(res.Status.Code, "gatewaysvc")
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, ""),
		}, nil
	}

	// validate valid userId
	uid := res.UserId
	if uid == nil {
		err := errors.New("gatewaysvc: uid after Authenticate is nil")
		log.Err(err).Msg("user id is nil")
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "user id is nil"),
		}, nil
	}

	userClient, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error getting user provider client")
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting user provider service client"),
		}, nil
	}

	getUserReq := &userproviderv0alphapb.GetUserRequest{
		UserId: uid,
	}

	getUserRes, err := userClient.GetUser(ctx, getUserReq)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in GetUser")
		res := &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}
		return res, nil
	}

	if getUserRes.Status.Code != rpcpb.Code_CODE_OK {
		err := status.NewErrorFromCode(getUserRes.Status.Code, "authsvc")
		return &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error getting user information"),
		}, nil
	}

	user := getUserRes.User

	token, err := s.tokenmgr.MintToken(ctx, user)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in MintToken")
		res := &gatewayv0alphapb.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error creating access token"),
		}
		return res, nil
	}

	gwRes := &gatewayv0alphapb.AuthenticateResponse{
		Status: status.NewOK(ctx),
		UserId: res.UserId,
		Token:  token,
	}
	return gwRes, nil
}

func (s *svc) WhoAmI(ctx context.Context, req *gatewayv0alphapb.WhoAmIRequest) (*gatewayv0alphapb.WhoAmIResponse, error) {
	u, err := s.tokenmgr.DismantleToken(ctx, req.Token)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error getting user from token")
		return &gatewayv0alphapb.WhoAmIResponse{
			Status: status.NewUnauthenticated(ctx, err, "error dismantling token"),
		}, nil
	}

	res := &gatewayv0alphapb.WhoAmIResponse{
		Status: status.NewOK(ctx),
		User:   u,
	}
	return res, nil
}

func (s *svc) findAuthProvider(ctx context.Context, authType string) (authproviderv0alphapb.AuthProviderServiceClient, error) {
	c, err := pool.GetAuthRegistryServiceClient(s.c.AuthRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error getting auth registry client")
		return nil, err
	}

	res, err := c.GetAuthProvider(ctx, &authregistryv0alphapb.GetAuthProviderRequest{
		Type: authType,
	})

	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetAuthProvider")
		return nil, err
	}

	if res.Status.Code == rpcpb.Code_CODE_OK && res.Provider != nil {
		// TODO(labkode): check for capabilities here
		c, err := pool.GetAuthProviderServiceClient(res.Provider.Address)
		if err != nil {
			err = errors.Wrap(err, "gatewaysvc: error getting an auth provider client")
			return nil, err
		}

		return c, nil
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gatewaysvc: auth provider not found for type:" + authType)
	}

	return nil, errors.New("gatewaysvc: error finding an auth provider for type: " + authType)
}
