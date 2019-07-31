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

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/pkg/errors"
)

func (s *svc) GenerateAccessToken(ctx context.Context, req *authv0alphapb.GenerateAccessTokenRequest) (*authv0alphapb.GenerateAccessTokenResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetAuthServiceClient(s.c.AuthEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting auth client")
		return &authv0alphapb.GenerateAccessTokenResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.GenerateAccessToken(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GenerateAccessToken")
	}

	return res, nil
}

func (s *svc) WhoAmI(ctx context.Context, req *authv0alphapb.WhoAmIRequest) (*authv0alphapb.WhoAmIResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetAuthServiceClient(s.c.AuthEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting auth client")
		return &authv0alphapb.WhoAmIResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.WhoAmI(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling WhoAmI")
	}

	return res, nil
}
