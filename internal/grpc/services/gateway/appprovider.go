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
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func (s *svc) OpenInApp(ctx context.Context, req *gateway.OpenInAppRequest) (*providerpb.OpenInAppResponse, error) {
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		if st.Code == rpc.Code_CODE_NOT_FOUND {
			return &providerpb.OpenInAppResponse{
				Status: status.NewNotFound(ctx, "gateway: resource not found:"+req.Ref.String()),
			}, nil
		}
		return &providerpb.OpenInAppResponse{
			Status: st,
		}, nil
	}

	if s.isSharedFolder(ctx, p) {
		return &providerpb.OpenInAppResponse{
			Status: status.NewInvalid(ctx, "gateway: can't open shared folder"),
		}, nil
	}

	resName, resChild := p, ""
	if s.isShareChild(ctx, p) {
		resName, resChild = s.splitShare(ctx, p)
	}

	statRes, err := s.Stat(ctx, &storageprovider.StatRequest{
		Ref: &storageprovider.Reference{Path: resName},
	})
	if err != nil {
		return &providerpb.OpenInAppResponse{
			Status: status.NewInternal(ctx, err, "gateway: error calling Stat on the resource path for the app provider: "+req.Ref.GetPath()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(statRes.Status.GetCode(), "gateway")
		return &providerpb.OpenInAppResponse{
			Status: status.NewInternal(ctx, err, "Stat failed on the resource path for the app provider: "+req.Ref.GetPath()),
		}, nil
	}

	fileInfo := statRes.Info

	// The file is a share
	if fileInfo.Type == storageprovider.ResourceType_RESOURCE_TYPE_REFERENCE {
		uri, err := url.Parse(fileInfo.Target)
		if err != nil {
			return &providerpb.OpenInAppResponse{
				Status: status.NewInternal(ctx, err, "gateway: error parsing target uri: "+fileInfo.Target),
			}, nil
		}
		if uri.Scheme == "webdav" {
			insecure, skipVerify := getGRPCConfig(req.Opaque)
			return s.openFederatedShares(ctx, fileInfo.Target, req, insecure, skipVerify, resChild)
		}

		res, err := s.Stat(ctx, &storageprovider.StatRequest{
			Ref: req.Ref,
		})
		if err != nil {
			return &providerpb.OpenInAppResponse{
				Status: status.NewInternal(ctx, err, "gateway: error calling Stat on the resource path for the app provider: "+req.Ref.GetPath()),
			}, nil
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			err := status.NewErrorFromCode(res.Status.GetCode(), "gateway")
			return &providerpb.OpenInAppResponse{
				Status: status.NewInternal(ctx, err, "Stat failed on the resource path for the app provider: "+req.Ref.GetPath()),
			}, nil
		}
		fileInfo = res.Info
	}
	return s.openLocalResources(ctx, fileInfo, req)
}

func (s *svc) openFederatedShares(ctx context.Context, targetURL string, req *gateway.OpenInAppRequest,
	insecure, skipVerify bool, nameQueries ...string) (*providerpb.OpenInAppResponse, error) {
	log := appctx.GetLogger(ctx)
	targetURL, err := appendNameQuery(targetURL, nameQueries...)
	if err != nil {
		return nil, err
	}
	ep, err := s.extractEndpointInfo(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	ref := &storageprovider.Reference{Path: ep.filePath}
	appProviderReq := &gateway.OpenInAppRequest{
		Ref:      ref,
		ViewMode: req.ViewMode,
		App:      req.App,
		Opaque:   req.Opaque,
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
	log.Debug().Msgf("Forwarding OpenInApp request to: %s", gatewayEP)

	conn, err := getConn(gatewayEP, insecure, skipVerify)
	if err != nil {
		log.Err(err).Msg("error connecting to remote reva")
		return nil, errors.Wrap(err, "gateway: error connecting to remote reva")
	}

	gatewayClient := gateway.NewGatewayAPIClient(conn)
	remoteCtx := appctx.ContextSetToken(context.Background(), ep.token)
	remoteCtx = metadata.AppendToOutgoingContext(remoteCtx, appctx.TokenHeader, ep.token)

	res, err := gatewayClient.OpenInApp(remoteCtx, appProviderReq)
	if err != nil {
		log.Err(err).Msg("error returned by remote OpenInApp call")
		return nil, errors.Wrap(err, "gateway: error calling OpenInApp")
	}
	return res, nil
}

func (s *svc) openLocalResources(ctx context.Context, ri *storageprovider.ResourceInfo, req *gateway.OpenInAppRequest) (*providerpb.OpenInAppResponse, error) {
	accessToken, ok := appctx.ContextGetToken(ctx)
	if !ok || accessToken == "" {
		return &providerpb.OpenInAppResponse{
			Status: status.NewUnauthenticated(ctx, errtypes.InvalidCredentials("Access token is invalid or empty"), ""),
		}, nil
	}

	provider, err := s.findAppProvider(ctx, ri, req.App)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling findAppProvider")
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &providerpb.OpenInAppResponse{
				Status: status.NewNotFound(ctx, "Could not find the requested app provider"),
			}, nil
		}
		return nil, err
	}

	appProviderClient, err := pool.GetAppProviderClient(pool.Endpoint(provider.Address))
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetAppProviderClient")
	}

	appProviderReq := &providerpb.OpenInAppRequest{
		ResourceInfo: ri,
		ViewMode:     providerpb.ViewMode(req.ViewMode),
		AccessToken:  accessToken,
		Opaque:       req.Opaque,
	}

	res, err := appProviderClient.OpenInApp(ctx, appProviderReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling OpenInApp")
	}

	return res, nil
}

