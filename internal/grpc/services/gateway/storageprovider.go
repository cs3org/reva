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
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"google.golang.org/grpc/codes"

	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/publicshare"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	sdk "github.com/cs3org/reva/v2/pkg/sdk/common"
	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"
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
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return &provider.CreateHomeResponse{
			Status: status.NewPermissionDenied(ctx, nil, "can't create home for anonymous user"),
		}, nil

	}
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
			Status: status.NewStatusFromErrType(ctx, "gateway could not call CreateStorageSpace", err),
		}, nil
	}
	return &provider.CreateHomeResponse{
		Opaque: res.Opaque,
		Status: res.Status,
	}, nil
}

func (s *svc) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
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
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could get storage registry client", err),
		}, nil
	}

	spaceJSON, err := json.Marshal(space)
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not marshal space json", err),
		}, nil
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
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call GetStorageProviders", err),
		}, nil
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return &provider.CreateStorageSpaceResponse{
			Status: res.Status,
		}, nil
	}

	if len(res.Providers) == 0 {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewNotFound(ctx, fmt.Sprintf("gateway found no provider for space %+v", space)),
		}, nil
	}

	// just pick the first provider, we expect only one
	c, err := s.getStorageProviderClient(ctx, res.Providers[0])
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not get storage provider client", err),
		}, nil
	}
	createRes, err := c.CreateStorageSpace(ctx, req)
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call CreateStorageSpace", err),
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
		return &provider.ListStorageSpacesResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not get storage registry client", err),
		}, nil
	}

	listReq := &registry.ListStorageProvidersRequest{Opaque: req.Opaque}
	if listReq.Opaque == nil {
		listReq.Opaque = &typesv1beta1.Opaque{}
	}
	if len(filters) > 0 {
		sdk.EncodeOpaqueMap(listReq.Opaque, filters)
	}
	res, err := c.ListStorageProviders(ctx, listReq)
	if err != nil {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call ListStorageSpaces", err),
		}, nil
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListStorageSpacesResponse{
			Status: res.Status,
		}, nil
	}

	spaces := []*provider.StorageSpace{}
	for _, providerInfo := range res.Providers {
		spaces = append(spaces, decodeSpaces(providerInfo)...)
	}

	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
}

func (s *svc) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	// TODO: needs to be fixed
	ref := &provider.Reference{ResourceId: req.StorageSpace.Root}
	c, _, err := s.find(ctx, ref)
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find reference %+v", ref), err),
		}, nil
	}

	res, err := c.UpdateStorageSpace(ctx, req)
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call UpdateStorageSpace", err),
		}, nil
	}

	if res.Status.Code == rpc.Code_CODE_OK {
		id := res.StorageSpace.Root
		s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), id)
		s.cache.RemoveListStorageProviders(id)
	}
	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	opaque := req.Opaque
	var purge bool
	// This is just a temporary hack until the CS3 API get's updated to have a dedicated purge parameter or a dedicated PurgeStorageSpace method.
	if opaque != nil {
		_, purge = opaque.Map["purge"]
	}

	storageid, opaqeid, err := utils.SplitStorageSpaceID(req.Id.OpaqueId)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not split space id %s", req.GetId().GetOpaqueId()), err),
		}, nil
	}

	ref := &provider.Reference{ResourceId: &provider.ResourceId{
		StorageId: storageid,
		OpaqueId:  opaqeid,
	}}
	c, _, err := s.find(ctx, ref)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find reference %+v", ref), err),
		}, nil
	}

	dsRes, err := c.DeleteStorageSpace(ctx, req)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call DeleteStorageSpace", err),
		}, nil
	}

	id := &provider.ResourceId{OpaqueId: req.Id.OpaqueId}
	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), id)
	s.cache.RemoveListStorageProviders(id)

	if dsRes.Status.Code != rpc.Code_CODE_OK {
		return dsRes, nil
	}

	if !purge {
		return dsRes, nil
	}

	log := appctx.GetLogger(ctx)
	log.Debug().Msg("purging storage space")
	// List all shares in this storage space
	lsRes, err := s.ListShares(ctx, &collaborationv1beta1.ListSharesRequest{
		Filters: []*collaborationv1beta1.Filter{share.StorageIDFilter(storageid)},
	})
	switch {
	case err != nil:
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not delete shares of StorageSpace", err),
		}, nil
	case lsRes.Status.Code != rpc.Code_CODE_OK:
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInternal(ctx, "gateway could not delete shares of StorageSpace"),
		}, nil
	}
	for _, share := range lsRes.Shares {
		rsRes, err := s.RemoveShare(ctx, &collaborationv1beta1.RemoveShareRequest{
			Ref: &collaborationv1beta1.ShareReference{
				Spec: &collaborationv1beta1.ShareReference_Id{Id: share.Id},
			},
		})
		if err != nil || rsRes.Status.Code != rpc.Code_CODE_OK {
			log.Error().Err(err).Interface("status", rsRes.Status).Str("share_id", share.Id.OpaqueId).Msg("failed to delete share")
		}
	}

	// List all public shares in this storage space
	lpsRes, err := s.ListPublicShares(ctx, &linkv1beta1.ListPublicSharesRequest{
		Filters: []*linkv1beta1.ListPublicSharesRequest_Filter{publicshare.StorageIDFilter(storageid)},
	})
	switch {
	case err != nil:
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not delete shares of StorageSpace", err),
		}, nil
	case lpsRes.Status.Code != rpc.Code_CODE_OK:
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInternal(ctx, "gateway could not delete shares of StorageSpace"),
		}, nil
	}
	for _, share := range lpsRes.Share {
		rsRes, err := s.RemovePublicShare(ctx, &linkv1beta1.RemovePublicShareRequest{
			Ref: &linkv1beta1.PublicShareReference{
				Spec: &linkv1beta1.PublicShareReference_Id{Id: share.Id},
			},
		})
		if err != nil || rsRes.Status.Code != rpc.Code_CODE_OK {
			log.Error().Err(err).Interface("status", rsRes.Status).Str("share_id", share.Id.OpaqueId).Msg("failed to delete share")
		}
	}

	return dsRes, nil
}

