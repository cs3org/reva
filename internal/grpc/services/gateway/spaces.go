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

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	c, err := pool.GetSpacesClient(pool.Endpoint(s.c.SpacesEndpoint))
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.CreateStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateStorageSpace")
	}

	return res, nil
}

func (s *svc) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	c, err := pool.GetSpacesClient(pool.Endpoint(s.c.SpacesEndpoint))
	if err != nil {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListStorageSpaces")
	}

	return res, nil
}

func (s *svc) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	c, err := pool.GetSpacesClient(pool.Endpoint(s.c.SpacesEndpoint))
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.UpdateStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateStorageSpace")
	}

	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	c, err := pool.GetSpacesClient(pool.Endpoint(s.c.SpacesEndpoint))
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.DeleteStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling DeleteStorageSpace")
	}

	return res, nil
}
