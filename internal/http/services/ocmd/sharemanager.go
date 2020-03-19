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

package ocmd

import (
	"context"
	"fmt"

	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/rs/zerolog"
)

func (h *sharesHandler) GetInternalShare(ctx context.Context, id string) (*share, error) {
	panic("implement me")
}

func (h *sharesHandler) NewShare(ctx context.Context, share *share, domain, shareWith string) (*share, error) {
	panic("implement me")
}

func (h *sharesHandler) getShares(ctx context.Context, logger *zerolog.Logger, user string) ([]*share, error) {

	gateway, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		return nil, err
	}

	filters := []*link.ListPublicSharesRequest_Filter{}
	req := link.ListPublicSharesRequest{
		Filters: filters,
	}

	logger.Debug().Str("gateway", fmt.Sprintf("%+v", gateway)).Str("req", fmt.Sprintf("%+v", req)).Msg("GetShares")

	res, err := gateway.ListPublicShares(ctx, &req)

	logger.Debug().Str("response", fmt.Sprintf("%+v", res)).Str("err", fmt.Sprintf("%+v", err)).Msg("GetShares")

	if err != nil {
		return nil, err
	}

	shares := make([]*share, 0)

	for i, publicShare := range res.GetShare() {
		logger.Debug().Str("idx", string(i)).Str("share", fmt.Sprintf("%+v", publicShare)).Msg("GetShares")

		share := convertPublicShareToShare(publicShare)
		shares = append(shares, share)
	}

	logger.Debug().Str("shares", fmt.Sprintf("%+v", shares)).Msg("GetShares")
	return shares, nil
}

func (s *svc) GetExternalShare(ctx context.Context, sharedWith, id string) (*share, error) {
	panic("implement me")
}

func convertPublicShareToShare(publicShare *link.PublicShare) *share {
	return &share{
		ID: publicShare.GetId().String(),
	}
}
