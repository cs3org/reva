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
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
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
	"github.com/cs3org/reva/pkg/rhttp/router"
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

	srClient, err := s.getStorageRegistryClient(ctx, s.c.StorageRegistryEndpoint)
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

	// TODO update CS3 api to forward the filters to the registry so it can filter the number of providers the gateway needs to query
	filters := map[string]string{}

	for _, f := range req.Filters {
		switch f.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			sid, oid, err := utils.SplitStorageSpaceID(f.GetId().OpaqueId)
			if err != nil {
				continue
			}
			filters["storage_id"], filters["opaque_id"] = sid, oid
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

	c, err := s.getStorageRegistryClient(ctx, s.c.StorageRegistryEndpoint)
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

	spaces := []*provider.StorageSpace{}
	for _, providerInfo := range res.Providers {
		spacePaths := decodeSpacePaths(providerInfo)
		for spaceID, spacePath := range spacePaths {
			storageid, opaqueid, err := utils.SplitStorageSpaceID(spaceID)
			if err != nil {
				// TODO: log? error?
				continue
			}
			spaces = append(spaces, &provider.StorageSpace{
				Id: &provider.StorageSpaceId{OpaqueId: spaceID},
				Opaque: &typesv1beta1.Opaque{
					Map: map[string]*typesv1beta1.OpaqueEntry{
						"path": {
							Decoder: "plain",
							Value:   []byte(spacePath),
						},
					},
				},
				Root: &provider.ResourceId{
					StorageId: storageid,
					OpaqueId:  opaqueid,
				},
			})
		}
	}

	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
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
	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), res.StorageSpace.Root)
	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO: needs to be fixed
	storageid, opaqeid, err := utils.SplitStorageSpaceID(req.Id.OpaqueId)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInvalidArg(ctx, "OpaqueID was empty"),
		}, nil
	}

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

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), &provider.ResourceId{OpaqueId: req.Id.OpaqueId})
	return res, nil
}

func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)

	srClient, err := s.getStorageRegistryClient(ctx, s.c.StorageRegistryEndpoint)
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

	// NOTE: this will cause confusion if len(spacePath) > 1
	spacePaths := decodeSpacePaths(res.Providers[0])
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
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
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
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "createContainer ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
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
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	var c provider.ProviderAPIClient
	var sourceProviderInfo, destinationProviderInfo *registry.ProviderInfo
	var err error

	rename := utils.IsAbsolutePathReference(req.Source) &&
		utils.IsAbsolutePathReference(req.Destination) &&
		filepath.Dir(req.Source.Path) == filepath.Dir(req.Destination.Path)

	c, sourceProviderInfo, req.Source, err = s.findAndUnwrap(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "Move ref="+req.Source.String(), err),
		}, nil
	}

	// do we try to rename the root of a mountpoint?
	// TODO how do we determine if the destination resides on the same storage space?
	if rename && req.Source.Path == "." {
		req.Destination.ResourceId = req.Source.ResourceId
		req.Destination.Path = utils.MakeRelativePath(filepath.Base(req.Destination.Path))
	} else {
		_, destinationProviderInfo, req.Destination, err = s.findAndUnwrap(ctx, req.Destination)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewStatusFromErrType(ctx, "Move ref="+req.Destination.String(), err),
			}, nil
		}

		// if the storage id is the same the storage provider decides if the move is allowedy or not
		if sourceProviderInfo.Address != destinationProviderInfo.Address {
			res := &provider.MoveResponse{
				Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage move not supported, use copy and delete"),
			}
			return res, nil
		}
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Source.ResourceId)
	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Destination.ResourceId)
	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

