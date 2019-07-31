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

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/pkg/errors"
)

func (s *svc) CreateShare(ctx context.Context, req *usershareproviderv0alphapb.CreateShareRequest) (*usershareproviderv0alphapb.CreateShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.CreateShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.CreateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling CreateShare")
	}

	return res, nil
}

func (s *svc) RemoveShare(ctx context.Context, req *usershareproviderv0alphapb.RemoveShareRequest) (*usershareproviderv0alphapb.RemoveShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.RemoveShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.RemoveShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling RemoveShare")
	}

	return res, nil
}

func (s *svc) GetShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.GetShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.GetShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetShare")
	}

	return res, nil
}

func (s *svc) ListShares(ctx context.Context, req *usershareproviderv0alphapb.ListSharesRequest) (*usershareproviderv0alphapb.ListSharesResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.ListSharesResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.ListShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateShareRequest) (*usershareproviderv0alphapb.UpdateShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.UpdateShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.UpdateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling UpdateShare")
	}

	return res, nil
}

func (s *svc) ListReceivedShares(ctx context.Context, req *usershareproviderv0alphapb.ListReceivedSharesRequest) (*usershareproviderv0alphapb.ListReceivedSharesResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.ListReceivedSharesResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.ListReceivedShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListReceivedShares")
	}

	return res, nil
}

func (s *svc) UpdateReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateReceivedShareRequest) (*usershareproviderv0alphapb.UpdateReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting usershareprovider client")
		return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.UpdateReceivedShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling UpdateReceivedShare")
	}

	return res, nil
}
