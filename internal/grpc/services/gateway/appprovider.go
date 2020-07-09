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

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) OpenFileInAppProvider(ctx context.Context, req *providerpb.OpenFileInAppProviderRequest) (*providerpb.OpenFileInAppProviderResponse, error) {

	log := appctx.GetLogger(ctx)

	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &providerpb.OpenFileInAppProviderResponse{
				Status: status.NewInternal(ctx, err, "storage provider not found"),
			}, nil
		}
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	statReq := &provider.StatRequest{
		Ref: req.Ref,
	}

	statRes, err := c.Stat(ctx, statReq)

	if err != nil {
		log.Err(err).Msg("gateway: error calling Stat for the share resource path:" + req.Ref.GetPath())
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Stat for the share resource id"),
		}, nil
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(statRes.Status.GetCode(), "gateway")
		log.Err(err).Msg("gateway: error calling Stat for the share resource id:" + req.Ref.GetPath())
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	fileInfo := statRes.Info

	provider, err := s.findAppProvider(ctx, fileInfo)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling findAppProvider")
		var st *rpc.Status
		if _, ok := err.(errtypes.IsNotFound); ok {
			st = status.NewNotFound(ctx, "app provider not found")
		} else {
			st = status.NewInternal(ctx, err, "error searching for app provider")
		}
		return &providerpb.OpenFileInAppProviderResponse{
			Status: st,
		}, nil
	}

	appProviderClient, err := pool.GetAppProviderClient(provider.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAppProviderClient")
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error getting appprovider client"),
		}, nil
	}

	res, err := appProviderClient.OpenFileInAppProvider(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling c.Open")
	}

	return res, nil
}

func (s *svc) findAppProvider(ctx context.Context, ri *storageprovider.ResourceInfo) (*registry.ProviderInfo, error) {
	c, err := pool.GetAppRegistryClient(s.c.AppRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting appregistry client")
		return nil, err
	}
	res, err := c.GetAppProviders(ctx, &registry.GetAppProvidersRequest{
		ResourceInfo: ri,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAppProviders")
		return nil, err
	}

	// TODO(labkode): when sending an Open to the proxy we need to choose one
	// provider from the list of available as the client
	if res.Status.Code == rpc.Code_CODE_OK {
		return res.Providers[0], nil
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gateway: app provider not found for resource: " + ri.String())
	}

	return nil, errors.New("gateway: error finding a storage provider")
}