func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	currentUser := ctxpkg.ContextMustGetUser(ctx)

	srClient, err := s.getStorageRegistryClient(ctx, s.c.StorageRegistryEndpoint)
	if err != nil {
		return &provider.GetHomeResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not get storage registry client", err),
		}, nil
	}

	spaceJSON, err := json.Marshal(&provider.StorageSpace{
		Owner:     currentUser,
		SpaceType: "personal",
	})
	if err != nil {
		return &provider.GetHomeResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not marshal space", err),
		}, nil
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
		return &provider.GetHomeResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call GetStorageProviders", err),
		}, nil
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

	// NOTE: this will cause confusion if len(spaces) > 1
	spaces := decodeSpaces(res.Providers[0])
	for _, space := range spaces {
		return &provider.GetHomeResponse{
			Path:   decodePath(space),
			Status: status.NewOK(ctx),
		}, nil
	}

	return &provider.GetHomeResponse{
		Status: status.NewNotFound(ctx, fmt.Sprintf("error finding home path for provider %+v with spaces %+v ", res.Providers[0], spaces)),
	}, nil
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	// TODO(ishank011): enable downloading references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not call InitiateFileDownload, ref=%+v", req.Ref), err),
		}, nil
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
					Status: status.NewStatusFromErrType(ctx, "wrong format for download endpoint", err),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewStatusFromErrType(ctx, "error creating signature for download", err),
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
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not call InitiateFileUpload, ref=%+v", req.Ref), err),
		}, nil
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
					Status: status.NewStatusFromErrType(ctx, "wrong format for upload endpoint", err),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewStatusFromErrType(ctx, "error creating signature for upload", err),
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
	c, _, ref, err := s.findAndUnwrap(ctx, &provider.Reference{ResourceId: req.ResourceId})
	if err != nil {
		return &provider.GetPathResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find reference %+v", ref), err),
		}, nil
	}

	req.ResourceId = ref.ResourceId
	return c.GetPath(ctx, req)
}

func (s *svc) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
<<<<<<< HEAD
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call CreateContainer", err),
		}, nil
	}
=======
			Status: status.NewStatusFromErrType(ctx, "createContainer ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.CreateContainerResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	return res, nil
}

func (s *svc) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	c, err := s.find(ctx, req.Ref)
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

// check if the path contains the prefix of the shared folder
func (s *svc) inSharedFolder(ctx context.Context, p string) bool {
	sharedFolder := s.getSharedFolder(ctx)
	return strings.HasPrefix(p, sharedFolder)
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.DeleteResponse{
			Status: st,
		}, nil
	}

	ctx, span := rtrace.Provider.Tracer("reva").Start(ctx, "Delete")
	defer span.End()

	if !s.inSharedFolder(ctx, p) {
		return s.delete(ctx, req)
	}

	if s.isSharedFolder(ctx, p) {
		// TODO(labkode): deleting share names should be allowed, means unmounting.
		err := errtypes.BadRequest("gateway: cannot delete share folder or share name: path=" + p)
		span.RecordError(err)
		return &provider.DeleteResponse{
			Status: status.NewInvalidArg(ctx, "path points to share folder or share name"),
		}, nil

	}

	if s.isShareName(ctx, p) {
		log.Debug().Msgf("path:%s points to share name", p)

		sRes, err := s.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
		if err != nil {
			return nil, err
		}

		statRes, err := s.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				Path: p,
			},
		})
		if err != nil {
			return nil, err
		}

		// the following will check that:
		// - the resource to delete is a share the current user received
		// - signal the storage the delete must not land in the trashbin
		// - delete the resource and update the share status to "rejected"
		for _, share := range sRes.Shares {
			if statRes != nil && (share.Share.ResourceId.OpaqueId == statRes.Info.Id.OpaqueId) && (share.Share.ResourceId.StorageId == statRes.Info.Id.StorageId) {
				// this opaque needs explanation. It signals the storage the resource we're about to delete does not
				// belong to the current user because it was share to her, thus delete the "node" and don't send it to
				// the trash bin, since the share can be mounted as many times as desired.
				req.Opaque = &types.Opaque{
					Map: map[string]*types.OpaqueEntry{
						"deleting_shared_resource": {
							Value:   []byte("true"),
							Decoder: "plain",
						},
					},
				}

				// the following block takes care of updating the state of the share to "rejected". This will ensure the user
				// can "Accept" the share once again.
				// TODO should this be pending? If so, update the two comments above as well. If not, get rid of this comment.
				share.State = collaboration.ShareState_SHARE_STATE_REJECTED
				r := &collaboration.UpdateReceivedShareRequest{
					Share:      share,
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
				}

				_, err := s.UpdateReceivedShare(ctx, r)
				if err != nil {
					return nil, err
				}

				return &provider.DeleteResponse{
					Status: status.NewOK(ctx),
				}, nil
			}
		}

		return &provider.DeleteResponse{
			Status: status.NewNotFound(ctx, "could not find share"),
		}, nil
	}

	if s.isShareChild(ctx, p) {
		shareName, shareChild := s.splitShare(ctx, p)
		log.Debug().Msgf("path:%s sharename:%s sharechild: %s", p, shareName, shareChild)

		ref := &provider.Reference{Path: shareName}

		statReq := &provider.StatRequest{Ref: ref}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.DeleteResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			return &provider.DeleteResponse{
				Status: statRes.Status,
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			return &provider.DeleteResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+statRes.Info.Target, err),
			}, nil
		}

		if protocol == "webdav" {
			err = s.webdavRefDelete(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.DeleteResponse{
					Status: status.NewInternal(ctx, err, "gateway: error deleting resource on webdav host: "+p),
				}, nil
			}
			return &provider.DeleteResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		// append child to target
		req.Ref.Path = path.Join(ri.Path, shareChild)
		return s.delete(ctx, req)
	}

	panic("gateway: delete called on unknown path:" + p)
}

