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
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

// TODO(labkode): add multi-phase commit logic when commit share or commit ref is enabled.
func (s *svc) CreateOCMShare(ctx context.Context, req *ocm.CreateOCMShareRequest) (*ocm.CreateOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.CreateOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateShare")
	}

	return res, nil
}

func (s *svc) RemoveOCMShare(ctx context.Context, req *ocm.RemoveOCMShareRequest) (*ocm.RemoveOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		return &ocm.RemoveOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.RemoveOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveShare")
	}
	return res, nil
}

// TODO(labkode): we need to validate share state vs storage grant and storage ref
// If there are any inconsitencies, the share needs to be flag as invalid and a background process
// or active fix needs to be performed.
func (s *svc) GetOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	return s.getOCMShare(ctx, req)
}

func (s *svc) getOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.GetOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.GetOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetShare")
	}

	return res, nil
}

// TODO(labkode): read GetShare comment.
func (s *svc) ListOCMShares(ctx context.Context, req *ocm.ListOCMSharesRequest) (*ocm.ListOCMSharesResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.ListOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListOCMShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateOCMShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) (*ocm.UpdateOCMShareResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.UpdateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateOCMShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateShare")
	}

	return res, nil
}

func (s *svc) ListReceivedOCMShares(ctx context.Context, req *ocm.ListReceivedOCMSharesRequest) (*ocm.ListReceivedOCMSharesResponse, error) {
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.ListReceivedOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.ListReceivedOCMShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListReceivedShares")
	}

	return res, nil
}

func (s *svc) UpdateReceivedOCMShare(ctx context.Context, req *ocm.UpdateReceivedOCMShareRequest) (*ocm.UpdateReceivedOCMShareResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := pool.GetOCMShareProviderClient(s.c.OCMShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateReceivedOCMShare(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}
	return res, nil
}