// Stat returns the Resoure info for a given resource by forwarding the request to all responsible providers.
// In the simplest case there is only one provider, eg. when statting a relative or id based reference
// However the registry can return multiple providers for a reference and Stat needs to take them all into account:
// The registry returns multiple providers when
// 1. embedded providers need to be taken into account, eg: there aro two providers /foo and /bar and / is being statted
// 2. multiple providers form a virtual view, eg: there are twe providers /users/[a-k] and /users/[l-z] and /users is being statted
// In contrast to ListContainer Stat can treat these cases equally by forwarding the request to all providers and aggregating the metadata:
// - The most recent mtime determines the etag
// - The size is summed up for all providers
// TODO cache info
func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {

	requestPath := req.Ref.Path
	// find the providers
	providerInfos, err := s.findSpaces(ctx, req.Ref)
	if err != nil {
		// we have no provider -> not found
		return &provider.StatResponse{
			Status: status.NewStatusFromErrType(ctx, "could not find provider", err),
		}, nil
	}

	var info *provider.ResourceInfo
	for i := range providerInfos {
		// get client for storage provider
		c, err := s.getStorageProviderClient(ctx, providerInfos[i])
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not get storage provider client, skipping")
			continue
		}

		for spaceID, mountPath := range decodeSpacePaths(providerInfos[i]) {
			var root *provider.ResourceId
			rootSpace, rootNode, _ := utils.SplitStorageSpaceID(spaceID)
			if rootSpace != "" && rootNode != "" {
				root = &provider.ResourceId{
					StorageId: rootSpace,
					OpaqueId:  rootNode,
				}
			}
			// build reference for the provider
			r := &provider.Reference{
				ResourceId: req.Ref.ResourceId,
				Path:       req.Ref.Path,
			}
			// NOTE: There are problems in the following case:
			// Given a req.Ref.Path = "/projects" and a mountpath = "/projects/projectA"
			// Then it will request path "/projects/projectA" from the provider
			// But it should only request "/" as the ResourceId already points to the correct resource
			// TODO: We need to cut the path in case the resourceId is already pointing to correct resource
			if r.Path != "" && strings.HasPrefix(mountPath, r.Path) { // requesting the root in that case - No Path needed
				r.Path = "/"
			}
			providerRef := unwrap(r, mountPath, root)

			// there are three cases:
			// 1. id based references -> send to provider as is. must return the path in the space. space root can be determined by the spaceid
			// 2. path based references -> replace mount point with space and forward relative reference
			// 3. relative reference -> forward as is

			var currentInfo *provider.ResourceInfo
			statResp, err := c.Stat(ctx, &provider.StatRequest{Opaque: req.Opaque, Ref: providerRef, ArbitraryMetadataKeys: req.ArbitraryMetadataKeys})
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not stat parent mount, skipping")
				continue
			}
			if statResp.Status.Code != rpc.Code_CODE_OK {
				appctx.GetLogger(ctx).Debug().Interface("status", statResp.Status).Msg("gateway: stating parent mount was not ok, skipping")
				continue
			}
			if statResp.Info == nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: stat response for parent mount carried no info, skipping")
				continue
			}

			if requestPath != "" && strings.HasPrefix(mountPath, requestPath) { // when path is used and requested path is above mount point

				// mount path might be the reuqest path for file based shares
				if mountPath != requestPath {
					// mountpoint is deeper than the statted path
					// -> make child a folder
					statResp.Info.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
					statResp.Info.MimeType = "httpd/unix-directory"
					// -> unset checksums for a folder
					statResp.Info.Checksum = nil
					if statResp.Info.Opaque != nil {
						delete(statResp.Info.Opaque.Map, "md5")
						delete(statResp.Info.Opaque.Map, "adler32")
					}
				}

				// -> update metadata for /foo/bar -> set path to './bar'?
				statResp.Info.Path = strings.TrimPrefix(mountPath, requestPath)
				statResp.Info.Path, _ = router.ShiftPath(statResp.Info.Path)
				statResp.Info.Path = utils.MakeRelativePath(statResp.Info.Path)
				// TODO invent resourceid?
				if utils.IsAbsoluteReference(req.Ref) {
					statResp.Info.Path = path.Join(requestPath, statResp.Info.Path)
				}
			}
			if statResp.Info.Id.StorageId == "" {
				statResp.Info.Id.StorageId = providerInfos[i].ProviderId
			}
			currentInfo = statResp.Info

			if info == nil {
				switch {
				case utils.IsAbsolutePathReference(req.Ref):
					currentInfo.Path = requestPath
				case utils.IsAbsoluteReference(req.Ref):
					// an id based reference needs to adjust the path in the response with the provider path
					currentInfo.Path = path.Join(mountPath, currentInfo.Path)
				}
				info = currentInfo
			} else {
				// aggregate metadata
				if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
					info.Size += currentInfo.Size
				}
				if info.Mtime == nil || (currentInfo.Mtime != nil && utils.TSToUnixNano(currentInfo.Mtime) > utils.TSToUnixNano(info.Mtime)) {
					info.Mtime = currentInfo.Mtime
					info.Etag = currentInfo.Etag
					// info.Checksum = resp.Info.Checksum
				}
				if info.Etag == "" && info.Etag != currentInfo.Etag {
					info.Etag = currentInfo.Etag
				}
			}
		}
	}

	if info == nil {
		return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
	}
	return &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Info: info}, nil
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errtypes.NotSupported("Unimplemented")
}

