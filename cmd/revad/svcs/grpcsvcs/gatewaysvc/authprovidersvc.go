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
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
)

func (s *svc) GenerateAccessToken(ctx context.Context, req *gatewayv0alphapb.GenerateAccessTokenRequest) (*authproviderv0alphapb.GenerateAccessTokenResponse, error) {
	// find auth provider
	c, err := s.findAuthProvider(ctx, req.Type)
	if err != nil {
		return &authproviderv0alphapb.GenerateAccessTokenResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider client"),
		}, nil
	}

	authProviderReq := &authproviderv0alphapb.GenerateAccessTokenRequest{
		ClientId:     req.ClientId,
		ClientSecret: req.ClientSecret,
	}
	res, err := c.GenerateAccessToken(ctx, authProviderReq)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GenerateAccessToken")
	}

	return res, nil
}

func (s *svc) WhoAmI(ctx context.Context, req *authproviderv0alphapb.WhoAmIRequest) (*authproviderv0alphapb.WhoAmIResponse, error) {
	c, err := pool.GetAuthProviderServiceClient(s.c.AuthEndpoint)
	if err != nil {
		return &authproviderv0alphapb.WhoAmIResponse{
			Status: status.NewInternal(ctx, err, "error getting auth client"),
		}, nil
	}

	res, err := c.WhoAmI(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling WhoAmI")
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

	return nil, errors.New("gatewaysvc: error finding an auth provider")
}