func (s *svc) delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	// TODO(ishank011): enable deleting references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewStatusFromErrType(ctx, "delete ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.DeleteResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	log := appctx.GetLogger(ctx)
	p, st := s.getPath(ctx, req.Source)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	dp, st := s.getPath(ctx, req.Destination)
	if st.Code != rpc.Code_CODE_OK && st.Code != rpc.Code_CODE_NOT_FOUND {
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	if !s.inSharedFolder(ctx, p) && !s.inSharedFolder(ctx, dp) {
		return s.move(ctx, req)
	}

	// allow renaming the share folder, the mount point, not the target.
	if s.isShareName(ctx, p) && s.isShareName(ctx, dp) {
		log.Info().Msgf("gateway: move: renaming share mountpoint: from:%s to:%s", p, dp)
		return s.move(ctx, req)
	}

	// resolve references and check the ref points to the same base path, paranoia check.
	if s.isShareChild(ctx, p) && s.isShareChild(ctx, dp) {
		shareName, shareChild := s.splitShare(ctx, p)
		dshareName, dshareChild := s.splitShare(ctx, dp)
		log.Debug().Msgf("srcpath:%s dstpath:%s srcsharename:%s srcsharechild: %s dstsharename:%s dstsharechild:%s ", p, dp, shareName, shareChild, dshareName, dshareChild)

		srcStatReq := &provider.StatRequest{Ref: &provider.Reference{Path: shareName}}
		srcStatRes, err := s.stat(ctx, srcStatReq)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+srcStatReq.Ref.String()),
			}, nil
		}

		if srcStatRes.Status.Code != rpc.Code_CODE_OK {
			return &provider.MoveResponse{
				Status: srcStatRes.Status,
			}, nil
		}

		dstStatReq := &provider.StatRequest{Ref: &provider.Reference{Path: dshareName}}
		dstStatRes, err := s.stat(ctx, dstStatReq)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+srcStatReq.Ref.String()),
			}, nil
		}

		if dstStatRes.Status.Code != rpc.Code_CODE_OK {
			return &provider.MoveResponse{
				Status: srcStatRes.Status,
			}, nil
		}

		srcRi, srcProtocol, err := s.checkRef(ctx, srcStatRes.Info)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+srcStatRes.Info.Target, err),
			}, nil
		}

		if srcProtocol == "webdav" {
			err = s.webdavRefMove(ctx, dstStatRes.Info.Target, shareChild, dshareChild)
			if err != nil {
				return &provider.MoveResponse{
					Status: status.NewInternal(ctx, err, "gateway: error moving resource on webdav host: "+p),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewOK(ctx),
			}, nil
		}
		dstRi, dstProtocol, err := s.checkRef(ctx, dstStatRes.Info)
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+srcStatRes.Info.Target, err),
			}, nil
		}

		if dstProtocol == "webdav" {
			err = s.webdavRefMove(ctx, dstStatRes.Info.Target, shareChild, dshareChild)
			if err != nil {
				return &provider.MoveResponse{
					Status: status.NewInternal(ctx, err, "gateway: error moving resource on webdav host: "+p),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		src := &provider.Reference{
			Path: path.Join(srcRi.Path, shareChild),
		}
		dst := &provider.Reference{
			Path: path.Join(dstRi.Path, dshareChild),
		}

		req.Source = src
		req.Destination = dst

		return s.move(ctx, req)
	}

	return &provider.MoveResponse{
		Status: status.NewStatusFromErrType(ctx, "move", errtypes.BadRequest("gateway: move called on unknown path: "+p)),
	}, nil
}

func (s *svc) move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	srcProviders, err := s.findProviders(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "move src="+req.Source.String(), err),
		}, nil
	}

	dstProviders, err := s.findProviders(ctx, req.Destination)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "move dst="+req.Destination.String(), err),
		}, nil
	}

	// if providers are not the same we do not implement cross storage move yet.
	if len(srcProviders) != 1 || len(dstProviders) != 1 {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	srcProvider, dstProvider := srcProviders[0], dstProviders[0]

	// if providers are not the same we do not implement cross storage copy yet.
	if srcProvider.Address != dstProvider.Address {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	c, err := s.getStorageProviderClient(ctx, srcProvider)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error connecting to storage provider="+srcProvider.Address),
		}, nil
	}

	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
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

	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
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

	return res, nil
}

