// Copyright 2018-2019 CERN
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

package gatewaysvc

import (
	"context"
	"net/url"
	"time"

	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/user"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// transerClaims are custom claims for a JWT token to be used between the metadata and data gateways.
type transferClaims struct {
	jwt.StandardClaims
	Target string `json:"target"`
}

func (s *svc) sign(ctx context.Context, target string) (string, error) {
	u := user.ContextMustGetUser(ctx)
	ttl := time.Duration(s.c.TranserExpires) * time.Second
	claims := transferClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(ttl).Unix(),
			Issuer:    u.Id.Idp,
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

func (s *svc) InitiateFileDownload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileDownloadRequest) (*gatewayv0alphapb.InitiateFileDownloadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gatewayv0alphapb.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gatewayv0alphapb.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling InitiateFileDownload")
	}

	res := &gatewayv0alphapb.InitiateFileDownloadResponse{
		Opaque:           storageRes.Opaque,
		Status:           storageRes.Status,
		DownloadEndpoint: storageRes.DownloadEndpoint,
	}

	if storageRes.Expose {
		log.Info().Msg("download is routed directly to data server - skiping datagatewaysvc")
		return res, nil
	}

	// sign the download location and pass it to the data gateway
	u, err := url.Parse(res.DownloadEndpoint)
	if err != nil {
		return &gatewayv0alphapb.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "wrong format for download endpoint"),
		}, nil
	}

	// TODO(labkode): calculate signature of the url, we only sign the URI. At some points maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
	target := u.String()
	token, err := s.sign(ctx, target)
	if err != nil {
		return &gatewayv0alphapb.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error creating signature for download"),
		}, nil
	}

	res.DownloadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*gatewayv0alphapb.InitiateFileUploadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &gatewayv0alphapb.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &gatewayv0alphapb.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageRes, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling InitiateFileUpload")
	}

	res := &gatewayv0alphapb.InitiateFileUploadResponse{
		Opaque:             storageRes.Opaque,
		Status:             storageRes.Status,
		UploadEndpoint:     storageRes.UploadEndpoint,
		AvailableChecksums: storageRes.AvailableChecksums,
	}

	if storageRes.Expose {
		log.Info().Msg("download is routed directly to data server - skiping datagatewaysvc")
		return res, nil
	}

	// sign the upload location and pass it to the data gateway
	u, err := url.Parse(res.UploadEndpoint)
	if err != nil {
		return &gatewayv0alphapb.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "wrong format for upload endpoint"),
		}, nil
	}

	// TODO(labkode): calculate signature of the url, we only sign the URI. At some points maybe worth https://tools.ietf.org/html/draft-cavage-http-signatures-11
	target := u.String()
	token, err := s.sign(ctx, target)
	if err != nil {
		return &gatewayv0alphapb.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error creating signature for download"),
		}, nil
	}

	res.UploadEndpoint = s.c.DataGatewayEndpoint
	res.Token = token

	return res, nil
}

func (s *svc) GetPath(ctx context.Context, req *storageproviderv0alphapb.GetPathRequest) (*storageproviderv0alphapb.GetPathResponse, error) {
	res := &storageproviderv0alphapb.GetPathResponse{
		Status: status.NewUnimplemented(ctx, nil, "GetPath not yet implemented"),
	}
	return res, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *storageproviderv0alphapb.CreateContainerRequest) (*storageproviderv0alphapb.CreateContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.CreateContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.CreateContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling CreateContainer")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *storageproviderv0alphapb.DeleteRequest) (*storageproviderv0alphapb.DeleteResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.DeleteResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.DeleteResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Delete")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *storageproviderv0alphapb.MoveRequest) (*storageproviderv0alphapb.MoveResponse, error) {
	res := &storageproviderv0alphapb.MoveResponse{
		Status: status.NewUnimplemented(ctx, nil, "Move not yet implemented"),
	}
	return res, nil
}