// ListContainer lists the Resoure infos for a given resource by forwarding the request to all responsible providers.
// In the simplest case there is only one provider, eg. when listing a relative or id based reference
// However the registry can return multiple providers for a reference and ListContainer needs to take them all into account:
// The registry returns multiple providers when
// 1. embedded providers need to be taken into account, eg: there aro two providers /foo and /bar and / is being listed
//    /foo and /bar need to be added to the listing of /
// 2. multiple providers form a virtual view, eg: there are twe providers /users/[a-k] and /users/[l-z] and /users is being listed
// In contrast to Stat ListContainer has to forward the request to all providers, collect the results and aggregate the metadata:
// - The most recent mtime determines the etag of the listed collection
// - The size of the root ... is summed up for all providers
// TODO cache info
func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {

	requestPath := req.Ref.Path
	// find the providers
	providerInfos, err := s.findSpaces(ctx, req.Ref)
	if err != nil {
		// we have no provider -> not found
		return &provider.ListContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "could not find provider", err),
		}, nil
	}
	// list /foo, mount points at /foo/bar, /foo/bif, /foo/bar/bam
	// 1. which provider needs to be listed
	// 2. which providers need to be statted
	// result:
	// + /foo/bif -> stat  /foo/bif
	// + /foo/bar -> stat  /foo/bar && /foo/bar/bif (and take the youngest metadata)

	// list /foo, mount points at /foo, /foo/bif, /foo/bar/bam
	// 1. which provider needs to be listed -> /foo listen
	// 2. which providers need to be statted
	// result:
	// + /foo/fil.txt   -> list /foo
	// + /foo/blarg.md  -> list /foo
	// + /foo/bif       -> stat  /foo/bif
	// + /foo/bar       -> stat  /foo/bar/bam (and construct metadata for /foo/bar)

	infos := map[string]*provider.ResourceInfo{}
	for i := range providerInfos {

		// get client for storage provider
		c, err := s.getStorageProviderClient(ctx, providerInfos[i])
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not get storage provider client, skipping")
			continue
		}

		for spaceID, mountPath := range decodeSpacePaths(providerInfos[i]) {
			var root *provider.ResourceId
			rootSpace, rootNode, _ := utils.SplitStorageSpaceID(spaceID)
			if rootSpace != "" && rootNode != "" {
				root = &provider.ResourceId{
					StorageId: rootSpace,
					OpaqueId:  rootNode,
				}
			}
			// build reference for the provider - copy to avoid side effects
			r := &provider.Reference{
				ResourceId: req.Ref.ResourceId,
				Path:       req.Ref.Path,
			}
			// NOTE: There are problems in the following case:
			// Given a req.Ref.Path = "/projects" and a mountpath = "/projects/projectA"
			// Then it will request path "/projects/projectA" from the provider
			// But it should only request "/" as the ResourceId already points to the correct resource
			// TODO: We need to cut the path in case the resourceId is already pointing to correct resource
			if r.Path != "" && strings.HasPrefix(mountPath, r.Path) { // requesting the root in that case - No Path accepted
				r.Path = "/"
			}
			providerRef := unwrap(r, mountPath, root)

			// ref Path: ., Id: a-b-c-d, provider path: /personal/a-b-c-d, provider id: a-b-c-d ->
			// ref Path: ., Id: a-b-c-d, provider path: /home, provider id: a-b-c-d ->
			// ref path: /foo/mop, provider path: /foo -> list(spaceid, ./mop)
			// ref path: /foo, provider path: /foo
			// if the requested path matches or is below a mount point we can list on that provider
			//           requested path   provider path
			// above   = /foo           <=> /foo/bar        -> stat(spaceid, .)    -> add metadata for /foo/bar
			// above   = /foo           <=> /foo/bar/bif    -> stat(spaceid, .)    -> add metadata for /foo/bar
			// matches = /foo/bar       <=> /foo/bar        -> list(spaceid, .)
			// below   = /foo/bar/bif   <=> /foo/bar        -> list(spaceid, ./bif)
			switch {
			case requestPath == "": // id based request
				fallthrough
			case strings.HasPrefix(requestPath, "."): // space request
				fallthrough
			case strings.HasPrefix(requestPath, mountPath): //  requested path is below mount point
				rsp, err := c.ListContainer(ctx, &provider.ListContainerRequest{
					Opaque:                req.Opaque,
					Ref:                   providerRef,
					ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
				})
				if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
					appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not list provider, skipping")
					continue
				}

				if utils.IsAbsoluteReference(req.Ref) {
					var prefix string
					if utils.IsAbsolutePathReference(providerRef) {
						prefix = mountPath
					} else {
						prefix = path.Join(mountPath, providerRef.Path)
					}
					for j := range rsp.Infos {

						rsp.Infos[j].Path = path.Join(prefix, rsp.Infos[j].Path)
					}
				}
				for i := range rsp.Infos {
					if info, ok := infos[rsp.Infos[i].Path]; ok {
						if info.Mtime != nil && rsp.Infos[i].Mtime != nil && utils.TSToUnixNano(rsp.Infos[i].Mtime) > utils.TSToUnixNano(info.Mtime) {
							continue
						}
					}
					// replace with younger info
					infos[rsp.Infos[i].Path] = rsp.Infos[i]
				}
			case strings.HasPrefix(mountPath, requestPath): // requested path is above mount point
				//  requested path     provider path
				//  /foo           <=> /foo/bar          -> stat(spaceid, .)    -> add metadata for /foo/bar
				//  /foo           <=> /foo/bar/bif      -> stat(spaceid, .)    -> add metadata for /foo/bar, overwrite type with dir
				statResp, err := c.Stat(ctx, &provider.StatRequest{
					Opaque:                req.Opaque,
					Ref:                   providerRef,
					ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
				})
				if err != nil {
					appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not stat parent mount for list, skipping")
					continue
				}
				if statResp.Status.Code != rpc.Code_CODE_OK {
					appctx.GetLogger(ctx).Debug().Interface("status", statResp.Status).Msg("gateway: stating parent mount for list was not ok, skipping")
					continue
				}
				if statResp.Info == nil {
					appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: stat response for list carried no info, skipping")
					continue
				}

				// is the mount point a direct child of the requested resurce? only works for absolute paths ... hmmm
				if filepath.Dir(mountPath) != requestPath {
					// mountpoint is deeper than one level
					// -> make child a folder
					statResp.Info.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
					statResp.Info.MimeType = "httpd/unix-directory"
					// -> unset checksums for a folder
					statResp.Info.Checksum = nil
					if statResp.Info.Opaque != nil {
						delete(statResp.Info.Opaque.Map, "md5")
						delete(statResp.Info.Opaque.Map, "adler32")
					}
				}

				// -> update metadata for /foo/bar -> set path to './bar'?
				statResp.Info.Path = strings.TrimPrefix(mountPath, requestPath)
				statResp.Info.Path, _ = router.ShiftPath(statResp.Info.Path)
				statResp.Info.Path = utils.MakeRelativePath(statResp.Info.Path)
				// TODO invent resourceid? or unset resourceid? derive from path?

				if utils.IsAbsoluteReference(req.Ref) {
					statResp.Info.Path = path.Join(requestPath, statResp.Info.Path)
				}

				if info, ok := infos[statResp.Info.Path]; !ok {
					// replace with younger info
					infos[statResp.Info.Path] = statResp.Info
				} else if info.Mtime == nil || (statResp.Info.Mtime != nil && utils.TSToUnixNano(statResp.Info.Mtime) > utils.TSToUnixNano(info.Mtime)) {
					// replace with younger info
					infos[statResp.Info.Path] = statResp.Info
				}
			default:
				log := appctx.GetLogger(ctx)
				log.Err(err).Msg("gateway: unhandled ListContainer case")
			}
		}
	}

	returnInfos := make([]*provider.ResourceInfo, 0, len(infos))
	for path := range infos {
		returnInfos = append(returnInfos, infos[path])
	}
	return &provider.ListContainerResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Infos:  returnInfos,
	}, nil
}

