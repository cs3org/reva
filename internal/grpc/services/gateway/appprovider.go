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
	"net/url"
	"strings"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/pkg/errors"
)

func (s *svc) OpenFileInAppProvider(ctx context.Context, req *gateway.OpenFileInAppProviderRequest) (*providerpb.OpenFileInAppProviderResponse, error) {
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &providerpb.OpenFileInAppProviderResponse{
				Status: status.NewNotFound(ctx, "gateway: file not found:"+req.Ref.String()),
			}, nil
		}
		return &providerpb.OpenFileInAppProviderResponse{
			Status: st,
		}, nil
	}

	if s.isSharedFolder(ctx, p) {
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInvalid(ctx, "gateway: can't open shares folder"),
		}, nil
	}

	resName, resChild := p, ""
	if s.isShareChild(ctx, p) {
		resName, resChild = s.splitShare(ctx, p)
	}

	statRes, err := s.stat(ctx, &storageprovider.StatRequest{
		Ref: &storageprovider.Reference{
			Spec: &storageprovider.Reference_Path{
				Path: resName,
			},
		},
	})
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

	// The file is a share
	if fileInfo.Type == storageprovider.ResourceType_RESOURCE_TYPE_REFERENCE {
		uri, err := url.Parse(fileInfo.Target)
		if err != nil {
			return &providerpb.OpenFileInAppProviderResponse{
				Status: status.NewInternal(ctx, err, "gateway: error parsing target uri: "+fileInfo.Target),
			}, nil
		}
		if uri.Scheme == "webdav" {
			return s.openFederatedShares(ctx, fileInfo.Target, req.ViewMode, resChild)
		}

		res, err := s.Stat(ctx, &storageprovider.StatRequest{
			Ref: req.Ref,
		})
		if err != nil {
			return &providerpb.OpenFileInAppProviderResponse{
				Status: status.NewInternal(ctx, err, "gateway: error calling Stat on the resource path for the app provider: "+req.Ref.GetPath()),
			}, nil
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			err := status.NewErrorFromCode(res.Status.GetCode(), "gateway")
			return &providerpb.OpenFileInAppProviderResponse{
				Status: status.NewInternal(ctx, err, "Stat failed on the resource path for the app provider: "+req.Ref.GetPath()),
			}, nil
		}
		fileInfo = res.Info
	}
	return s.openLocalResources(ctx, fileInfo, req.ViewMode)
}

func (s *svc) openFederatedShares(ctx context.Context, targetURL string, vm gateway.OpenFileInAppProviderRequest_ViewMode,
	nameQueries ...string) (*providerpb.OpenFileInAppProviderResponse, error) {
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return nil, err
	}
	ep, err := s.extractEndpointInfo(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	ref := &storageprovider.Reference{
		Spec: &storageprovider.Reference_Path{
			Path: ep.filePath,
		},
	}
	appProviderReq := &gateway.OpenFileInAppProviderRequest{
		Ref:      ref,
		ViewMode: vm,
	}

	meshProvider, err := s.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: ep.endpoint,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetInfoByDomain")
	}
	var gatewayEP string
	for _, s := range meshProvider.ProviderInfo.Services {
		if strings.ToLower(s.Endpoint.Type.Name) == "gateway" {
			gatewayEP = s.Endpoint.Path
		}
	}

	gatewayClient, err := pool.GetGatewayServiceClient(gatewayEP)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetGatewayClient")
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewInternal(ctx, err, "error getting gateway client"),
		}, nil
	}

	ctx = tokenpkg.ContextSetToken(ctx, ep.token)
	res, err := gatewayClient.OpenFileInAppProvider(ctx, appProviderReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling OpenFileInAppProvider")
	}
	return res, nil
}

func (s *svc) openLocalResources(ctx context.Context, ri *storageprovider.ResourceInfo,
	vm gateway.OpenFileInAppProviderRequest_ViewMode) (*providerpb.OpenFileInAppProviderResponse, error) {

	accessToken, ok := tokenpkg.ContextGetToken(ctx)
	if !ok || accessToken == "" {
		return &providerpb.OpenFileInAppProviderResponse{
			Status: status.NewUnauthenticated(ctx, errors.New("Access token is invalid or empty"), ""),
		}, nil
	}

	provider, err := s.findAppProvider(ctx, ri)
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

	appProviderReq := &providerpb.OpenFileInAppProviderRequest{
		ResourceInfo: ri,
		ViewMode:     providerpb.OpenFileInAppProviderRequest_ViewMode(vm),
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