func (s *svc) SetArbitraryMetadata(ctx context.Context, req *storageproviderv0alphapb.SetArbitraryMetadataRequest) (*storageproviderv0alphapb.SetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.SetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.SetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.SetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) UnsetArbitraryMetadata(ctx context.Context, req *storageproviderv0alphapb.UnsetArbitraryMetadataRequest) (*storageproviderv0alphapb.UnsetArbitraryMetadataResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.UnsetArbitraryMetadataResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.UnsetArbitraryMetadataResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.UnsetArbitraryMetadata(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) Stat(ctx context.Context, req *storageproviderv0alphapb.StatRequest) (*storageproviderv0alphapb.StatResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.StatResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.StatResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.Stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) ListContainerStream(req *storageproviderv0alphapb.ListContainerStreamRequest, ss gatewayv0alphapb.GatewayService_ListContainerStreamServer) error {
	return errors.New("Unimplemented")
}

func (s *svc) ListContainer(ctx context.Context, req *storageproviderv0alphapb.ListContainerRequest) (*storageproviderv0alphapb.ListContainerResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.ListContainerResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListContainer")
	}

	return res, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *storageproviderv0alphapb.ListFileVersionsRequest) (*storageproviderv0alphapb.ListFileVersionsResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.ListFileVersionsResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.ListFileVersionsResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.ListFileVersions(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling ListFileVersions")
	}

	return res, nil
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *storageproviderv0alphapb.RestoreFileVersionRequest) (*storageproviderv0alphapb.RestoreFileVersionResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.RestoreFileVersionResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.RestoreFileVersionResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling RestoreFileVersion")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(req *gatewayv0alphapb.ListRecycleStreamRequest, ss gatewayv0alphapb.GatewayService_ListRecycleStreamServer) error {
	return errors.New("Unimplemented")
}

func (s *svc) ListRecycle(ctx context.Context, req *gatewayv0alphapb.ListRecycleRequest) (*storageproviderv0alphapb.ListRecycleResponse, error) {
	// TODO(labkode): query all available storage providers to get unified list as the request does not come
	// with ref information to target only one storage provider.
	res := &storageproviderv0alphapb.ListRecycleResponse{
		Status: status.NewUnimplemented(ctx, nil, "ListRecycle not yet implemented"),
	}
	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *storageproviderv0alphapb.RestoreRecycleItemRequest) (*storageproviderv0alphapb.RestoreRecycleItemResponse, error) {
	res := &storageproviderv0alphapb.RestoreRecycleItemResponse{
		Status: status.NewUnimplemented(ctx, nil, "RestoreRecycleItem not yet implemented"),
	}
	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *gatewayv0alphapb.PurgeRecycleRequest) (*storageproviderv0alphapb.PurgeRecycleResponse, error) {
	res := &storageproviderv0alphapb.PurgeRecycleResponse{
		Status: status.NewUnimplemented(ctx, nil, "PurgeRecycle not yet implemented"),
	}
	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *gatewayv0alphapb.GetQuotaRequest) (*storageproviderv0alphapb.GetQuotaResponse, error) {
	res := &storageproviderv0alphapb.GetQuotaResponse{
		Status: status.NewUnimplemented(ctx, nil, "GetQuota not yet implemented"),
	}
	return res, nil
}

func (s *svc) findByID(ctx context.Context, id *storageproviderv0alphapb.ResourceId) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Id{
			Id: id,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) findByPath(ctx context.Context, path string) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{
			Path: path,
		},
	}
	return s.find(ctx, ref)
}

func (s *svc) find(ctx context.Context, ref *storageproviderv0alphapb.Reference) (storageproviderv0alphapb.StorageProviderServiceClient, error) {
	c, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error getting storage registry client")
		return nil, err
	}

	res, err := c.GetStorageProvider(ctx, &storageregistryv0alphapb.GetStorageProviderRequest{
		Ref: ref,
	})

	if err != nil {
		err = errors.Wrap(err, "gatewaysvc: error calling GetStorageProvider")
		return nil, err
	}

	if res.Status.Code == rpcpb.Code_CODE_OK && res.Provider != nil {
		// TODO(labkode): check for capabilities here
		c, err := pool.GetStorageProviderServiceClient(res.Provider.Address)
		if err != nil {
			err = errors.Wrap(err, "gatewaysvc: error getting a storage provider client")
			return nil, err
		}

		return c, nil
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gatewaysvc: storage provider not found for reference:" + ref.String())
	}

	return nil, errors.New("gatewaysvc: error finding a storage provider")
}