func (s *svc) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return &provider.CreateSymlinkResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListFileVersionsResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.ListFileVersions(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListFileVersions")
	}

	return res, nil
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreFileVersion ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreFileVersion")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) ListRecycleStream(_ *provider.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errtypes.NotSupported("ListRecycleStream unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trash of a specific storage
func (s *svc) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	requestPath := req.Ref.Path
	providerInfos, err := s.findSpaces(ctx, req.Ref)
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "ListRecycle ref="+req.Ref.String(), err),
		}, nil
	}
	for i := range providerInfos {

		// get client for storage provider
		c, err := s.getStorageProviderClient(ctx, providerInfos[i])
		if err != nil {
			return &provider.ListRecycleResponse{
				Status: status.NewInternal(ctx, "gateway: could not get storage provider client"),
			}, nil
		}

		var root *provider.ResourceId
		for spaceID, mountPath := range decodeSpacePaths(providerInfos[i]) {
			rootSpace, rootNode, _ := utils.SplitStorageSpaceID(spaceID)
			root = &provider.ResourceId{
				StorageId: rootSpace,
				OpaqueId:  rootNode,
			}
			// build reference for the provider
			r := &provider.Reference{
				ResourceId: req.Ref.ResourceId,
				Path:       req.Ref.Path,
			}
			// NOTE: There are problems in the following case:
			// Given a req.Ref.Path = "/projects" and a mountpath = "/projects/projectA"
			// Then it will request path "/projects/projectA" from the provider
			// But it should only request "/" as the ResourceId already points to the correct resource
			// TODO: We need to cut the path in case the resourceId is already pointing to correct resource
			if r.Path != "" && strings.HasPrefix(mountPath, r.Path) { // requesting the root in that case - No Path accepted
				r.Path = "/"
			}
			providerRef := unwrap(r, mountPath, root)

			// there are three valid cases when listing trash
			// 1. id based references of a space
			// 2. path based references of a space
			// 3. relative reference -> forward as is

			// we can ignore spaces below the mount point
			// -> only match exact references
			if requestPath == mountPath {

				res, err := c.ListRecycle(ctx, &provider.ListRecycleRequest{
					Opaque: req.Opaque,
					FromTs: req.FromTs,
					ToTs:   req.ToTs,
					Ref:    providerRef,
					Key:    req.Key,
				})
				if err != nil {
					return nil, errors.Wrap(err, "gateway: error calling ListRecycle")
				}

				if utils.IsAbsoluteReference(req.Ref) {
					for j := range res.RecycleItems {
						// wrap(res.RecycleItems[j].Ref, p) only handles ResourceInfo
						res.RecycleItems[j].Ref.Path = path.Join(mountPath, res.RecycleItems[j].Ref.Path)
					}
				}

				return res, nil
			}
		}

	}

	return &provider.ListRecycleResponse{
		Status: status.NewNotFound(ctx, "ListRecycle no matching provider found ref="+req.Ref.String()),
	}, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	// requestPath := req.Ref.Path
	providerInfos, err := s.findSpaces(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem source ref="+req.Ref.String(), err),
		}, nil
	}
	var srcProvider *registry.ProviderInfo
	var srcRef *provider.Reference
	for i := range providerInfos {

		var root *provider.ResourceId
		for spaceID, mountPath := range decodeSpacePaths(providerInfos[i]) {
			rootSpace, rootNode, _ := utils.SplitStorageSpaceID(spaceID)
			root = &provider.ResourceId{
				StorageId: rootSpace,
				OpaqueId:  rootNode,
			}
			// build reference for the provider
			r := &provider.Reference{
				ResourceId: req.Ref.ResourceId,
				Path:       req.Ref.Path,
			}
			// NOTE: There are problems in the following case:
			// Given a req.Ref.Path = "/projects" and a mountpath = "/projects/projectA"
			// Then it will request path "/projects/projectA" from the provider
			// But it should only request "/" as the ResourceId already points to the correct resource
			// TODO: We need to cut the path in case the resourceId is already pointing to correct resource
			if r.Path != "" && strings.HasPrefix(mountPath, r.Path) { // requesting the root in that case - No Path accepted
				r.Path = "/"
			}
			srcRef = unwrap(r, mountPath, root)
			srcProvider = providerInfos[i]
			break
		}
	}

	if srcProvider == nil || srcRef == nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewNotFound(ctx, "RestoreRecycleItemResponse no matching provider found ref="+req.Ref.String()),
		}, nil
	}

	// find destination
	dstProviderInfos, err := s.findSpaces(ctx, req.RestoreRef)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem source ref="+req.Ref.String(), err),
		}, nil
	}
	var dstProvider *registry.ProviderInfo
	var dstRef *provider.Reference
	for i := range dstProviderInfos {
		var root *provider.ResourceId
		for spaceID, mountPath := range decodeSpacePaths(dstProviderInfos[i]) {
			rootSpace, rootNode, _ := utils.SplitStorageSpaceID(spaceID)
			root = &provider.ResourceId{
				StorageId: rootSpace,
				OpaqueId:  rootNode,
			}
			// build reference for the provider
			r := &provider.Reference{
				ResourceId: req.RestoreRef.ResourceId,
				Path:       req.RestoreRef.Path,
			}
			// NOTE: There are problems in the following case:
			// Given a req.Ref.Path = "/projects" and a mountpath = "/projects/projectA"
			// Then it will request path "/projects/projectA" from the provider
			// But it should only request "/" as the ResourceId already points to the correct resource
			// TODO: We need to cut the path in case the resourceId is already pointing to correct resource
			if r.Path != "" && strings.HasPrefix(mountPath, r.Path) { // requesting the root in that case - No Path accepted
				r.Path = "/"
			}
			dstRef = unwrap(r, mountPath, root)
			dstProvider = providerInfos[i]
			break
		}
	}

	if dstProvider == nil || dstRef == nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewNotFound(ctx, "RestoreRecycleItemResponse no matching destination provider found ref="+req.RestoreRef.String()),
		}, nil
	}

	if srcRef.ResourceId.StorageId != dstRef.ResourceId.StorageId || srcProvider.Address != dstProvider.Address {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewPermissionDenied(ctx, err, "gateway: cross-storage restores are forbidden"),
		}, nil
	}

	// get client for storage provider
	c, err := s.getStorageProviderClient(ctx, srcProvider)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, "gateway: could not get storage provider client"),
		}, nil
	}

	req.Ref = srcRef
	req.RestoreRef = dstRef

	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	if req.RestoreRef != nil {
		s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.RestoreRef.ResourceId)
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, _, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "PurgeRecycle ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    relativeReference,
		Key:    req.Key,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, _, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, "GetQuota ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetQuota(ctx, &provider.GetQuotaRequest{
		Opaque: req.GetOpaque(),
		Ref:    relativeReference,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetQuota")
	}
	return res, nil
}

