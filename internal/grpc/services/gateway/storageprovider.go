// Copyright 2018-2021 CERN
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
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	sdk "github.com/cs3org/reva/pkg/sdk/common"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"

	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

/*  About caching
    The gateway is doing a lot of requests to look up the responsible storage providers for a reference.
    - when the reference uses an id we can use a global id -> provider cache because it is the same for all users
    - when the reference is an absolute path we
   	 - 1. look up the corresponding space in the space registry
     - 2. can reuse the global id -> provider cache to look up the provider
	 - paths are unique per user: when a rule mounts shares at /shares/{{.Space.Name}}
	   the path /shares/Documents might show different content for einstein than for marie
	   -> path -> spaceid lookup needs a per user cache
	When can we invalidate?
	- the global cache needs to be invalidated when the provider for a space id changes.
		- happens when a space is moved from one provider to another. Not yet implemented
		-> should be good enough to use a TTL. daily should be good enough
	- the user individual file cache is actually a cache of the mount points
	    - we could do a registry.ListProviders (for user) on startup to warm up the cache ...
		- when a share is granted or removed we need to invalidate that path
		- when a share is renamed we need to invalidate the path
		- we can use a ttl for all paths?
		- the findProviders func in the gateway needs to look up in the user cache first
	We want to cache the root etag of spaces
	    - can be invalidated on every write or delete with fallback via TTL?
*/

// transferClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target string `json:"target"`
}

func (s *svc) sign(_ context.Context, target string) (string, error) {
	// Tus sends a separate request to the datagateway service for every chunk.
	// For large files, this can take a long time, so we extend the expiration
	ttl := time.Duration(s.c.TransferExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Audience:  "reva",
			IssuedAt:  time.Now().Unix(),
		},
		Target: target,
	}

	t := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), claims)

	tkn, err := t.SignedString([]byte(s.c.TransferSharedSecret))
	if err != nil {
		return "", errors.Wrapf(err, "error signing token with claims %+v", claims)
	}

	return tkn, nil
}

func (s *svc) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	u := ctxpkg.ContextMustGetUser(ctx)
	createReq := &provider.CreateStorageSpaceRequest{
		Type:  "personal",
		Owner: u,
		Name:  u.DisplayName,
	}

	// send the user id as the space id, makes debugging easier
	if u.Id != nil && u.Id.OpaqueId != "" {
		createReq.Opaque = &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				"space_id": {
					Decoder: "plain",
					Value:   []byte(u.Id.OpaqueId),
				},
			},
		}
	}
	res, err := s.CreateStorageSpace(ctx, createReq)
	if err != nil {
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, "error calling CreateHome"),
		}, nil
	}
	if res.Status.Code != rpc.Code_CODE_OK && res.Status.Code != rpc.Code_CODE_ALREADY_EXISTS {
		return &provider.CreateHomeResponse{
			Status: res.Status,
		}, nil
	}

	return &provider.CreateHomeResponse{
		Opaque: res.Opaque,
		Status: res.Status,
	}, nil
}

func (s *svc) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)

	// TODO change the CreateStorageSpaceRequest to contain a space instead of sending individual properties
	space := &provider.StorageSpace{
		Owner:     req.Owner,
		SpaceType: req.Type,
		Name:      req.Name,
		Quota:     req.Quota,
	}

	if req.Opaque != nil && req.Opaque.Map != nil && req.Opaque.Map["id"] != nil {
		if req.Opaque.Map["space_id"].Decoder == "plain" {
			space.Id = &provider.StorageSpaceId{OpaqueId: string(req.Opaque.Map["id"].Value)}
		}
	}

	srClient, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	spaceJSON, err := json.Marshal(space)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: marshaling space failed")
	}

	// The registry is responsible for choosing the right provider
	res, err := srClient.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
		Opaque: &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				"space": {
					Decoder: "json",
					Value:   spaceJSON,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return &provider.CreateStorageSpaceResponse{
			Status: res.Status,
		}, nil
	}

	if len(res.Providers) == 0 {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewNotFound(ctx, fmt.Sprintf("error finding provider for space %+v", space)),
		}, nil
	}

	// just pick the first provider, we expect only one
	c, err := s.getStorageProviderClient(ctx, res.Providers[0])
	if err != nil {
		return nil, err
	}
	createRes, err := c.CreateStorageSpace(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating storage space on storage provider")
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewInternal(ctx, "error calling CreateStorageSpace"),
		}, nil
	}

	return createRes, nil
}