// SetLock puts a lock on the given reference
func (s *svc) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.SetLockResponse{
			Status: status.NewStatusFromErrType(ctx, "SetLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.SetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.SetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling SetLock")
	}

	return res, nil
}

// GetLock returns an existing lock on the given reference
func (s *svc) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetLockResponse{
			Status: status.NewStatusFromErrType(ctx, "GetLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.GetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling GetLock")
	}

	return res, nil
}

// RefreshLock refreshes an existing lock on the given reference
func (s *svc) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RefreshLockResponse{
			Status: status.NewStatusFromErrType(ctx, "RefreshLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RefreshLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.RefreshLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling RefreshLock")
	}

	return res, nil
}

// Unlock removes an existing lock from the given reference
func (s *svc) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.UnlockResponse{
			Status: status.NewStatusFromErrType(ctx, "Unlock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Unlock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.UnlockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Unlock")
	}

	return res, nil
}

func (s *svc) statHome(ctx context.Context) (*provider.StatResponse, error) {
	statRes, err := s.stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: s.getHome(ctx)}})
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating home"),
		}, nil
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: statRes.Status,
		}, nil
	}

	statSharedFolder, err := s.statSharesFolder(ctx)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating shares folder"),
		}, nil
	}
	if statSharedFolder.Status.Code != rpc.Code_CODE_OK {
		// If shares folder is not found, skip updating the etag
		if statSharedFolder.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return statRes, nil
		}
		// otherwise return stat of share folder
		return &provider.StatResponse{
			Status: statSharedFolder.Status,
		}, nil
	}

	if etagIface, err := s.etagCache.Get(statRes.Info.Owner.OpaqueId + ":" + statRes.Info.Path); err == nil {
		resMtime := utils.TSToTime(statRes.Info.Mtime)
		resEtag := etagIface.(etagWithTS)
		// Use the updated etag if the home folder has been modified
		if resMtime.Before(resEtag.Timestamp) {
			statRes.Info.Etag = resEtag.Etag
		}
	} else {
		statRes.Info.Etag = etag.GenerateEtagFromResources(statRes.Info, []*provider.ResourceInfo{statSharedFolder.Info})
		if s.c.EtagCacheTTL > 0 {
			_ = s.etagCache.Set(statRes.Info.Owner.OpaqueId+":"+statRes.Info.Path, etagWithTS{statRes.Info.Etag, time.Now()})
		}
	}

	return statRes, nil
}

func (s *svc) statSharesFolder(ctx context.Context) (*provider.StatResponse, error) {
	statRes, err := s.stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: s.getSharedFolder(ctx)}})
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating shares folder"),
		}, nil
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: statRes.Status,
		}, nil
	}

	lsRes, err := s.listSharesFolder(ctx)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "gateway: error listing shares folder"),
		}, nil
	}
	if lsRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: lsRes.Status,
		}, nil
	}

	if etagIface, err := s.etagCache.Get(statRes.Info.Owner.OpaqueId + ":" + statRes.Info.Path); err == nil {
		resMtime := utils.TSToTime(statRes.Info.Mtime)
		resEtag := etagIface.(etagWithTS)
		// Use the updated etag if the shares folder has been modified, i.e., a new
		// reference has been created.
		if resMtime.Before(resEtag.Timestamp) {
			statRes.Info.Etag = resEtag.Etag
		}
	} else {
		statRes.Info.Etag = etag.GenerateEtagFromResources(statRes.Info, lsRes.Infos)
		if s.c.EtagCacheTTL > 0 {
			_ = s.etagCache.Set(statRes.Info.Owner.OpaqueId+":"+statRes.Info.Path, etagWithTS{statRes.Info.Etag, time.Now()})
		}
	}
	return statRes, nil
}

func (s *svc) stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	providers, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewStatusFromErrType(ctx, "stat ref: "+req.Ref.String(), err),
		}, nil
	}
	providers = getUniqueProviders(providers)

	resPath := req.Ref.GetPath()
	if len(providers) == 1 && (utils.IsRelativeReference(req.Ref) || resPath == "" || strings.HasPrefix(resPath, providers[0].ProviderPath)) {
		c, err := s.getStorageProviderClient(ctx, providers[0])
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "error connecting to storage provider="+providers[0].Address),
			}, nil
		}
		rsp, err := c.Stat(ctx, req)
		if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
			return rsp, err
		}
		return rsp, nil
	}

	return s.statAcrossProviders(ctx, req, providers)
}

