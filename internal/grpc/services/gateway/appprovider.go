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

	appproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/appprovider/v1beta1"
	appregistryv1beta1pb "github.com/cs3org/go-cs3apis/cs3/appregistry/v1beta1"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) Open(ctx context.Context, req *appproviderv1beta1pb.OpenRequest) (*appproviderv1beta1pb.OpenResponse, error) {
	provider, err := s.findAppProvider(ctx, req.ResourceInfo)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling findAppProvider")
		var st *rpcpb.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "app provider not found")
		} else {
			st = status.NewInternal(ctx, err, "error searching for app provider")
		}

		return &appproviderv1beta1pb.OpenResponse{
			Status: st,
		}, nil
	}

	c, err := pool.GetAppProviderClient(provider.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAppProviderClient")
		return &appproviderv1beta1pb.OpenResponse{
			Status: status.NewInternal(ctx, err, "error getting appprovider client"),
		}, nil
	}

	res, err := c.Open(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling c.Open")
	}

	return res, nil
}

func (s *svc) findAppProvider(ctx context.Context, ri *storageproviderv1beta1pb.ResourceInfo) (*appregistryv1beta1pb.ProviderInfo, error) {
	c, err := pool.GetAppRegistryClient(s.c.AppRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting appregistry client")
		return nil, err
	}

	res, err := c.GetAppProviders(ctx, &appregistryv1beta1pb.GetAppProvidersRequest{
		ResourceInfo: ri,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAppProviders")
		return nil, err
	}

	// TODO(labkode): when sending an Open to the proxy we need to choose one
	// provider from the list of available as the client
	if res.Status.Code == rpcpb.Code_CODE_OK {
		return res.Providers[0], nil
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gateway: app provider not found for resource: " + ri.String())
	}

	return nil, errors.New("gateway: error finding a storage provider")
}
