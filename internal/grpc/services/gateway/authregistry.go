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

	authregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/authregistry/v0alpha"
	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) ListAuthProviders(ctx context.Context, req *authregistryv0alphapb.ListAuthProvidersRequest) (*gatewayv0alphapb.ListAuthProvidersResponse, error) {
	c, err := pool.GetAuthRegistryServiceClient(s.c.AuthRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting auth registry client")
		return &gatewayv0alphapb.ListAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "gateway"),
		}, nil
	}

	res, err := c.ListAuthProviders(ctx, req)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling ListAuthProviders")
		return &gatewayv0alphapb.ListAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "gateway"),
		}, nil
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		return &gatewayv0alphapb.ListAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "gateway"),
		}, nil
	}

	types := make([]string, len(res.Providers))
	for i, p := range res.Providers {
		types[i] = p.ProviderType
	}

	gwRes := &gatewayv0alphapb.ListAuthProvidersResponse{
		Status: res.Status,
		Opaque: res.Opaque,
		Types:  types,
	}

	return gwRes, nil
}
