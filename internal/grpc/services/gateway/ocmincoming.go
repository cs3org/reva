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

	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreateOCMIncomingShare(ctx context.Context, req *ocmincoming.CreateOCMIncomingShareRequest) (*ocmincoming.CreateOCMIncomingShareResponse, error) {
	c, err := pool.GetOCMIncomingClient(pool.Endpoint(s.c.OCMIncomingEndpoint))
	if err != nil {
		return &ocmincoming.CreateOCMIncomingShareResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm incoming client"),
		}, nil
	}

	res, err := c.CreateOCMIncomingShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateOCMIncomingShare")
	}

	return res, nil
}

// We return not implemented since the api is deprecated but needs to be implemented to register the gateway service.
func (s *svc) CreateOCMCoreShare(ctx context.Context, req *ocmcore.CreateOCMCoreShareRequest) (*ocmcore.CreateOCMCoreShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (s *svc) UpdateOCMIncomingShare(ctx context.Context, req *ocmincoming.UpdateOCMIncomingShareRequest) (*ocmincoming.UpdateOCMIncomingShareResponse, error) {
	c, err := pool.GetOCMIncomingClient(pool.Endpoint(s.c.OCMIncomingEndpoint))
	if err != nil {
		return &ocmincoming.UpdateOCMIncomingShareResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm incoming client"),
		}, nil
	}

	res, err := c.UpdateOCMIncomingShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateOCMIncomingShare")
	}

	return res, nil
}

// We return not implemented since the api is deprecated but needs to be implemented to register the gateway service.
func (s *svc) UpdateOCMCoreShare(ctx context.Context, req *ocmcore.UpdateOCMCoreShareRequest) (*ocmcore.UpdateOCMCoreShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (s *svc) DeleteOCMIncomingShare(ctx context.Context, req *ocmincoming.DeleteOCMIncomingShareRequest) (*ocmincoming.DeleteOCMIncomingShareResponse, error) {
	c, err := pool.GetOCMIncomingClient(pool.Endpoint(s.c.OCMIncomingEndpoint))
	if err != nil {
		return &ocmincoming.DeleteOCMIncomingShareResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm incoming client"),
		}, nil
	}

	res, err := c.DeleteOCMIncomingShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling DeleteOCMIncomingShare")
	}

	return res, nil
}

// We return not implemented since the api is deprecated but needs to be implemented to register the gateway service.
func (s *svc) DeleteOCMCoreShare(ctx context.Context, req *ocmcore.DeleteOCMCoreShareRequest) (*ocmcore.DeleteOCMCoreShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}
