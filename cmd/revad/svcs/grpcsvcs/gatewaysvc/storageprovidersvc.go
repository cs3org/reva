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

	gatewayv0alphapb "github.com/cs3org/go-cs3apis/cs3/gateway/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/pkg/errors"
)

func (s *svc) InitiateFileDownload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileDownloadRequest) (*storageproviderv0alphapb.InitiateFileDownloadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.InitiateFileDownloadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.InitiateFileDownloadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	storageReq := &storageproviderv0alphapb.InitiateFileDownloadRequest{
		Opaque: req.Opaque,
		Ref:    req.Ref,
	}
	res, err := c.InitiateFileDownload(ctx, storageReq)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling InitiateFileDownload")
	}

	storageRes := &storageproviderv0alphapb.InitiateFileDownloadResponse{
		Status:           res.Status,
		Opaque:           res.Opaque,
		DownloadEndpoint: res.DownloadEndpoint,
	}
	return storageRes, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*storageproviderv0alphapb.InitiateFileUploadResponse, error) {
	c, err := s.find(ctx, req.Ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return &storageproviderv0alphapb.InitiateFileUploadResponse{
				Status: status.NewNotFound(ctx, "storage provider not found"),
			}, nil
		}
		return &storageproviderv0alphapb.InitiateFileUploadResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	res, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling InitiateFileUpload")
	}

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