func (s *svc) statAcrossProviders(ctx context.Context, req *provider.StatRequest, providers []*registry.ProviderInfo) (*provider.StatResponse, error) {
	// TODO(ishank011): aggregrate properties such as etag, checksum, etc.
	log := appctx.GetLogger(ctx)
	info := &provider.ResourceInfo{
		Id: &provider.ResourceId{
			StorageId: "/",
			OpaqueId:  uuid.New().String(),
		},
		Type:     provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Path:     req.Ref.GetPath(),
		MimeType: "httpd/unix-directory",
		Size:     0,
		Mtime:    &types.Timestamp{},
	}

	for _, p := range providers {
		c, err := s.getStorageProviderClient(ctx, p)
		if err != nil {
			log.Err(err).Msg("error connecting to storage provider=" + p.Address)
			continue
		}
		resp, err := c.Stat(ctx, req)
		if err != nil {
			log.Err(err).Msgf("gateway: error calling Stat %s: %+v", req.Ref.String(), p)
			continue
		}
		if resp.Status.Code != rpc.Code_CODE_OK {
			log.Err(status.NewErrorFromCode(rpc.Code_CODE_OK, "gateway"))
			continue
		}
		if resp.Info != nil {
			info.Size += resp.Info.Size
			if utils.TSToUnixNano(resp.Info.Mtime) > utils.TSToUnixNano(info.Mtime) {
				info.Mtime = resp.Info.Mtime
				info.Etag = resp.Info.Etag
				info.Checksum = resp.Info.Checksum
			}
			if info.Etag == "" && info.Etag != resp.Info.Etag {
				info.Etag = resp.Info.Etag
			}
		}
	}

	return &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info:   info,
	}, nil
}

func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	if utils.IsRelativeReference(req.Ref) {
		return s.stat(ctx, req)
	}

	p := ""
	var res *provider.StatResponse
	var err error
	if utils.IsAbsolutePathReference(req.Ref) {
		p = req.Ref.Path
	} else {
		// Reference by just resource ID
		// Stat it and store for future use
		res, err = s.stat(ctx, req)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}
		if res != nil && res.Status.Code != rpc.Code_CODE_OK {
			return res, nil
		}
		p = res.Info.Path
	}

	if path.Clean(p) == s.getHome(ctx) {
		return s.statHome(ctx)
	}

	if s.isSharedFolder(ctx, p) {
		return s.statSharesFolder(ctx)
	}

	if !s.inSharedFolder(ctx, p) {
		if res != nil {
			return res, nil
		}
		return s.stat(ctx, req)
	}

	// we need to provide the info of the target, not the reference.
	if s.isShareName(ctx, p) {
		// If we haven't returned an error by now and res is nil, it means that
		// req is an absolute path based ref, so we didn't stat it previously.
		// So stat it now
		if res == nil {
			res, err = s.stat(ctx, req)
			if err != nil {
				return &provider.StatResponse{
					Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
				}, nil
			}

			if res.Status.Code != rpc.Code_CODE_OK {
				return &provider.StatResponse{
					Status: res.Status,
				}, nil
			}
		}

		ri, protocol, err := s.checkRef(ctx, res.Info)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+res.Info.Target, err),
			}, nil
		}

		if protocol == "webdav" {
			ri, err = s.webdavRefStat(ctx, res.Info.Target)
			if err != nil {
				return &provider.StatResponse{
					Status: status.NewInternal(ctx, err, "gateway: error resolving webdav reference: "+p),
				}, nil
			}
		}

		// we need to make sure we don't expose the reference target in the resource
		// information. For example, if requests comes to: /home/MyShares/photos and photos
		// is reference to /user/peter/Holidays/photos, we need to still return to the user
		// /home/MyShares/photos
		orgPath := res.Info.Path
		res.Info = ri
		res.Info.Path = orgPath
		return res, nil

	}

	if s.isShareChild(ctx, p) {
		shareName, shareChild := s.splitShare(ctx, p)

		statReq := &provider.StatRequest{Ref: &provider.Reference{Path: shareName}}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			return &provider.StatResponse{
				Status: statRes.Status,
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+statRes.Info.Target, err),
			}, nil
		}

		if protocol == "webdav" {
			ri, err = s.webdavRefStat(ctx, statRes.Info.Target, shareChild)
			if err != nil {
				return &provider.StatResponse{
					Status: status.NewInternal(ctx, err, "gateway: error resolving webdav reference: "+p),
				}, nil
			}
			ri.Path = p
			return &provider.StatResponse{
				Status: status.NewOK(ctx),
				Info:   ri,
			}, nil
		}

		// append child to target
		req.Ref.Path = path.Join(ri.Path, shareChild)
		res, err := s.stat(ctx, req)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+req.Ref.String()),
			}, nil
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return &provider.StatResponse{
				Status: res.Status,
			}, nil
		}

		// we need to make sure we don't expose the reference target in the resource
		// information.
		res.Info.Path = p
		return res, nil
	}

	panic("gateway: stating an unknown path:" + p)
}

func (s *svc) checkRef(ctx context.Context, ri *provider.ResourceInfo) (*provider.ResourceInfo, string, error) {
	if ri.Type != provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		panic("gateway: calling checkRef on a non reference type:" + ri.String())
	}

	// reference types MUST have a target resource id.
	if ri.Target == "" {
		err := errtypes.BadRequest("gateway: ref target is an empty uri")
		return nil, "", err
	}

	uri, err := url.Parse(ri.Target)
	if err != nil {
		return nil, "", errors.Wrapf(err, "gateway: error parsing target uri: %s", ri.Target)
	}

	switch uri.Scheme {
	case "cs3":
		ref, err := s.handleCS3Ref(ctx, uri.Opaque)
		return ref, "cs3", err
	case "webdav":
		return nil, "webdav", nil
	default:
		err := errtypes.BadRequest("gateway: no reference handler for scheme: " + uri.Scheme)
		return nil, "", err
	}
}

