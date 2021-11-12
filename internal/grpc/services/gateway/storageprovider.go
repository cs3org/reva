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
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"

	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/pkg/errors"

	"google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
)

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
	log := appctx.GetLogger(ctx)

	// ask registry for home provider
	storageRegistryClient, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}
	gHRes, err := storageRegistryClient.GetHome(ctx, &registry.GetHomeRequest{})
	if err != nil {
		log.Err(err).Msg("gateway: error getting home from storage registry")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}
	if gHRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.CreateHomeResponse{
			Status: gHRes.Status,
		}, nil
	}

	storageProviderClient, err := s.getStorageProviderClient(ctx, gHRes.Provider)
	if err != nil {
		log.Err(err).Msg("gateway: error getting storage provider cllient")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}

	u := ctxpkg.ContextMustGetUser(ctx)
	res, err := storageProviderClient.CreateStorageSpace(ctx, &provider.CreateStorageSpaceRequest{
		Type: "personal",
		// TODO sendt Id = u.Id.Opaqueid
		Owner: u,
		Name:  u.DisplayName,
	})
	if err != nil {
		log.Err(err).Msg("gateway: error creating personal storage space")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}
	if res.Status.Code != rpc.Code_CODE_OK && res.Status.Code != rpc.Code_CODE_ALREADY_EXISTS {
		return &provider.CreateHomeResponse{
			Status: res.Status,
		}, nil
	}

	/* TODO faill back to old CreateHome

	res, err := storageProviderClient.CreateHome(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating home on storage provider")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}
	*/
	return &provider.CreateHomeResponse{
		Status: res.Status,
	}, nil
}

func (s *svc) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO: needs to be fixed
	c, _, err := s.findByPath(ctx, "/users")
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewStatusFromErrType(ctx, "error finding path", err),
		}, nil
	}

	res, err := c.CreateStorageSpace(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating storage space on storage provider")
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateStorageSpace"),
		}, nil
	}
	return res, nil
}

func (s *svc) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	log := appctx.GetLogger(ctx)
	var id *provider.StorageSpaceId
	for _, f := range req.Filters {
		if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
			id = f.GetId()
		}
	}

	var (
		providers []*registry.ProviderInfo
		err       error
	)
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	if id != nil {
		// query that specific storage provider
		/*
			storageid, opaqeid, err := utils.SplitStorageSpaceID(id.OpaqueId)
			if err != nil {
				return &provider.ListStorageSpacesResponse{
					Status: status.NewInvalidArg(ctx, "space id must be separated by !"),
				}, nil
			}
		*/

		// TODO This actually returns spaces when using the space registry
		res, err := c.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
			Ref: &provider.Reference{ResourceId: &provider.ResourceId{
				StorageId: id.OpaqueId,
				//OpaqueId:  opaqeid,
			},
				Path: "./"}, // use a relative reference
		})
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
		providers = res.Providers
	} else {
		// get list of all storage providers
		// TODO This actually returns spaces when using the space registry
		res, err := c.ListStorageProviders(ctx, &registry.ListStorageProvidersRequest{})

		if err != nil {
			return &provider.ListStorageSpacesResponse{
				Status: status.NewStatusFromErrType(ctx, "error listing providers", err),
			}, nil
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return &provider.ListStorageSpacesResponse{
				Status: res.Status,
			}, nil
		}

		providers = res.Providers
	}

	spacesFromProviders := make([][]*provider.StorageSpace, len(providers))
	errors := make([]error, len(providers))

	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go s.listStorageSpacesOnProvider(ctx, req, &spacesFromProviders[i], p, &errors[i], &wg)
	}
	wg.Wait()

	uniqueSpaces := map[string]*provider.StorageSpace{}
	for i := range providers {
		if errors[i] != nil {
			if len(providers) > 1 {
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
			Status: status.NewInternal(ctx, err, "error calling UpdateStorageSpace"),
		}, nil
	}
	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO: needs to be fixed
	storageid, opaqeid, err := utils.SplitStorageSpaceID(req.Id.OpaqueId)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInvalidArg(ctx, "space id must be separated by !"),
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
			Status: status.NewInternal(ctx, err, "error calling DeleteStorageSpace"),
		}, nil
	}
	return res, nil
}

func (s *svc) GetHome(ctx context.Context, _ *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return &provider.GetHomeResponse{
		Path:   s.getHome(ctx),
		Status: status.NewOK(ctx),
	}, nil
}