func (s *svc) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	log := appctx.GetLogger(ctx)

	// TODO update CS3 api to forward the filters to the registry so it can filter the number of providers the gateway needs to query
	filters := map[string]string{}

	for _, f := range req.Filters {
		switch f.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			filters["storage_id"], filters["opaque_id"] = utils.SplitStorageSpaceID(f.GetId().OpaqueId)
		case provider.ListStorageSpacesRequest_Filter_TYPE_OWNER:
			filters["owner_idp"] = f.GetOwner().Idp
			filters["owner_id"] = f.GetOwner().OpaqueId
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			filters["space_type"] = f.GetSpaceType()
		default:
			return &provider.ListStorageSpacesResponse{
				Status: status.NewInvalidArg(ctx, fmt.Sprintf("unknown filter %v", f.Type)),
			}, nil
		}
	}

	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	listReq := &registry.ListStorageProvidersRequest{}
	if len(filters) > 0 {
		listReq.Opaque = &typesv1beta1.Opaque{}
		sdk.EncodeOpaqueMap(listReq.Opaque, filters)
	}
	res, err := c.ListStorageProviders(ctx, listReq)
	if err != nil {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewStatusFromErrType(ctx, "ListStorageSpaces filters: req "+req.String(), err),
		}, nil
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListStorageSpacesResponse{
			Status: res.Status,
		}, nil
	}

	// TODO the providers now have an opaque "spaces_paths" property
	providerInfos := res.Providers

	spacesFromProviders := make([][]*provider.StorageSpace, len(providerInfos))
	errors := make([]error, len(providerInfos))

	var wg sync.WaitGroup
	for i, p := range providerInfos {
		// we need to ask the provider for the space details
		wg.Add(1)
		go s.listStorageSpacesOnProvider(ctx, req, &spacesFromProviders[i], p, &errors[i], &wg)
	}
	wg.Wait()

	uniqueSpaces := map[string]*provider.StorageSpace{}
	for i := range providerInfos {
		if errors[i] != nil {
			if len(providerInfos) > 1 {
				log.Debug().Err(errors[i]).Msg("skipping provider")
				continue
			}
			return &provider.ListStorageSpacesResponse{
				Status: status.NewStatusFromErrType(ctx, "error listing space", errors[i]),
			}, nil
		}
		for j := range spacesFromProviders[i] {
			uniqueSpaces[spacesFromProviders[i][j].Id.OpaqueId] = spacesFromProviders[i][j]
		}
	}
	spaces := make([]*provider.StorageSpace, 0, len(uniqueSpaces))
	for spaceID := range uniqueSpaces {
		spaces = append(spaces, uniqueSpaces[spaceID])
	}
	if len(spaces) == 0 {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewNotFound(ctx, "space not found"),
		}, nil
	}

	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
}

func (s *svc) listStorageSpacesOnProvider(ctx context.Context, req *provider.ListStorageSpacesRequest, res *[]*provider.StorageSpace, p *registry.ProviderInfo, e *error, wg *sync.WaitGroup) {
	defer wg.Done()
	c, err := s.getStorageProviderClient(ctx, p)
	if err != nil {
		*e = errors.Wrap(err, "error connecting to storage provider="+p.Address)
		return
	}

	r, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		*e = errors.Wrap(err, "gateway: error calling ListStorageSpaces")
		return
	}

	*res = r.StorageSpaces
}

func (s *svc) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO: needs to be fixed
	c, _, err := s.find(ctx, &provider.Reference{ResourceId: req.StorageSpace.Root})
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "error finding ID", err),
		}, nil
	}

	res, err := c.UpdateStorageSpace(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating update space on storage provider")
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewInternal(ctx, "error calling UpdateStorageSpace"),
		}, nil
	}
	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), res.StorageSpace.Root)
	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO: needs to be fixed
	storageid, opaqeid := utils.SplitStorageSpaceID(req.Id.OpaqueId)
	c, _, err := s.find(ctx, &provider.Reference{ResourceId: &provider.ResourceId{
		StorageId: storageid,
		OpaqueId:  opaqeid,
	}})
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "error finding path", err),
		}, nil
	}

	res, err := c.DeleteStorageSpace(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error deleting storage space on storage provider")
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInternal(ctx, "error calling DeleteStorageSpace"),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), &provider.ResourceId{OpaqueId: req.Id.OpaqueId})
	return res, nil
}