func (s *svc) handleCS3Ref(ctx context.Context, opaque string) (*provider.ResourceInfo, error) {
	// a cs3 ref has the following layout: <storage_id>/<opaque_id>
	parts := strings.SplitN(opaque, "/", 2)
	if len(parts) < 2 {
		err := errtypes.BadRequest("gateway: cs3 ref does not follow the layout storageid/opaqueid:" + opaque)
		return nil, err
	}

	// we could call here the Stat method again, but that is calling for problems in case
	// there is a loop of targets pointing to targets, so better avoid it.

	req := &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: parts[0],
				OpaqueId:  parts[1],
			},
		},
	}
	res, err := s.stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling stat")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			return nil, errtypes.NotFound(req.Ref.String())
		case rpc.Code_CODE_PERMISSION_DENIED:
			return nil, errtypes.PermissionDenied(req.Ref.String())
		case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_OUT_OF_RANGE:
			return nil, errtypes.BadRequest(req.Ref.String())
		case rpc.Code_CODE_UNIMPLEMENTED:
			return nil, errtypes.NotSupported(req.Ref.String())
		default:
			return nil, errtypes.InternalError("gateway: error stating target reference")
		}
	}

	if res.Info.Type == provider.ResourceType_RESOURCE_TYPE_REFERENCE {
		err := errtypes.BadRequest("gateway: error the target of a reference cannot be another reference")
		return nil, err
	}

	return res.Info, nil
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errtypes.NotSupported("Unimplemented")
}

func (s *svc) listHome(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	lcr, err := s.listContainer(ctx, &provider.ListContainerRequest{
		Ref:                   &provider.Reference{Path: s.getHome(ctx)},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "gateway: error listing home"),
		}, nil
	}
	if lcr.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: lcr.Status,
		}, nil
	}

	for i := range lcr.Infos {
		if s.isSharedFolder(ctx, lcr.Infos[i].GetPath()) {
			statSharedFolder, err := s.statSharesFolder(ctx)
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error stating shares folder"),
				}, nil
			}
			if statSharedFolder.Status.Code != rpc.Code_CODE_OK {
				return &provider.ListContainerResponse{
					Status: statSharedFolder.Status,
				}, nil
			}
			lcr.Infos[i] = statSharedFolder.Info
			break
		}
	}

	return lcr, nil
}

func (s *svc) listSharesFolder(ctx context.Context) (*provider.ListContainerResponse, error) {
	lcr, err := s.listContainer(ctx, &provider.ListContainerRequest{Ref: &provider.Reference{Path: s.getSharedFolder(ctx)}})
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "gateway: error listing shared folder"),
		}, nil
	}
	if lcr.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: lcr.Status,
		}, nil
	}
	checkedInfos := make([]*provider.ResourceInfo, 0)
	for i := range lcr.Infos {
		info, protocol, err := s.checkRef(ctx, lcr.Infos[i])
		if err != nil {
			// create status to log the proper messages
			// this might arise when the shared resource has been moved to the recycle bin
			// this might arise when the resource was unshared, but the share reference was not removed
			status.NewStatusFromErrType(ctx, "error resolving reference "+lcr.Infos[i].Target, err)
			// continue on errors so the user can see a list of the working shares
			continue
		}

		if protocol == "webdav" {
			info, err = s.webdavRefStat(ctx, lcr.Infos[i].Target)
			if err != nil {
				// Might be the case that the webdav token has expired
				continue
			}
		}

		info.Path = lcr.Infos[i].Path
		checkedInfos = append(checkedInfos, info)
	}
	lcr.Infos = checkedInfos

	return lcr, nil
}

func (s *svc) isPathAllowed(ua *ua.UserAgent, path string) bool {
	uaLst, ok := s.c.AllowedUserAgents[path]
	if !ok {
		// if no user agent is defined for a path, all user agents are allowed
		return true
	}
	return useragent.IsUserAgentAllowed(ua, uaLst)
}

func (s *svc) filterProvidersByUserAgent(ctx context.Context, providers []*registry.ProviderInfo) []*registry.ProviderInfo {
	ua, ok := ctxpkg.ContextGetUserAgent(ctx)
	if !ok {
		return providers
	}

	filters := []*registry.ProviderInfo{}
	for _, p := range providers {
		if s.isPathAllowed(ua, p.ProviderPath) {
			filters = append(filters, p)
		}
	}
	return filters
}

func (s *svc) listContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	providers, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "listContainer ref: "+req.Ref.String(), err),
		}, nil
	}
	providers = getUniqueProviders(providers)

	resPath := req.Ref.GetPath()

	if len(providers) == 1 && (utils.IsRelativeReference(req.Ref) || resPath == "" || strings.HasPrefix(resPath, providers[0].ProviderPath)) {
		c, err := s.getStorageProviderClient(ctx, providers[0])
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "error connecting to storage provider="+providers[0].Address),
			}, nil
		}
		rsp, err := c.ListContainer(ctx, req)
		if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
			return rsp, err
		}
		return rsp, nil
	}

	return s.listContainerAcrossProviders(ctx, req, providers)
}