func (s *svc) getHome(_ context.Context) string {
	//u := ctxpkg.ContextMustGetUser(ctx)
	//return filepath.Join("/personal", u.Id.OpaqueId)
	// TODO use user layout
	// TODO(labkode): issue #601, /home will be hardcoded.
	return "/home"
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	// TODO(ishank011): enable downloading references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
					Status: status.NewInternal(ctx, err, "wrong format for download endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileDownloadResponse{
					Status: status.NewInternal(ctx, err, "error creating signature for download"),
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
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
					Status: status.NewInternal(ctx, err, "wrong format for upload endpoint"),
				}, nil
			}

			// TODO(labkode): calculate signature of the whole request? we only sign the URI now. Maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
			target := u.String()
			token, err := s.sign(ctx, target)
			if err != nil {
				return &gateway.InitiateFileUploadResponse{
					Status: status.NewInternal(ctx, err, "error creating signature for upload"),
				}, nil
			}

			protocols[p].UploadEndpoint = s.c.DataGatewayEndpoint
			protocols[p].Token = token
		}
	}

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
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.CreateContainerResponse{
			Status: status.NewStatusFromErrType(ctx, "createContainer ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	// TODO(ishank011): enable deleting references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.DeleteResponse{
			Status: status.NewStatusFromErrType(ctx, "delete ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	var c provider.ProviderAPIClient
	var err error
	c, req.Source, err = s.findAndUnwrap(ctx, req.Source)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "Move ref="+req.Source.String(), err),
		}, nil
	}

	_, req.Destination, err = s.findAndUnwrap(ctx, req.Destination)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewStatusFromErrType(ctx, "Move ref="+req.Destination.String(), err),
		}, nil
	}

	// if spaces are not the same we do not implement cross storage copy yet.
	// TODO allow for spaces on the same provider
	if !utils.ResourceIDEqual(req.Source.ResourceId, req.Destination.ResourceId) {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	// TODO(ishank011): enable for references spread across storage providers, eg. /eos
	var c provider.ProviderAPIClient
	var err error
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
	var c provider.ProviderAPIClient
	var err error
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
	// find the providers
	providers, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		// we have no provider -> not found
		return &provider.StatResponse{
			Status: status.NewStatusFromErrType(ctx, "could not find provider", err),
		}, nil
	}

	var info *provider.ResourceInfo
	for i := range providers {

		// get client for storage provider
		c, err := s.getStorageProviderClient(ctx, providers[i])
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not get storage provider client, skipping")
			continue
		}

		// build relative reference
		sRef := req.Ref
		if utils.IsAbsolutePathReference(req.Ref) {
			sRef, err = unwrap(req.Ref, providers[i].ProviderPath)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not unwrap reference, skipping")
				continue
			}
			parts := strings.SplitN(providers[i].ProviderId, "!", 2)
			if len(parts) != 2 {
				appctx.GetLogger(ctx).Error().Msg("gateway: invalid provider id, expected <storageid>!<opaqueid> format, got " + providers[i].ProviderId)
				continue
			}
			sRef.ResourceId = &provider.ResourceId{StorageId: parts[0], OpaqueId: parts[1]}
			sRef.Path = utils.MakeRelativePath(sRef.Path)
		}

		var currentInfo *provider.ResourceInfo
		switch {
		case req.Ref.Path == "": // id based request
			fallthrough
		case strings.HasPrefix(req.Ref.Path, "."): // space request
			fallthrough
		case providers[i].ProviderPath == req.Ref.Path: // matches
			fallthrough
		case strings.HasPrefix(req.Ref.Path, providers[i].ProviderPath): //  requested path is below mount point
			resp, err := c.Stat(ctx, &provider.StatRequest{Opaque: req.Opaque, Ref: sRef, ArbitraryMetadataKeys: req.ArbitraryMetadataKeys})
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not stat embedded mount, skipping")
				continue
			}
			if resp.Status.Code != rpc.Code_CODE_OK {
				appctx.GetLogger(ctx).Debug().Interface("status", resp.Status).Msg("gateway: stating embedded mount was not ok, skipping")
				continue
			}
			if resp.Info == nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: stat response for embedded mount carried no info, skipping")
				continue
			}
			currentInfo = resp.Info
		case strings.HasPrefix(providers[i].ProviderPath, req.Ref.Path): // requested path is above mount point
			parts := strings.SplitN(providers[i].ProviderId, "!", 2)
			if len(parts) != 2 {
				appctx.GetLogger(ctx).Error().Msg("gateway: invalid provider id, expected <storageid>!<opaqueid> format, got " + providers[i].ProviderId)
				continue
			}
			sRef := &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: parts[0], OpaqueId: parts[1]},
				Path:       ".",
			}
			statResp, err := c.Stat(ctx, &provider.StatRequest{Opaque: req.Opaque, Ref: sRef, ArbitraryMetadataKeys: req.ArbitraryMetadataKeys})
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
			// -> update metadata for /foo/bar -> set path to './bar'?
			statResp.Info.Path = strings.TrimPrefix(providers[i].ProviderPath, req.Ref.Path)
			statResp.Info.Path, _ = router.ShiftPath(statResp.Info.Path)
			statResp.Info.Path = utils.MakeRelativePath(statResp.Info.Path)
			// TODO invent resourceid?

			if utils.IsAbsoluteReference(req.Ref) {
				statResp.Info.Path = path.Join(req.Ref.Path, statResp.Info.Path)
			}
			currentInfo = statResp.Info
		default:
			log := appctx.GetLogger(ctx)
			log.Err(err).Msg("gateway: unhandled Stat case")
		}

		if info == nil {
			switch {
			case utils.IsAbsolutePathReference(req.Ref):
				currentInfo.Path = req.Ref.Path
			case utils.IsAbsoluteReference(req.Ref):
				// an id based references needs to adjust the path in the response with the provider path
				// TODO but the provider path is empty for
				wrap(currentInfo, providers[i])
			}
			info = currentInfo
		} else {
			// aggregate metadata

			info.Size += currentInfo.Size
			if info.Mtime == nil || (currentInfo.Mtime != nil && utils.TSToUnixNano(currentInfo.Mtime) > utils.TSToUnixNano(info.Mtime)) {
				info.Mtime = currentInfo.Mtime
				info.Etag = currentInfo.Etag
				//info.Checksum = resp.Info.Checksum
			}
			if info.Etag == "" && info.Etag != currentInfo.Etag {
				info.Etag = currentInfo.Etag
			}
			//info.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
			//info.MimeType = "httpd/unix-directory"
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
	// find the providers
	providers, err := s.findProviders(ctx, req.Ref)
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
	for i := range providers {

		// get client for storage provider
		c, err := s.getStorageProviderClient(ctx, providers[i])
		if err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not get storage provider client, skipping")
			continue
		}

		// build relative reference
		lcRef := req.Ref
		if utils.IsAbsolutePathReference(req.Ref) {
			lcRef, err = unwrap(req.Ref, providers[i].ProviderPath)
			if err != nil {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not unwrap reference, skipping")
				continue
			}
			parts := strings.SplitN(providers[i].ProviderId, "!", 2)
			if len(parts) != 2 {
				appctx.GetLogger(ctx).Error().Msg("gateway: invalid provider id, expected <storageid>!<opaqueid> format, got " + providers[i].ProviderId)
				continue
			}
			lcRef.ResourceId = &provider.ResourceId{StorageId: parts[0], OpaqueId: parts[1]}
			lcRef.Path = utils.MakeRelativePath(lcRef.Path)
		}

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
		case providers[i].ProviderPath == req.Ref.Path: // matches
			fallthrough
		case strings.HasPrefix(req.Ref.Path, providers[i].ProviderPath): //  requested path is below mount point
			rsp, err := c.ListContainer(ctx, &provider.ListContainerRequest{
				Opaque:                req.Opaque,
				Ref:                   lcRef,
				ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
			})
			if err != nil || rsp.Status.Code != rpc.Code_CODE_OK {
				appctx.GetLogger(ctx).Error().Err(err).Msg("gateway: could not list provider, skipping")
				continue
			}

			if utils.IsAbsoluteReference(req.Ref) {
				for j := range rsp.Infos {
					rsp.Infos[j].Path = path.Join(lcRef.Path, rsp.Infos[j].Path)
					wrap(rsp.Infos[j], providers[i])
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
		case strings.HasPrefix(providers[i].ProviderPath, req.Ref.Path): // requested path is above mount point
			//  requested path   provider path
			//  /foo           <=> /foo/bar        -> stat(spaceid, .)    -> add metadata for /foo/bar
			//  /foo           <=> /foo/bar/bif    -> stat(spaceid, .)    -> add metadata for /foo/bar
			parts := strings.SplitN(providers[i].ProviderId, "!", 2)
			if len(parts) != 2 {
				appctx.GetLogger(ctx).Error().Msg("gateway: invalid provider id, expected <storageid>!<opaqueid> format, got " + providers[i].ProviderId)
				continue
			}
			sRef := &provider.Reference{
				ResourceId: &provider.ResourceId{StorageId: parts[0], OpaqueId: parts[1]},
				Path:       ".",
			}
			statResp, err := c.Stat(ctx, &provider.StatRequest{Opaque: req.Opaque, Ref: sRef, ArbitraryMetadataKeys: req.ArbitraryMetadataKeys})
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
			// -> update metadata for /foo/bar -> set path to './bar'?
			statResp.Info.Path = strings.TrimPrefix(providers[i].ProviderPath, req.Ref.Path)
			statResp.Info.Path, _ = router.ShiftPath(statResp.Info.Path)
			statResp.Info.Path = utils.MakeRelativePath(statResp.Info.Path)
			// TODO invent resourceid?

			if utils.IsAbsoluteReference(req.Ref) {
				statResp.Info.Path = path.Join(req.Ref.Path, statResp.Info.Path)
			}

			// the stated path is above a mountpoint, so it must be a folder
			statResp.Info.Type = provider.ResourceType_RESOURCE_TYPE_CONTAINER

			if info, ok := infos[statResp.Info.Path]; !ok {
				// replace with younger info
				infos[statResp.Info.Path] = statResp.Info
			} else {
				if info.Mtime == nil || (statResp.Info.Mtime != nil && utils.TSToUnixNano(statResp.Info.Mtime) > utils.TSToUnixNano(info.Mtime)) {
					// replace with younger info
					infos[statResp.Info.Path] = statResp.Info
				}
			}
		default:
			log := appctx.GetLogger(ctx)
			log.Err(err).Msg("gateway: unhandled ListContainer case")
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
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
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
	c, req.Ref, err = s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreFileVersionResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreFileVersion ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreFileVersion")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(_ *provider.ListRecycleStreamRequest, _ gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errtypes.NotSupported("ListRecycleStream unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trash of a specific storage
func (s *svc) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.ListRecycle(ctx, &provider.ListRecycleRequest{
		Opaque: req.Opaque,
		FromTs: req.FromTs,
		ToTs:   req.ToTs,
		Ref:    relativeReference,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListRecycleRequest")
	}

	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	sourceProviderInfo, err := s.findProviders(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem ref="+req.Ref.String(), err),
		}, nil
	}
	destinationProviderInfo, err := s.findProviders(ctx, req.RestoreRef)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem ref="+req.Ref.String(), err),
		}, nil
	}
	if sourceProviderInfo[0].ProviderId != destinationProviderInfo[0].ProviderId ||
		sourceProviderInfo[0].ProviderPath != destinationProviderInfo[0].ProviderPath {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewPermissionDenied(ctx, err, "gateway: cross-storage restores are forbidden"),
		}, nil
	}

	c, p, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem ref="+req.Ref.String(), err),
		}, nil
	}
	if req.Ref, err = unwrap(req.Ref, p.ProviderPath); err != nil {
		return nil, err
	}
	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "PurgeRecycle ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    relativeReference,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, relativeReference, err := s.findAndUnwrap(ctx, req.Ref)
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