func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)

	srClient, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	spaceJSON, err := json.Marshal(&provider.StorageSpace{
		Owner:     currentUser,
		SpaceType: "personal",
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: marshaling space failed")
	}

	// The registry is responsible for choosing the right provider
	// TODO fix naming GetStorageProviders calls the GetProvider functon on the registry implementation
	res, err := srClient.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
		Opaque: &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				"space": {
					Decoder: "json",
					Value:   spaceJSON,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return &provider.GetHomeResponse{
			Status: res.Status,
		}, nil
	}

	if len(res.Providers) == 0 {
		return &provider.GetHomeResponse{
			Status: status.NewNotFound(ctx, fmt.Sprintf("error finding provider for home space of %+v", currentUser)),
		}, nil
	}

	spacePaths := decodeSpacePaths(res.Providers[0].Opaque)
	if len(spacePaths) == 0 {
		spacePaths[""] = res.Providers[0].ProviderPath
	}
	for _, spacePath := range spacePaths {
		return &provider.GetHomeResponse{
			Path:   spacePath,
			Status: status.NewOK(ctx),
		}, nil
	}

	return &provider.GetHomeResponse{
		Status: status.NewNotFound(ctx, fmt.Sprintf("error finding home path for provider %+v with spacePaths  %+v ", res.Providers[0], spacePaths)),
	}, nil
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	// TODO(ishank011): enable downloading references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, _, err = s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewStatusFromErrType(ctx, "error initiating download ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &gateway.InitiateFileDownloadResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileDownload")
	}

	protocols := make([]*gateway.FileDownloadProtocol, len(storageRes.Protocols))
	for p := range storageRes.Protocols {
		protocols[p] = &gateway.FileDownloadProtocol{
			Opaque:           storageRes.Protocols[p].Opaque,
			Protocol:         storageRes.Protocols[p].Protocol,
			DownloadEndpoint: storageRes.Protocols[p].DownloadEndpoint,
		}

		if !storageRes.Protocols[p].Expose {
			// sign the download location and pass it to the data gateway
			u, err := url.Parse(protocols[p].DownloadEndpoint)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, "wrong format for download endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, "error creating signature for download"),
				}, nil
			}

			protocols[p].DownloadEndpoint = s.c.DataGatewayEndpoint
			protocols[p].Token = token
		}
	}

	return &gateway.InitiateFileDownloadResponse{
		Opaque:    storageRes.Opaque,
		Status:    storageRes.Status,
		Protocols: protocols,
	}, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, err = s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewStatusFromErrType(ctx, "initiateFileUpload ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &gateway.InitiateFileUploadResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileUpload")
	}

	if storageRes.Status.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileUploadResponse{
			Status: storageRes.Status,
		}, nil
	}

	protocols := make([]*gateway.FileUploadProtocol, len(storageRes.Protocols))
	for p := range storageRes.Protocols {
		protocols[p] = &gateway.FileUploadProtocol{
			Opaque:             storageRes.Protocols[p].Opaque,
			Protocol:           storageRes.Protocols[p].Protocol,
			UploadEndpoint:     storageRes.Protocols[p].UploadEndpoint,
			AvailableChecksums: storageRes.Protocols[p].AvailableChecksums,
		}

		if !storageRes.Protocols[p].Expose {
			// sign the upload location and pass it to the data gateway
			u, err := url.Parse(protocols[p].UploadEndpoint)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, "wrong format for upload endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, "error creating signature for upload"),
				}, nil
			}

			protocols[p].UploadEndpoint = s.c.DataGatewayEndpoint
			protocols[p].Token = token
		}
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return &gateway.InitiateFileUploadResponse{
		Opaque:    storageRes.Opaque,
		Status:    storageRes.Status,
		Protocols: protocols,
	}, nil
}

