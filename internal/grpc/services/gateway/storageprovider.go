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
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// transerClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target string `json:"target"`
}

func (s *svc) sign(ctx context.Context, target string) (string, error) {
	ttl := time.Duration(s.c.TranserExpires) * time.Second
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

	homeReq := &provider.GetHomeRequest{}
	homeRes, err := s.GetHome(ctx, homeReq)
	if err != nil {
		log.Err(err).Msgf("gateway: error calling GetHome")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error creating home"),
		}, nil
	}

	if homeRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(homeRes.Status.Code, "gateway")
		log.Err(err).Msg("gateway: bad grpc code")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling GetHome"),
		}, nil
	}

	c, err := s.findByPath(ctx, homeRes.Path)
	if err != nil {
		log.Err(err).Msg("gateway: error finding storage provider")
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.CreateHomeResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.CreateHome(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error creating home on storage provider")
		return &provider.CreateHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling CreateHome"),
		}, nil
	}

	return res, nil

}
func (s *svc) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting storage registry client")
		return &provider.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error finding storage registry"),
		}, nil
	}

	res, err := c.GetHome(ctx, &registry.GetHomeRequest{})
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetHome")
		return &provider.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling GetHome"),
		}, nil
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		return &provider.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error calling GetHome"),
		}, nil
	}

	log.Info().Msg("gateway: home for user at provider=" + res.Provider.Address)

	storageClient, err := pool.GetStorageProviderServiceClient(res.Provider.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting storage provider client")
		return &provider.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error getting home"),
		}, nil
	}

	homeRes, err := storageClient.GetHome(ctx, req)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting GetHome from storage provider")
		return &provider.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error getting home"),
		}, nil
	}

	return homeRes, nil
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*gateway.InitiateFileDownloadResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gateway.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gateway.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileDownload")
	}

	res := &gateway.InitiateFileDownloadResponse{
		Opaque:           storageRes.Opaque,
		Status:           storageRes.Status,
		DownloadEndpoint: storageRes.DownloadEndpoint,
	}

	if storageRes.Expose {
		log.Info().Msg("download is routed directly to data server - skiping datagateway")
		return res, nil
	}

	// sign the download location and pass it to the data gateway
	u, err := url.Parse(res.DownloadEndpoint)
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

	res.DownloadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*gateway.InitiateFileUploadResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gateway.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling InitiateFileUpload")
	}

	res := &gateway.InitiateFileUploadResponse{
		Opaque:             storageRes.Opaque,
		Status:             storageRes.Status,
		UploadEndpoint:     storageRes.UploadEndpoint,
		AvailableChecksums: storageRes.AvailableChecksums,
	}

	if storageRes.Expose {
		log.Info().Msg("upload is routed directly to data server - skiping datagateway")
		return res, nil
	}

	// sign the upload location and pass it to the data gateway
	u, err := url.Parse(res.UploadEndpoint)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "wrong format for upload endpoint"),
		}, nil
	}

	// TODO(labkode): calculate signature of the url, we only sign the URI. At some points maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
	target := u.String()
	token, err := s.sign(ctx, target)
	if err != nil {
		return &gateway.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error creating signature for download"),
		}, nil
	}

	res.UploadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	res := &provider.GetPathResponse{
		Status: status.NewUnimplemented(ctx, nil, "GetPath not yet implemented"),
	}
	return res, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.CreateContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateContainer")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.DeleteResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	srcP, err := s.findProvider(ctx, req.Source)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.MoveResponse{
				Status: status.NewNotFound(ctx, "source storage provider not found"),
			}, nil
		}
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	dstP, err := s.findProvider(ctx, req.Destination)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.MoveResponse{
				Status: status.NewNotFound(ctx, "destination storage provider not found"),
			}, nil
		}
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	// if providers are not the same we do not implement cross storage copy yet.
	if srcP.Address != dstP.Address {
		res := &provider.MoveResponse{
			Status: status.NewUnimplemented(ctx, nil, "gateway: cross storage copy not yet implemented"),
		}
		return res, nil
	}

	c, err := s.getStorageProviderClient(ctx, srcP)
	if err != nil {
		return &provider.MoveResponse{
			Status: status.NewInternal(ctx, err, "error connecting to storage provider="+srcP.Address),
		}, nil
	}

	return c.Move(ctx, req)
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.SetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.SetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.SetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Stat")
	}

	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.UnsetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.UnsetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.UnsetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Stat")
	}

	return res, nil
}

func (s *svc) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.StatResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.Stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling Stat")
	}

	return res, nil
}

func (s *svc) ListContainerStream(req *provider.ListContainerStreamRequest, ss gateway.GatewayAPI_ListContainerStreamServer) error {
	return errors.New("Unimplemented")
}

func (s *svc) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListContainer")
	}

	return res, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListFileVersionsResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
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
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.RestoreFileVersionResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreFileVersion")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(req *gateway.ListRecycleStreamRequest, ss gateway.GatewayAPI_ListRecycleStreamServer) error {
	return errors.New("Unimplemented")
}

// TODO use the ListRecycleRequest.Ref to only list the trish of a specific storage
func (s *svc) ListRecycle(ctx context.Context, req *gateway.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	c, err := s.find(ctx, req.GetRef())
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.ListRecycleResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.ListRecycleResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListRecycle(ctx, &provider.ListRecycleRequest{
		Opaque: req.Opaque,
		FromTs: req.FromTs,
		ToTs:   req.ToTs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListRecycleRequest")
	}

	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.RestoreRecycleItemResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.RestoreRecycleItemResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.RestoreRecycleItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RestoreRecycleItem")
	}

	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *gateway.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	// lookup storagy by treating the key as a path. It has been prefixed with the storage path in ListRecycle
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &provider.PurgeRecycleResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &provider.PurgeRecycleResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{
		Opaque: req.GetOpaque(),
		Ref:    req.GetRef(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling PurgeRecycle")
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gateway.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	res := &provider.GetQuotaResponse{
		Status: status.NewUnimplemented(ctx, nil, "GetQuota not yet implemented"),
	}
	return res, nil
}

func (s *svc) findByID(ctx context.Context, id *provider.ResourceId) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{
		Spec: &provider.Reference_Id{
			Id: id,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) findByPath(ctx context.Context, path string) (provider.ProviderAPIClient, error) {
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{
			Path: path,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) find(ctx context.Context, ref *provider.Reference) (provider.ProviderAPIClient, error) {
	p, err := s.findProvider(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.getStorageProviderClient(ctx, p)
}

func (s *svc) getStorageProviderClient(ctx context.Context, p *registry.ProviderInfo) (provider.ProviderAPIClient, error) {
	c, err := pool.GetStorageProviderServiceClient(p.Address)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting a storage provider client")
		return nil, err
	}

	return c, nil
}

func (s *svc) findProvider(ctx context.Context, ref *provider.Reference) (*registry.ProviderInfo, error) {
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting storage registry client")
		return nil, err
	}

	res, err := c.GetStorageProvider(ctx, &registry.GetStorageProviderRequest{
		Ref: ref,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetStorageProvider")
		return nil, err
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return nil, errtypes.NotFound("gateway: storage provider not found for reference:" + ref.String())
		}
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		return nil, err
	}

	if res.Provider == nil {
		err := errors.New("gateway: provider is nil")
		return nil, err
	}

	return res.Provider, nil
}
