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
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
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
	"github.com/cs3org/reva/pkg/utils"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
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
	c, err := s.findByPath(ctx, "/users")
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

		providers = make([]*registry.ProviderInfo, 0, len(res.Providers))
		// FIXME filter only providers that have an id set ... currently none have?
		// bug? only ProviderPath is set
		for i := range res.Providers {
			// use only providers whose path does not start with a /?
			if strings.HasPrefix(res.Providers[i].ProviderPath, "/") {
				continue
			}
			providers = append(providers, res.Providers[i])
		}
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
	c, err := s.find(ctx, &provider.Reference{ResourceId: req.StorageSpace.Root})
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
	c, err := s.find(ctx, &provider.Reference{ResourceId: &provider.ResourceId{
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
	if utils.IsRelativeReference(req.Ref) {
		return s.initiateFileDownload(ctx, req)
	}

	_, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileDownloadResponse{
			Status: st,
		}, nil
	}

	statReq := &provider.StatRequest{Ref: req.Ref}
	statRes, err := s.stat(ctx, statReq)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "gateway: error stating ref:"+statReq.Ref.String()),
		}, nil
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileDownloadResponse{
			Status: statRes.Status,
		}, nil
	}
	return s.initiateFileDownload(ctx, req)
}

func (s *svc) initiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	// TODO(ishank011): enable downloading references spread across storage providers, eg. /eos
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewStatusFromErrType(ctx, "error initiating download ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
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
	if utils.IsRelativeReference(req.Ref) {
		return s.initiateFileUpload(ctx, req)
	}
	_, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		return &gateway.InitiateFileUploadResponse{
			Status: st,
		}, nil
	}

	return s.initiateFileUpload(ctx, req)
}

func (s *svc) initiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewStatusFromErrType(ctx, "initiateFileUpload ref="+req.Ref.String(), err),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
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
	statRes, err := s.stat(ctx, statReq)
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
	if utils.IsRelativeReference(req.Ref) {
		return s.createContainer(ctx, req)
	}

	_, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.CreateContainerResponse{
			Status: st,
		}, nil
	}

	return s.createContainer(ctx, req)
}

func (s *svc) createContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
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
	_, st := s.getPath(ctx, req.Ref)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.DeleteResponse{
			Status: st,
		}, nil
	}

	return s.delete(ctx, req)
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
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	_, st := s.getPath(ctx, req.Source)
	if st.Code != rpc.Code_CODE_OK {
		return &provider.MoveResponse{
			Status: st,
		}, nil
	}

	_, st2 := s.getPath(ctx, req.Destination)
	if st2.Code != rpc.Code_CODE_OK && st2.Code != rpc.Code_CODE_NOT_FOUND {
		return &provider.MoveResponse{
			Status: st2,
		}, nil
	}

	return s.move(ctx, req)
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
		return nil, errors.Wrap(err, "gateway: error calling Stat")
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
		return nil, errors.Wrap(err, "gateway: error calling Stat")
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

	if etagIface, err := s.etagCache.Get(statRes.Info.Owner.OpaqueId + ":" + statRes.Info.Path); err == nil {
		resMtime := utils.TSToTime(statRes.Info.Mtime)
		resEtag := etagIface.(etagWithTS)
		// Use the updated etag if the home folder has been modified
		if resMtime.Before(resEtag.Timestamp) {
			statRes.Info.Etag = resEtag.Etag
		}
	} else if s.c.EtagCacheTTL > 0 {
		_ = s.etagCache.Set(statRes.Info.Owner.OpaqueId+":"+statRes.Info.Path, etagWithTS{statRes.Info.Etag, time.Now()})
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

		res, err := c.Stat(ctx, req)
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "error connecting to storage provider="+providers[0].Address),
			}, nil
		}

		embeddedMounts := s.findEmbeddedMounts(resPath)
		if len(embeddedMounts) > 0 {
			etagHash := md5.New()
			if res.Info != nil {
				_, _ = io.WriteString(etagHash, res.Info.Etag)
			}
			for _, child := range embeddedMounts {
				childStatRes, err := s.stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: child}})
				if err != nil {
					return &provider.StatResponse{
						Status: status.NewStatusFromErrType(ctx, "stat ref: "+req.Ref.String(), err),
					}, nil
				}
				_, _ = io.WriteString(etagHash, childStatRes.Info.Etag)
			}

			if res.Info == nil {
				res.Info = &provider.ResourceInfo{}
			}
			res.Info.Etag = fmt.Sprintf("%x", etagHash.Sum(nil))
		}

		return res, nil
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
		Mtime:    &typesv1beta1.Timestamp{},
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

	return s.stat(ctx, req)
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

	return lcr, nil
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

	for _, p := range providers {
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

	// Inject mountpoints if they do not exist on disk
	embeddedMounts := s.findEmbeddedMounts(path.Clean(req.Ref.GetPath()))
	for _, mount := range embeddedMounts {
		for _, info := range infos {
			if info.Path == mount {
				continue
			}
		}
		infos = append(infos,
			&provider.ResourceInfo{
				Id: &provider.ResourceId{
					StorageId: "/",
					OpaqueId:  uuid.New().String(),
				},
				Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Etag: uuid.New().String(),
				Path: mount,
				Size: 0,
			})
	}

	return &provider.ListContainerResponse{
		Status: status.NewOK(ctx),
		Infos:  infos,
	}, nil
}