func (s *svc) findByPath(ctx context.Context, path string) (provider.ProviderAPIClient, *registry.ProviderInfo, error) {
	ref := &provider.Reference{Path: path}
	return s.find(ctx, ref)
}

// find looks up the provider that is responsible for the given request
// It will return a client that the caller can use to make the call, as well as the ProviderInfo. It:
// - contains the provider path, which is the mount point of the provider
// - may contain a list of storage spaces with their id and space path
func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, error) {
	p, err := s.findSpaces(ctx, ref)
	if err != nil {
		return nil, nil, err
	}

	client, err := s.getStorageProviderClient(ctx, p[0])
	return client, p[0], err
}

// FIXME findAndUnwrap currently just returns the first provider ... which may not be what is needed.
// for the ListRecycle call we need an exact match, for Stat and List we need to query all related providers
func (s *svc) findAndUnwrap(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, *provider.Reference, error) {
	c, p, err := s.find(ctx, ref)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		root      *provider.ResourceId
		mountPath string
	)
	for spaceID, spacePath := range decodeSpacePaths(p) {
		mountPath = spacePath
		rootSpace, rootNode, err := utils.SplitStorageSpaceID(spaceID)
		if err != nil {
			continue
		}
		root = &provider.ResourceId{
			StorageId: rootSpace,
			OpaqueId:  rootNode,
		}
		break // TODO can there be more than one space for a path?
	}

	relativeReference := unwrap(ref, mountPath, root)

	return c, p, relativeReference, nil
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return s.cache.StorageProviderClient(c), nil
}