func (s *svc) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	statReq := &provider.StatRequest{Ref: &provider.Reference{ResourceId: req.ResourceId}}
	statRes, err := s.Stat(ctx, statReq)
	if err != nil {
		err = errors.Wrap(err, "gateway: error stating ref:"+statReq.Ref.String())
		return nil, err
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.GetPathResponse{
			Status: statRes.Status,
		}, nil
	}

	return &provider.GetPathResponse{
		Status: statRes.Status,
		Path:   statRes.GetInfo().GetPath(),
	}, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, err = s.find(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "createContainer ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.TouchFileResponse{
			Status: status.NewStatusFromErrType(ctx, "TouchFile ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.TouchFile(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.TouchFileResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling TouchFile")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	// TODO(ishank011): enable deleting references spread across storage providers, eg. /eos
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewStatusFromErrType(ctx, "delete ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.DeleteResponse{
				Status: status.NewPermissionDenied(ctx, err, "permission denied"),
			}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	if req.Source.ResourceId == nil || req.Destination.ResourceId == nil {
		return &provider.MoveResponse{
			Status: status.NewInvalidArg(ctx, "need relative references to move"),
		}, nil
	}

	if req.Source.ResourceId.StorageId != req.Destination.ResourceId.StorageId {
		return &provider.MoveResponse{
			Status: status.NewInvalidArg(ctx, "cross space move is not supported"),
		}, nil
	}

	c, _, err := s.find(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "Move ref="+req.Source.String(), err),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Source.ResourceId)
	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Destination.ResourceId)
	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.SetArbitraryMetadataResponse{
			Status: status.NewStatusFromErrType(ctx, "SetArbitraryMetadata ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.SetArbitraryMetadata(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.SetArbitraryMetadataResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling SetArbitraryMetadata")
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.UnsetArbitraryMetadataResponse{
			Status: status.NewStatusFromErrType(ctx, "UnsetArbitraryMetadata ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.UnsetArbitraryMetadata(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.UnsetArbitraryMetadataResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling UnsetArbitraryMetadata")
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

// Stat returns the Resoure info for a given resource by forwarding the request to the responsible provider.
// It expects a relative Reference pointing to a unique resource. All other calls will fail.
func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewStatusFromErrType(ctx, "could not find provider", err),
		}, nil
	}

	return c.Stat(ctx, &provider.StatRequest{
		Opaque:                req.Opaque,
		Ref:                   req.Ref,
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errtypes.NotSupported("Unimplemented")
}

// ListContainer lists the Resoure infos for a given resource by forwarding the request to the responsible provider.
// It expects a relative Reference pointing to a unique resource. All other calls will fail
func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		// we have no provider -> not found
		return &provider.ListContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "could not find provider", err),
		}, nil
	}

	return c.ListContainer(ctx, &provider.ListContainerRequest{
		Opaque:                req.Opaque,
		Ref:                   req.Ref,
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *svc) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return &provider.CreateSymlinkResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, err = s.find(ctx, req.Ref)
	if err != nil {
		return &provider.ListFileVersionsResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	return c.ListFileVersions(ctx, req)
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, err = s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreFileVersion ref="+req.Ref.String(), err),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return c.RestoreFileVersion(ctx, req)
}

func (s *svc) ListRecycleStream(_ *provider.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errtypes.NotSupported("ListRecycleStream unimplemented")
}

// ListRecycle lists the recycle bin of a specific space. It expects a relative reference. Other calls fail
func (s *svc) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "cannot find provider for ref", err),
		}, nil
	}

	return c.ListRecycle(ctx, req)
}

// RestoreRecycleItem restores a recycle item from the trash.
// req.Source must be set and a relative references pointing to a unique resource
// Addtionally source and dest StorageId must be the same as cross space restoration is not supported.
func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	if req.Ref.ResourceId == nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewInvalidArg(ctx, "gateway: need resourceid to restore"),
		}, nil

	}

	if req.RestoreRef != nil {
		if req.RestoreRef.ResourceId == nil {
			return &provider.RestoreRecycleItemResponse{
				Status: status.NewInvalidArg(ctx, "gateway: destref needs resourceid if given"),
			}, nil

		}
		if req.Ref.ResourceId.StorageId != req.RestoreRef.ResourceId.StorageId {
			return &provider.RestoreRecycleItemResponse{
				Status: status.NewInvalidArg(ctx, "gateway: cross-storage restores are forbidden"),
			}, nil
		}
	}

	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem source ref="+req.Ref.String(), err),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	if req.RestoreRef != nil {
		RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.RestoreRef.ResourceId)
	}
	return c.RestoreRecycleItem(ctx, req)
}

