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

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
)

func (s *svc) Open(ctx context.Context, req *appproviderv0alphapb.OpenRequest) (*appproviderv0alphapb.OpenResponse, error) {
	log := appctx.GetLogger(ctx)
	provider, err := s.findAppProvider(ctx, req.ResourceInfo)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding app provider")
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &appproviderv0alphapb.OpenResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}
		return &appproviderv0alphapb.OpenResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	c, err := pool.GetAppProviderClient(provider.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting appprovider client")
		return &appproviderv0alphapb.OpenResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.Open(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Open")
	}

	return res, nil
}

func (s *svc) findAppProvider(ctx context.Context, ri *storageproviderv0alphapb.ResourceInfo) (*appregistryv0alphapb.ProviderInfo, error) {
	c, err := pool.GetAppRegistryClient(s.c.AppRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error getting appregistry client")
		return nil, err
	}

	res, err := c.GetAppProviders(ctx, &appregistryv0alphapb.GetAppProvidersRequest{
		ResourceInfo: ri,
	})

	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetAppProviders")
		return nil, err
	}

	// TODO(labkode): when sending an Open to the proxy we need to choose one
	// provider from the list of available as the client
	if res.Status.Code == rpcpb.Code_CODE_OK {
		return res.Providers[0], nil
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gatewaysvc: app provider not found for resource:" + ri.String())
	}

	return nil, errors.New("gatewaysvc: error finding a storage provider")
}