func (s *svc) getStorageRegistryClient(_ context.Context, address string) (registry.RegistryAPIClient, error) {
	c, err := pool.GetStorageRegistryClient(address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return s.cache.StorageRegistryClient(c), nil
}

// func (s *svc) findMountPoint(ctx context.Context, id *provider.ResourceId) ([]*registry.ProviderInfo, error) {
// // TODO can we use a provider cache for mount points?
// if id == nil {
// return nil, errtypes.BadRequest("invalid reference, at least path or id must be set")
// }

// filters := map[string]string{
// "type":       "mountpoint",
// "storage_id": id.StorageId,
// "opaque_id":  id.OpaqueId,
// }

// listReq := &registry.ListStorageProvidersRequest{
// Opaque: &typesv1beta1.Opaque{},
// }
// sdk.EncodeOpaqueMap(listReq.Opaque, filters)

// return s.findProvider(ctx, listReq)
// }

func (s *svc) findSpaces(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
	switch {
	case ref == nil:
		return nil, errtypes.BadRequest("missing reference")
	case ref.ResourceId != nil:
		// no action needed in that case
	case ref.Path != "": //  TODO implement a mount path cache in the registry?
		// nothing to do here either
	default:
		return nil, errtypes.BadRequest("invalid reference, at least path or id must be set")
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

	return s.findProvider(ctx, listReq)
}

func (s *svc) findProvider(ctx context.Context, listReq *registry.ListStorageProvidersRequest) ([]*registry.ProviderInfo, error) {
	// lookup
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}
	res, err := c.ListStorageProviders(ctx, listReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListStorageProviders")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			// TODO use tombstone cache item?
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + listReq.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied("gateway: " + res.Status.Message + " for " + listReq.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest("gateway: " + res.Status.Message + " for " + listReq.String() + " with code " + res.Status.Code.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported("gateway: " + res.Status.Message + " for " + listReq.String() + " with code " + res.Status.Code.String())
		default:
			return nil, status.NewErrorFromCode(res.Status.Code, "gateway")
		}
	}

	if res.Providers == nil {
		return nil, errtypes.NotFound("gateway: provider is nil")
	}

	return res.Providers, nil
}

// unwrap takes a reference and builds a reference for the provider. can be absolute or relative to a root node
func unwrap(ref *provider.Reference, mountPoint string, root *provider.ResourceId) *provider.Reference {
	if utils.IsAbsolutePathReference(ref) {
		providerRef := &provider.Reference{
			Path: strings.TrimPrefix(ref.Path, mountPoint),
		}
		// if we have a root use it and make the path relative
		if root != nil {
			providerRef.ResourceId = root
			providerRef.Path = utils.MakeRelativePath(providerRef.Path)
		}
		return providerRef
	}

	return &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: ref.ResourceId.StorageId,
			OpaqueId:  ref.ResourceId.OpaqueId,
		},
		Path: ref.Path,
	}
}

func decodeSpacePaths(r *registry.ProviderInfo) map[string]string {
	spacePaths := map[string]string{}
	if r.Opaque != nil {
		if entry, ok := r.Opaque.Map["space_paths"]; ok {
			switch entry.Decoder {
			case "json":
				_ = json.Unmarshal(entry.Value, &spacePaths)
			case "toml":
				_ = toml.Unmarshal(entry.Value, &spacePaths)
			case "xml":
				_ = xml.Unmarshal(entry.Value, &spacePaths)
			}
		}
	}

	if len(spacePaths) == 0 {
		spacePaths[""] = r.ProviderPath
	}
	return spacePaths
}