func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	if utils.IsAbsolutePathReference(req.Ref) {
		// look up provider in space registry
		return s.listContainer(ctx, req)
	}
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
		res, err := s.listHome(ctx, req)
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewStatusFromErrType(ctx, "listContainer ref: "+req.Ref.String(), err),
			}, nil
		}

		// Inject mountpoints if they do not exist on disk
		embeddedMounts := s.findEmbeddedMounts(path.Clean(req.Ref.GetPath()))
		for _, mount := range embeddedMounts {
			for _, info := range res.Infos {
				if info.Path == mount { // hmm this makes existing folders hide a mount, right?
					continue
				}
			}
			childStatRes, err := s.stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: mount}})
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewStatusFromErrType(ctx, "ListContainer ref: "+req.Ref.String(), err),
				}, nil
			}
			res.Infos = append(res.Infos, childStatRes.Info)
		}
		return res, nil
	}

	return s.listContainer(ctx, req)
}

func (s *svc) getPath(ctx context.Context, ref *provider.Reference, keys ...string) (string, *rpc.Status) {

	// check if it is an id based or combined reference first
	if ref.ResourceId != nil {
		req := &provider.StatRequest{Ref: ref, ArbitraryMetadataKeys: keys}
		res, err := s.stat(ctx, req)
		if err != nil {
			return "", status.NewStatusFromErrType(ctx, "getPath ref="+ref.String(), err)
		}
		if res != nil && res.Status.Code != rpc.Code_CODE_OK {
			return "", res.Status
		}

		return res.Info.Path, res.Status
	}

	if utils.IsAbsolutePathReference(ref) {
		return ref.Path, &rpc.Status{Code: rpc.Code_CODE_OK}
	}
	return "", &rpc.Status{Code: rpc.Code_CODE_INTERNAL}
}

func (s *svc) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return &provider.CreateSymlinkResponse{
		Status: status.NewUnimplemented(ctx, errtypes.NotSupported("CreateSymlink not implemented"), "CreateSymlink not implemented"),
	}, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	c, err := s.find(ctx, req.Ref)
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
	c, err := s.find(ctx, req.Ref)
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
	c, err := s.find(ctx, req.GetRef())
	if err != nil {
		return &provider.ListRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "ListFileVersions ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.ListRecycle(ctx, req)
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

	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewStatusFromErrType(ctx, "RestoreRecycleItem ref="+req.Ref.String(), err),
		}, nil
	}

	// TODO: HELP?!?
	req.RestoreRef.Path = strings.TrimPrefix(req.RestoreRef.Path, destinationProviderInfo[0].ProviderPath)
	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.PurgeRecycleResponse{
			Status: status.NewStatusFromErrType(ctx, "PurgeRecycle ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		return &provider.GetQuotaResponse{
			Status: status.NewStatusFromErrType(ctx, "GetQuota ref="+req.Ref.String(), err),
		}, nil
	}

	res, err := c.GetQuota(ctx, &provider.GetQuotaRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.GetRef(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetQuota")
	}
	return res, nil
}

func (s *svc) findByPath(ctx context.Context, path string) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{Path: path}
	return s.find(ctx, ref)
}

func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, error) {
	p, err := s.findProviders(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.getStorageProviderClient(ctx, p[0])
}

func (s *svc) getStorageProviderClient(_ context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (s *svc) findEmbeddedMounts(basePath string) []string {
	if basePath == "" {
		return []string{}
	}
	mounts := []string{}
	for mountPath := range s.c.StorageRules {
		if strings.HasPrefix(mountPath, basePath) && mountPath != basePath {
			mounts = append(mounts, mountPath)
		}
	}
	return mounts
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

func getUniqueProviders(providers []*registry.ProviderInfo) []*registry.ProviderInfo {
	unique := make(map[string]bool)
	for _, p := range providers {
		unique[p.Address] = true
	}
	p := make([]*registry.ProviderInfo, 0, len(unique))
	for addr := range unique {
		p = append(p, &registry.ProviderInfo{Address: addr})
	}
	return p
}

type etagWithTS struct {
	Etag      string
	Timestamp time.Time
}