func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *registry.ProviderInfo, error) {
	p, err := s.findProviders(ctx, ref)
	if err != nil {
		return nil, nil, err
	}

	client, err := s.getStorageProviderClient(ctx, p[0])
	return client, p[0], err
}

func (s *svc) findAndUnwrap(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, *provider.Reference, error) {
	c, p, err := s.find(ctx, ref)
	if err != nil {
		return nil, nil, err
	}

	relativeReference := ref
	if utils.IsAbsolutePathReference(ref) {
		if relativeReference, err = unwrap(ref, p.ProviderPath); err != nil {
			return nil, nil, err
		}
		relativeReference.Path = utils.MakeRelativePath(relativeReference.Path)
		parts := strings.SplitN(p.ProviderId, "!", 2)
		if len(parts) != 2 {
			return nil, nil, errtypes.BadRequest("gateway: invalid provider id, expected <storageid>!<opaqueid> format, got " + p.ProviderId)
		}
		relativeReference.ResourceId = &provider.ResourceId{StorageId: parts[0], OpaqueId: parts[1]}
	}

	return c, relativeReference, nil
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (s *svc) findProviders(ctx context.Context, ref *provider.Reference) ([]*registry.ProviderInfo, error) {
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error getting storage registry client")
	}

	res, err := c.GetStorageProviders(ctx, &registry.GetStorageProvidersRequest{
		Ref: ref,
	})

	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetStorageProvider")
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		switch res.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
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

	return res.Providers, nil
}

func unwrap(ref *provider.Reference, providerPath string) (*provider.Reference, error) {
	// all references with an id can be passed on to the driver
	// there are two cases:
	// 1. absolute id references (resource_id is set, path is empty)
	// 2. relative references (resource_id is set, path starts with a `.`)
	if ref.GetResourceId() != nil {
		return ref, nil
	}

	if !strings.HasPrefix(ref.GetPath(), "/") {
		// abort, absolute path references must start with a `/`
		return nil, errtypes.BadRequest("ref is invalid: " + ref.String())
	}

	p := strings.TrimPrefix(ref.Path, providerPath)
	if p == "" {
		p = "/"
	}
	return &provider.Reference{Path: p}, nil
}

func wrap(ri *provider.ResourceInfo, providerInfo *registry.ProviderInfo) {
	ri.Path = path.Join(providerInfo.ProviderPath, ri.Path)
}
