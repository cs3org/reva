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

	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) GetAppProviders(ctx context.Context, req *appregistryv0alphapb.GetAppProvidersRequest) (*appregistryv0alphapb.GetAppProvidersResponse, error) {
	c, err := pool.GetAppRegistryClient(s.c.AppRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetOCMShareProviderClient")
		return &appregistryv0alphapb.GetAppProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.GetAppProviders(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListShares")
	}

	return res, nil
}

func (s *svc) ListAppProviders(ctx context.Context, req *appregistryv0alphapb.ListAppProvidersRequest) (*appregistryv0alphapb.ListAppProvidersResponse, error) {
	c, err := pool.GetAppRegistryClient(s.c.AppRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetOCMShareProviderClient")
		return &appregistryv0alphapb.ListAppProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListAppProviders(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListShares")
	}

	return res, nil
}