func (s *svc) findAppProvider(ctx context.Context, ri *storageprovider.ResourceInfo, app string) (*registry.ProviderInfo, error) {
	c, err := pool.GetAppRegistryClient(pool.Endpoint(s.c.AppRegistryEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting appregistry client")
		return nil, err
	}

	// when app is empty it means the user assumes a default behaviour.
	// From a web perspective, means the user click on the file itself.
	// Normally the file will get downloaded but if a suitable application exists
	// the behaviour will change from download to open the file with the app.
	if app == "" {
		// If app is empty means that we need to rely on "default" behaviour.
		// We currently do not have user preferences implemented so the only default
		// we can currently enforce is one configured by the system admins, the
		// "system default".
		// If a default is not set we raise an error rather that giving the user the first provider in the list
		// as the list is built on init time and is not deterministic, giving the user different results on service
		// reload.
		res, err := c.GetDefaultAppProviderForMimeType(ctx, &registry.GetDefaultAppProviderForMimeTypeRequest{
			MimeType: ri.MimeType,
		})
		if err != nil {
			err = errors.Wrap(err, "gateway: error calling GetDefaultAppProviderForMimeType")
			return nil, err
		}

		// we've found a provider
		if res.Status.Code == rpc.Code_CODE_OK && res.Provider != nil {
			return res.Provider, nil
		}

		// we did not find a default provider
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			err := errtypes.NotFound(fmt.Sprintf("gateway: default app provider for mime type:%s not found", ri.MimeType))
			return nil, err
		}

		// response code is something else, bubble up error
		// if a default is not set we abort as returning the first application available is not
		// deterministic for the end-user as it depends on initialization order of the app approviders with the registry.
		// It also provides a good hint to the system admin to configure the defaults accordingly.
		err = errtypes.InternalError(fmt.Sprintf("gateway: unexpected grpc response status:%s when calling GetDefaultAppProviderForMimeType", res.Status))
		return nil, err
	}

	// app has been forced and is set, we try to get an app provider that can satisfy it
	// Note that we ask for the list of all available providers for a given resource
	// even though we're only interested into the one set by the "app" parameter.
	// A better call will be to issue a (to be added) GetAppProviderByName(app) method
	// to just get what we ask for.
	res, err := c.GetAppProviders(ctx, &registry.GetAppProvidersRequest{
		ResourceInfo: ri,
	})
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAppProviders")
		return nil, err
	}

	// if the list of app providers is empty means we expect a CODE_NOT_FOUND in the response
	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound("gateway: app provider not found for resource: " + ri.String())
		}
		return nil, errtypes.InternalError("gateway: error finding app providers")
	}

	// as long as the above mentioned GetAppProviderByName(app) method is not available
	// we need to apply a manual filter
	filteredProviders := []*registry.ProviderInfo{}
	for _, p := range res.Providers {
		if p.Name == app {
			filteredProviders = append(filteredProviders, p)
		}
	}
	res.Providers = filteredProviders

	if len(res.Providers) == 0 {
		return nil, errtypes.NotFound(fmt.Sprintf("app '%s' not found", app))
	}

	if len(res.Providers) == 1 {
		return res.Providers[0], nil
	}

	// we should never arrive to the point of having more than one
	// provider for the given "app" parameters sent by the user
	return nil, errtypes.InternalError(fmt.Sprintf("gateway: user requested app %q and we provided %d applications", app, len(res.Providers)))
}

func getGRPCConfig(opaque *typespb.Opaque) (bool, bool) {
	if opaque == nil {
		return false, false
	}
	_, insecure := opaque.Map["insecure"]
	_, skipVerify := opaque.Map["skip-verify"]
	return insecure, skipVerify
}

func getConn(host string, ins, skipverify bool) (*grpc.ClientConn, error) {
	if ins {
		return grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// TODO(labkode): if in the future we want client-side certificate validation,
	// we need to load the client cert here
	tlsconf := &tls.Config{InsecureSkipVerify: skipverify}
	creds := credentials.NewTLS(tlsconf)
	return grpc.NewClient(host, grpc.WithTransportCredentials(creds))
}
