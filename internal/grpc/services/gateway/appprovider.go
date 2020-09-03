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
	"fmt"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/pkg/errors"
)

func (s *svc) OpenFileInAppProvider(ctx context.Context, req *gateway.OpenFileInAppProviderRequest) (*providerpb.OpenFileInAppProviderResponse, error) {

	accessToken, ok := tokenpkg.ContextGetToken(ctx)
	if !ok || accessToken == "" {
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewUnauthenticated(ctx, errors.New("Access token is invalid or empty"), ""),
		}, nil
	}

	statReq := &provider.StatRequest{
		Ref: req.Ref,
	}

	statRes, err := s.Stat(ctx, statReq)
	if err != nil {
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Stat on the resource path for the app provider: "+req.Ref.GetPath()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(statRes.Status.GetCode(), "gateway")
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "Stat failed on the resource path for the app provider: "+req.Ref.GetPath()),
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

	// build the appProvider specific request with the required extra info that has been obtained

	log := appctx.GetLogger(ctx)
	log.Debug().Msg(fmt.Sprintf("request: %s", req))

	appProviderReq := &providerpb.OpenFileInAppProviderRequest{
		ResourceInfo: fileInfo,
		ViewMode:     providerpb.OpenFileInAppProviderRequest_ViewMode(req.ViewMode),
		AccessToken:  accessToken,
	}

	res, err := appProviderClient.OpenFileInAppProvider(ctx, appProviderReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling OpenFileInAppProvider")
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