// PurgeRecycle purges an item from the recycle bin. It expects a relative reference pointing to a valid resource
func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "PurgeRecycle ref="+req.Ref.String(), err),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.Ref,
		Key:    req.Key,
	})
}

// GetQuota gets the quota for a space. It expects to get a relative reference pointing to a valid resource
func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, "GetQuota ref="+req.Ref.String(), err),
		}, nil
	}

	return c.GetQuota(ctx, &provider.GetQuotaRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.Ref,
	})
}

// find looks up the provider that is responsible for the given request
// It will return a client that the caller can use to make the call, as well as the ProviderInfo. It:
// - contains the provider path, which is the mount point of the provider
// - may contain a list of storage spaces with their id and space path
func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, error) {
	p, err := s.findProviders(ctx, ref)
	if err != nil {
		return nil, nil, err
	}

	client, err := s.getStorageProviderClient(ctx, p[0])
	return client, p[0], err
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return Cached(c, s.statCache), nil
}

/*
func userKey(ctx context.Context) string {
	u := ctxpkg.ContextMustGetUser(ctx)
	sb := strings.Builder{}
	if u.Id != nil {
		sb.WriteString(u.Id.OpaqueId)
		sb.WriteString("@")
		sb.WriteString(u.Id.Idp)
	} else {
		// fall back to username
		sb.WriteString(u.Username)
	}
	return sb.String()
}
*/