func (s *svc) listContainerAcrossProviders(ctx context.Context, req *provider.ListContainerRequest, providers []*registry.ProviderInfo) (*provider.ListContainerResponse, error) {
	nestedInfos := make(map[string]*provider.ResourceInfo)
	log := appctx.GetLogger(ctx)

	for _, p := range s.filterProvidersByUserAgent(ctx, providers) {
		c, err := s.getStorageProviderClient(ctx, p)
		if err != nil {
			log.Err(err).Msg("error connecting to storage provider=" + p.Address)
			continue
		}
		resp, err := c.ListContainer(ctx, req)
		if err != nil {
			log.Err(err).Msgf("gateway: error calling Stat %s: %+v", req.Ref.String(), p)
			continue
		}
		if resp.Status.Code != rpc.Code_CODE_OK {
			log.Err(status.NewErrorFromCode(rpc.Code_CODE_OK, "gateway"))
			continue
		}

		for _, info := range resp.Infos {
			if p, ok := nestedInfos[info.Path]; ok {
				// Since more than one providers contribute to this path,
				// use a generic ID
				p.Id = &provider.ResourceId{
					StorageId: "/",
					OpaqueId:  uuid.New().String(),
				}
				// TODO(ishank011): aggregrate properties such as etag, checksum, etc.
				p.Size += info.Size
				if utils.TSToUnixNano(info.Mtime) > utils.TSToUnixNano(p.Mtime) {
					p.Mtime = info.Mtime
					p.Etag = info.Etag
					p.Checksum = info.Checksum
				}
				if p.Etag == "" && p.Etag != info.Etag {
					p.Etag = info.Etag
				}
				p.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
				p.MimeType = "httpd/unix-directory"
			} else {
				nestedInfos[info.Path] = info
			}
		}
	}

	infos := make([]*provider.ResourceInfo, 0, len(nestedInfos))
	for _, info := range nestedInfos {
		infos = append(infos, info)
	}

	return &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}, nil
}

func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	log := appctx.GetLogger(ctx)

	if utils.IsRelativeReference(req.Ref) {
		return s.listContainer(ctx, req)
	}

	p, st := s.getPath(ctx, req.Ref, req.ArbitraryMetadataKeys...)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: st,
		}, nil
	}

	if path.Clean(p) == s.getHome(ctx) {
		return s.listHome(ctx, req)
	}

	if s.isSharedFolder(ctx, p) {
		return s.listSharesFolder(ctx)
	}

	if !s.inSharedFolder(ctx, p) {
		return s.listContainer(ctx, req)
	}

	// we need to provide the info of the target, not the reference.
	if s.isShareName(ctx, p) {
		statReq := &provider.StatRequest{Ref: &provider.Reference{Path: p}}
		statRes, err := s.stat(ctx, statReq)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "gateway: error stating share:"+statReq.Ref.String()),
			}, nil
		}

		if statRes.Status.Code != rpc.Code_CODE_OK {
			return &provider.ListContainerResponse{
				Status: statRes.Status,
			}, nil
		}

		ri, protocol, err := s.checkRef(ctx, statRes.Info)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewStatusFromErrType(ctx, "error resolving reference "+statRes.Info.Target, err),
			}, nil
		}

		if protocol == "webdav" {
			infos, err := s.webdavRefLs(ctx, statRes.Info.Target)
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "gateway: error listing webdav reference: "+p),
				}, nil
			}

			for _, info := range infos {
				base := path.Base(info.Path)
				info.Path = path.Join(p, base)
			}
			return &provider.ListContainerResponse{
				Status: status.NewOK(ctx),
				Infos:  infos,
			}, nil
		}

		if ri.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			err := errtypes.NotSupported("gateway: list container: cannot list non-container type:" + ri.Path)
			log.Err(err).Msg("gateway: error listing")
			return &provider.ListContainerResponse{
				Status: status.NewInvalidArg(ctx, "resource is not a container"),
			}, nil
		}
>>>>>>> master

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
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call Delete", err),
		}, nil
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	c, sourceProviderInfo, sref, err := s.findAndUnwrap(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Source), err),
		}, nil
	}

	_, destProviderInfo, dref, err := s.findAndUnwrap(ctx, req.Destination)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Source), err),
		}, nil
	}

	if sourceProviderInfo.Address != destProviderInfo.Address {
		return &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway does not support cross storage move, use copy and delete"),
		}, nil
	}

	req.Source = sref
	req.Destination = dref
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
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
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
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
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

// SetLock puts a lock on the given reference
func (s *svc) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.SetLockResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.SetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.SetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling SetLock")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

// GetLock returns an existing lock on the given reference
func (s *svc) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetLockResponse{
			Status: status.NewStatusFromErrType(ctx, "GetLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.GetLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling GetLock")
	}

	return res, nil
}

// RefreshLock refreshes an existing lock on the given reference
func (s *svc) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RefreshLockResponse{
			Status: status.NewStatusFromErrType(ctx, "RefreshLock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RefreshLock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.RefreshLockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling RefreshLock")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

// Unlock removes an existing lock from the given reference
func (s *svc) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	c, _, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.UnlockResponse{
			Status: status.NewStatusFromErrType(ctx, "Unlock ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Unlock(ctx, req)
	if err != nil {
		if gstatus.Code(err) == codes.PermissionDenied {
			return &provider.UnlockResponse{Status: &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}}, nil
		}
		return nil, errors.Wrap(err, "gateway: error calling Unlock")
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

// Stat returns the Resoure info for a given resource by forwarding the request to the responsible provider.
// TODO cache info
func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	c, _, ref, err := s.findAndUnwrapUnique(ctx, req.Ref)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewNotFound(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref)),
		}, nil
	}

	return c.Stat(ctx, &provider.StatRequest{Opaque: req.Opaque, Ref: ref, ArbitraryMetadataKeys: req.ArbitraryMetadataKeys})
}

