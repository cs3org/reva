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

	ocmauthorizer "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) IsProviderAllowed(ctx context.Context, req *ocmauthorizer.IsProviderAllowedRequest) (*ocmauthorizer.IsProviderAllowedResponse, error) {
	c, err := pool.GetOCMAuthorizerProviderClient(s.c.OCMAuthorizerProviderEndpoint)
	if err != nil {
		return &ocmauthorizer.IsProviderAllowedResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm authorizer provider client"),
		}, nil
	}

	res, err := c.IsProviderAllowed(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling IsProviderAllowed")
	}

	return res, nil
}

func (s *svc) GetInfoByDomain(ctx context.Context, req *ocmauthorizer.GetInfoByDomainRequest) (*ocmauthorizer.GetInfoByDomainResponse, error) {
	c, err := pool.GetOCMAuthorizerProviderClient(s.c.OCMAuthorizerProviderEndpoint)
	if err != nil {
		return &ocmauthorizer.GetInfoByDomainResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm authorizer provider client"),
		}, nil
	}

	res, err := c.GetInfoByDomain(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling IsProviderAllowed")
	}

	return res, nil
}

func (s *svc) ListAllProviders(ctx context.Context, req *ocmauthorizer.ListAllProvidersRequest) (*ocmauthorizer.ListAllProvidersResponse, error) {
	c, err := pool.GetOCMAuthorizerProviderClient(s.c.OCMAuthorizerProviderEndpoint)
	if err != nil {
		return &ocmauthorizer.ListAllProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting ocm authorizer provider client"),
		}, nil
	}

	res, err := c.ListAllProviders(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling IsProviderAllowed")
	}

	return res, nil
}
