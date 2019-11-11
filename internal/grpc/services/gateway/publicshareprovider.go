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

	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreatePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.CreatePublicShareRequest) (*publicshareproviderv0alphapb.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("create public share")

	c, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		return nil, err
	}

	res, err := c.CreatePublicShare(ctx, req)
	if err != nil {
		return nil, err
	}

	// TODO(refs) commit to storage if configured
	return res, nil
}

func (s *svc) RemovePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.RemovePublicShareRequest) (*publicshareproviderv0alphapb.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv0alphapb.RemovePublicShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *svc) GetPublicShareByToken(ctx context.Context, req *publicshareproviderv0alphapb.GetPublicShareByTokenRequest) (*publicshareproviderv0alphapb.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv0alphapb.GetPublicShareByTokenResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *svc) GetPublicShare(ctx context.Context, req *publicshareproviderv0alphapb.GetPublicShareRequest) (*publicshareproviderv0alphapb.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share")

	return &publicshareproviderv0alphapb.GetPublicShareResponse{
		Status: status.NewOK(ctx),
		// Share:  share,
	}, nil
}

func (s *svc) ListPublicShares(ctx context.Context, req *publicshareproviderv0alphapb.ListPublicSharesRequest) (*publicshareproviderv0alphapb.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("listing public shares")

	pClient, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error connecting to a public share provider")
		return &publicshareproviderv0alphapb.ListPublicSharesResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := pClient.ListPublicShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error calling ListShares")
	}

	// res := &publicshareproviderv0alphapb.ListPublicSharesResponse{
	// 	Status: status.NewOK(ctx),
	// 	Share: []*publicshareproviderv0alphapb.PublicShare{
	// &publicshareproviderv0alphapb.PublicShare{
	// 	Id: &publicshareproviderv0alphapb.PublicShareId{
	// 		OpaqueId: "some_publicly_shared_id",
	// 	},
	// 	Token:       "my_token",
	// 	ResourceId:  &v0alpha.ResourceId{},
	// 	Permissions: &publicshareproviderv0alphapb.PublicSharePermissions{},
	// 	Owner:       &types.UserId{},
	// 	Creator:     &types.UserId{},
	// 	Ctime:       &types.Timestamp{},
	// 	Expiration:  &types.Timestamp{},
	// 	Mtime:       &types.Timestamp{},
	// 	DisplayName: "some_public_share",
	// },
	// 	},
	// }
	return res, nil
}

func (s *svc) UpdatePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.UpdatePublicShareRequest) (*publicshareproviderv0alphapb.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("list public share")

	res := &publicshareproviderv0alphapb.UpdatePublicShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