func (s *svc) ListContainerStream(_ *provider.ListContainerStreamRequest, _ gateway.GatewayAPI_ListContainerStreamServer) error {
	return errtypes.NotSupported("Unimplemented")
}

// ListContainer lists the Resoure infos for a given resource by forwarding the request to the responsible provider.
func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	c, _, ref, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		// we have no provider -> not found
		return &provider.ListContainerResponse{
			Status: status.NewNotFound(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref)),
		}, nil
	}

	return c.ListContainer(ctx, &provider.ListContainerRequest{
		Opaque:                req.Opaque,
		Ref:                   ref,
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
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListFileVersionsResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	return c.ListFileVersions(ctx, req)
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, _, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call RestoreFileVersion", err),
		}, nil
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) ListRecycleStream(_ *provider.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errtypes.NotSupported("ListRecycleStream unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trash of a specific storage
func (s *svc) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, _, ref, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}
	return c.ListRecycle(ctx, &provider.ListRecycleRequest{
		Opaque: req.Opaque,
		FromTs: req.FromTs,
		ToTs:   req.ToTs,
		Ref:    ref,
		Key:    req.Key,
	})
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	c, si, ref, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	_, di, rref, err := s.findAndUnwrap(ctx, req.RestoreRef)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	if si.Address != di.Address {
		return &provider.RestoreRecycleItemResponse{
			// TODO in Move() we return an unimplemented / supported ... align?
			Status: status.NewPermissionDenied(ctx, err, "gateway: cross-storage restores are forbidden"),
		}, nil
	}

	req.Ref = ref
	req.RestoreRef = rref
	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call RestoreRecycleItem", err),
		}, nil
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, _, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    relativeReference,
		Key:    req.Key,
	})
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call PurgeRecycle", err),
		}, nil
	}

	s.cache.RemoveStat(ctxpkg.ContextMustGetUser(ctx), req.Ref.ResourceId)
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, _, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, fmt.Sprintf("gateway could not find space for ref=%+v", req.Ref), err),
		}, nil
	}

	res, err := c.GetQuota(ctx, &provider.GetQuotaRequest{
		Opaque: req.GetOpaque(),
		Ref:    relativeReference,
	})
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, "gateway could not call GetQuota", err),
		}, nil
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

func (s *svc) findUnique(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, error) {
	p, err := s.findSingleSpace(ctx, ref)
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
	for _, space := range decodeSpaces(p) {
		mountPath = decodePath(space)
		root = space.Root
		break // TODO can there be more than one space for a path?
	}

	relativeReference := unwrap(ref, mountPath, root)

	return c, p, relativeReference, nil
}

func (s *svc) findAndUnwrapUnique(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, *provider.Reference, error) {
	c, p, err := s.findUnique(ctx, ref)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		root      *provider.ResourceId
		mountPath string
	)
	for _, space := range decodeSpaces(p) {
		mountPath = decodePath(space)
		root = space.Root
		break // TODO can there be more than one space for a path?
	}

	relativeReference := unwrap(ref, mountPath, root)

	return c, p, relativeReference, nil
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		return nil, err
	}

	return s.cache.StorageProviderClient(c), nil
}

func (s *svc) getStorageRegistryClient(_ context.Context, address string) (registry.RegistryAPIClient, error) {
	c, err := pool.GetStorageRegistryClient(address)
	if err != nil {
		return nil, err
	}

	return s.cache.StorageRegistryClient(c), nil
}

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
		Opaque: &typesv1beta1.Opaque{Map: make(map[string]*typesv1beta1.OpaqueEntry)},
	}

	sdk.EncodeOpaqueMap(listReq.Opaque, filters)

	return s.findProvider(ctx, listReq)
}

func (s *svc) findSingleSpace(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
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
		"path":   ref.Path,
		"unique": "true",
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

func decodeSpaces(r *registry.ProviderInfo) []*provider.StorageSpace {
	spaces := []*provider.StorageSpace{}
	if r.Opaque != nil {
		if entry, ok := r.Opaque.Map["spaces"]; ok {
			switch entry.Decoder {
			case "json":
				_ = json.Unmarshal(entry.Value, &spaces)
			case "toml":
				_ = toml.Unmarshal(entry.Value, &spaces)
			case "xml":
				_ = xml.Unmarshal(entry.Value, &spaces)
			}
		}
	}
	if len(spaces) == 0 {
		// we need to convert the provider into a space, needed for the static registry
		spaces = append(spaces, &provider.StorageSpace{
			Opaque: &typesv1beta1.Opaque{Map: map[string]*typesv1beta1.OpaqueEntry{
				"path": {
					Decoder: "plain",
					Value:   []byte(r.ProviderPath),
				},
			}},
		})
	}
	return spaces
}

func decodePath(s *provider.StorageSpace) (path string) {
	if s.Opaque != nil {
		if entry, ok := s.Opaque.Map["path"]; ok {
			switch entry.Decoder {
			case "plain":
				path = string(entry.Value)
			case "json":
				_ = json.Unmarshal(entry.Value, &path)
			case "toml":
				_ = toml.Unmarshal(entry.Value, &path)
			case "xml":
				_ = xml.Unmarshal(entry.Value, &path)
			}
		}
	}
	return
}