func (s *svc) findProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
	switch {
	case ref == nil:
		return nil, errtypes.BadRequest("missing reference")
	case ref.ResourceId != nil: // can we use the provider cache?
		// only the StorageId is used to look up the provider. the opaqueid can only be a share and as such part of a storage
		if value, exists := s.providerCache.Get(ref.ResourceId.StorageId); exists == nil {
			if providers, ok := value.([]*registry.ProviderInfo); ok {
				return providers, nil
			}
		}
	case ref.Path != "": //  TODO implement a mount path cache in the registry?
	/*
		// path / mount point lookup from cache
		if value, exists := s.mountCache.Get(userKey(ctx)); exists == nil {
			if m, ok := value.(map[string][]*registry.ProviderInfo); ok {
				providers := make([]*registry.ProviderInfo, 0, len(m))
				deepestMountPath := ""
				for mountPath, providerInfos := range m {
					switch {
					case strings.HasPrefix(mountPath, ref.Path):
						// and add all providers below and exactly matching the path
						// requested /foo, mountPath /foo/sub
						providers = append(providers, providerInfos...)
					case strings.HasPrefix(ref.Path, mountPath) && len(mountPath) > len(deepestMountPath):
						// eg. three providers: /foo, /foo/sub, /foo/sub/bar
						// requested /foo/sub/mob
						deepestMountPath = mountPath
					}
				}
				if deepestMountPath != "" {
					providers = append(providers, m[deepestMountPath]...)
				}
				return providers, nil
			}
		}
	*/
	default:
		return nil, errtypes.BadRequest("invalid reference, at least path or id must be set")
	}

	// lookup
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	filters := map[string]string{
		"path": ref.Path,
	}
	if ref.ResourceId != nil {
		filters["storage_id"] = ref.ResourceId.StorageId
		filters["opaque_id"] = ref.ResourceId.OpaqueId
	}

	listReq := &registry.ListStorageProvidersRequest{
		Opaque: &typesv1beta1.Opaque{},
	}
	sdk.EncodeOpaqueMap(listReq.Opaque, filters)
	res, err := c.ListStorageProviders(ctx, listReq)

	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListStorageProviders")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			// TODO use tombstone cache item?
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + ref.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported("gateway: " + res.Status.Message + " for " + ref.String() + " with code " + res.Status.Code.String())
		default:
			return nil, status.NewErrorFromCode(res.Status.Code, "gateway")
		}
	}

	if res.Providers == nil {
		return nil, errtypes.NotFound("gateway: provider is nil")
	}

	if ref.ResourceId != nil {
		if err = s.providerCache.Set(ref.ResourceId.StorageId, res.Providers); err != nil {
			appctx.GetLogger(ctx).Warn().Err(err).Interface("reference", ref).Msg("gateway: could not cache providers")
		}
	} /* else {
		// every user has a cache for mount points?
		// the path map must be cached in the registry, not in the gateway?
		//   - in the registry we cannot determine if other spaces have been mounted or removed. if a new project space was mounted that happens in the registry
		//   - but the registry does not know when we rename a space ... or does it?
		//     - /.../Shares is a collection the gateway builds by aggregating the liststoragespaces response
		//     - the spaces registry builds a path for every space, treating every share as a distinct space.
		//       - findProviders() will return a long list of spaces, the Stat / ListContainer calls will stat the root etags of every space and share
		//       -> FIXME cache the root etag of every space, ttl ... do we need to stat? or can we cach the root etag in the providerinfo?
		//     - large amounts of shares
		// use the root etag of a space to determine if we can read from cache?
		// (finished) uploads, created dirs, renamed nodes, deleted nodes cause the root etag of a space to change
		//
		var providersCache *ttlcache.Cache
		cache, err := s.mountCache.Get(userKey(ctx))
		if err != nil {
			providersCache = ttlcache.NewCache()
			_ = providersCache.SetTTL(time.Duration(s.c.MountCacheTTL) * time.Second)
			providersCache.SkipTTLExtensionOnHit(true)
			s.mountCache.Set(userKey(ctx), providersCache)
		} else {
			providersCache = cache.(*ttlcache.Cache)
		}

		for _, providerInfo := range res.Providers {

			mountPath := providerInfo.ProviderPath
			var root *provider.ResourceId

			if spacePaths := decodeSpacePaths(p.Opaque); len(spacePaths) > 0 {
				for spaceID, spacePath := range spacePaths {
					mountPath = spacePath
					rootSpace, rootNode := utils.SplitStorageSpaceID(spaceID)
					root = &provider.ResourceId{
						StorageId: rootSpace,
						OpaqueId:  rootNode,
					}
					break // TODO can there be more than one space for a path?
				}
			}
			providersCache.Set(userKey(ctx), res.Providers) // FIXME needs a map[string]*registry.ProviderInfo

		}
		// use ListProviders? make it return all providers a user has access to aka all mount points?
		// cache that list in the gateway.
		// -> invalidate the cached list of mountpoints when a modification happens
		// refres by loading all mountpoints from spaces registry
		// - in the registry cache listStorageSpaces responses for every provider so we don't have to query every provider?
		//   - how can we determine which listStorageSpaces response to invalidate?
		//     - misuse ListContainerStream to get notified of root changes of every space?
		//     - or send a ListStorageSpaces request to the registry with an invalidate(spaceid) property?
		//       - This would allow the gateway could tell the registry which space(s) to refresh
		//         - but the registry might not be using a cache
		//     - we still don't know when an upload finishes ... so we cannot invalidate the cache for that event
		//       - especially if there are workflows involved?
		//       - actually, the initiate upload response should make the provider show the file immediately. it should not be downloadable though
		//         - with stat we want to see the progress. actually multiple uploads (-> workflows) to the same file might be in progress...
		// example:
		//  - user accepts a share in the web ui, then navigates into his /Shares folder
		//    -> he should see the accepted share, and he should be able to navigate into it
		// - actually creating a share should already create a space, but it has no name yet
		// - the problem arises when someone mounts a spaece (can pe a share or a project, does not matter)
		//    -> when do we update the list of mount points which we cache in the gateway?
		// - we want to maintain a list of all mount points (and their root etag/mtime) to allow clients to efficiently poll /
		//   and query the list of all storage spaces the user has access to
		//   - the simplest 'maintenance' is caching the complete list and invalidating it on changes
		//   - a more elegant 'maintenance' would add and remove paths as they occur ... which is what the spaces registry is supposed to do...
		//     -> don't cache anything in the gateway for path based requests. Instead maintain a cache in the spaces registry.
		//
		// Caching needs to take the last modification time into account to discover new mount points -> needs to happen in the registry
	}*/

	return res.Providers, nil
}

func decodeSpacePaths(o *typesv1beta1.Opaque) map[string]string {
	spacePaths := map[string]string{}
	if o == nil {
		return spacePaths
	}
	if entry, ok := o.Map["space_paths"]; ok {
		_ = json.Unmarshal(entry.Value, &spacePaths)
		// TODO log
	}
	return spacePaths
}
