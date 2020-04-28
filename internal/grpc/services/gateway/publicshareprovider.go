// Copyright 2018-2020 CERN
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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest) (*link.CreatePublicShareResponse, error) {
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

	// Grants are not accessible through a link.PublicShare, in order for a service to access a grant (link.Grant) this needs to be
	// commited to the Storage, for later retrievals. The following intends to do just that.

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	// if s.c.CommitShareToStorageGrant {
	// 	raw, err := json.Marshal(req.Grant)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	grantReq := &provider.AddGrantRequest{
	// 		Ref: &provider.Reference{
	// 			Spec: &provider.Reference_Id{
	// 				Id: req.ResourceInfo.Id,
	// 			},
	// 		},
	// 		Grant: &provider.Grant{
	// 			Permissions: req.Grant.Permissions.Permissions,
	// 		},
	// 		Opaque: &typesv1beta1.Opaque{
	// 			Map: map[string]*typesv1beta1.OpaqueEntry{
	// 				"grant": &typesv1beta1.OpaqueEntry{
	// 					Decoder: "json",
	// 					Value:   raw,
	// 				},
	// 			},
	// 		},
	// 	}

	// 	log.Info().Interface("grantReq", grantReq).Msg("commiting share to storage grant")

	// 	c, err := s.findByID(ctx, req.ResourceInfo.Id)
	// 	if err != nil {
	// 		if _, ok := err.(errtypes.IsNotFound); ok {
	// 			return &link.CreatePublicShareResponse{
	// 				Status: status.NewNotFound(ctx, "storage provider not found"),
	// 			}, nil
	// 		}
	// 		return &link.CreatePublicShareResponse{
	// 			Status: status.NewInternal(ctx, err, "error finding storage provider"),
	// 		}, nil
	// 	}

	// 	grantRes, err := c.AddGrant(ctx, grantReq)
	// 	if err != nil {
	// 		return nil, errors.Wrap(err, "gateway: error calling AddGrant")
	// 	}
	// 	if grantRes.Status.Code != rpc.Code_CODE_OK {
	// 		return &link.CreatePublicShareResponse{
	// 			Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
	// 				"error committing share to storage grant"),
	// 		}, nil
	// 	}
	// }

	// TODO(refs) commit to storage if configured
	return res, nil
}

func (s *svc) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest) (*link.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &link.RemovePublicShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *svc) GetPublicShareByToken(ctx context.Context, req *link.GetPublicShareByTokenRequest) (*link.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share by token")

	pClient, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error connecting to a public share provider")
		return &link.GetPublicShareByTokenResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	return pClient.GetPublicShareByToken(ctx, req)
}

func (s *svc) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest) (*link.GetPublicShareResponse, error) {
	// TODO if the date is expired DO NOT return
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share")

	pClient, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error connecting to a public share provider")
		return &link.GetPublicShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	return pClient.GetPublicShare(ctx, req)
}

func (s *svc) ListPublicShares(ctx context.Context, req *link.ListPublicSharesRequest) (*link.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("listing public shares")

	pClient, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error connecting to a public share provider")
		return &link.ListPublicSharesResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := pClient.ListPublicShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error listing shares")
	}

	return res, nil
}

func (s *svc) UpdatePublicShare(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("update public share")

	pClient, err := pool.GetPublicShareProviderClient(s.c.PublicShareProviderEndpoint)
	if err != nil {
		log.Err(err).Msg("error connecting to a public share provider")
		return &link.UpdatePublicShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := pClient.UpdatePublicShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error updating share")
	}
	return res, nil
}
