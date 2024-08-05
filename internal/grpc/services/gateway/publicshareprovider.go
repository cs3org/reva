// Copyright 2018-2024 CERN
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

	"github.com/alitto/pond"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest) (*link.CreatePublicShareResponse, error) {
	if s.isSharedFolder(ctx, req.ResourceInfo.GetPath()) {
		return nil, errtypes.AlreadyExists("gateway: can't create a public share of the share folder itself")
	}

	log := appctx.GetLogger(ctx)
	log.Info().Msg("create public share")

	c, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
	if err != nil {
		return nil, err
	}

	res, err := c.CreatePublicShare(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (s *svc) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest) (*link.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	driver, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
	if err != nil {
		return nil, err
	}
	res, err := driver.RemovePublicShare(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *svc) GetPublicShareByToken(ctx context.Context, req *link.GetPublicShareByTokenRequest) (*link.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share by token")

	driver, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
	if err != nil {
		return nil, err
	}

	res, err := driver.GetPublicShareByToken(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (s *svc) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest) (*link.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share")

	pClient, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
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

	pClient, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
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

func (s *svc) ListExistingPublicShares(ctx context.Context, req *link.ListPublicSharesRequest) (*gateway.ListExistingPublicSharesResponse, error) {
	shares, err := s.ListPublicShares(ctx, req)
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling ListExistingReceivedShares")
		return &gateway.ListExistingPublicSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing received shares"),
		}, nil
	}

	sharesCh := make(chan *gateway.PublicShareResourceInfo, len(shares.Share))
	pool := pond.New(50, len(shares.Share))
	for _, share := range shares.Share {
		share := share
		// TODO (gdelmont): we should report any eventual error raised by the goroutines
		pool.Submit(func() {
			// TODO(lopresti) incorporate the cache layer from internal/http/services/owncloud/ocs/handlers/apps/sharing/shares/shares.go
			stat, err := s.Stat(ctx, &provider.StatRequest{
				Ref: &provider.Reference{
					ResourceId: share.ResourceId,
				},
			})
			if err != nil {
				return
			}
			if stat.Status.Code != rpc.Code_CODE_OK {
				return
			}

			sharesCh <- &gateway.PublicShareResourceInfo{
				ResourceInfo: stat.Info,
				PublicShare:  share,
			}
		})
	}

	sris := make([]*gateway.PublicShareResourceInfo, 0, len(shares.Share))
	done := make(chan struct{})
	go func() {
		for s := range sharesCh {
			sris = append(sris, s)
		}
		done <- struct{}{}
	}()
	pool.StopAndWait()
	close(sharesCh)
	<-done
	close(done)

	return &gateway.ListExistingPublicSharesResponse{
		ShareInfos: sris,
		Status:     status.NewOK(ctx),
	}, nil
}

func (s *svc) UpdatePublicShare(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("update public share")

	pClient, err := pool.GetPublicShareProviderClient(pool.Endpoint(s.c.PublicShareProviderEndpoint))
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
