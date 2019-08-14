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

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/pkg/errors"
)

func (s *svc) GetProvider(ctx context.Context, req *storageproviderv0alphapb.GetProviderRequest) (*storageproviderv0alphapb.GetProviderResponse, error) {
	res := &storageproviderv0alphapb.GetProviderResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) InitiateFileDownload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileDownloadRequest) (*storageproviderv0alphapb.InitiateFileDownloadResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.InitiateFileDownloadResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.InitiateFileDownloadResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.InitiateFileDownloadResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.InitiateFileDownload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) InitiateFileUpload(ctx context.Context, req *storageproviderv0alphapb.InitiateFileUploadRequest) (*storageproviderv0alphapb.InitiateFileUploadResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.InitiateFileUploadResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.InitiateFileUploadResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.InitiateFileUploadResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.InitiateFileUpload(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) GetPath(ctx context.Context, req *storageproviderv0alphapb.GetPathRequest) (*storageproviderv0alphapb.GetPathResponse, error) {
	res := &storageproviderv0alphapb.GetPathResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) CreateContainer(ctx context.Context, req *storageproviderv0alphapb.CreateContainerRequest) (*storageproviderv0alphapb.CreateContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.CreateContainerResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.CreateContainerResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.CreateContainerResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.CreateContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) Delete(ctx context.Context, req *storageproviderv0alphapb.DeleteRequest) (*storageproviderv0alphapb.DeleteResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.DeleteResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.DeleteResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.DeleteResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.Delete(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) Move(ctx context.Context, req *storageproviderv0alphapb.MoveRequest) (*storageproviderv0alphapb.MoveResponse, error) {
	res := &storageproviderv0alphapb.MoveResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) Stat(ctx context.Context, req *storageproviderv0alphapb.StatRequest) (*storageproviderv0alphapb.StatResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.StatResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.StatResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.StatResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.Stat(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) ListContainerStream(req *storageproviderv0alphapb.ListContainerStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListContainerStreamServer) error {
	return errors.New("unimplemented")
}

func (s *svc) ListContainer(ctx context.Context, req *storageproviderv0alphapb.ListContainerRequest) (*storageproviderv0alphapb.ListContainerResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.ListContainerResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.ListContainerResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.ListContainerResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.ListContainer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) ListFileVersions(ctx context.Context, req *storageproviderv0alphapb.ListFileVersionsRequest) (*storageproviderv0alphapb.ListFileVersionsResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.ListFileVersionsResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.ListFileVersionsResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.ListFileVersionsResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.ListFileVersions(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) RestoreFileVersion(ctx context.Context, req *storageproviderv0alphapb.RestoreFileVersionRequest) (*storageproviderv0alphapb.RestoreFileVersionResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.RestoreFileVersionResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.RestoreFileVersionResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.RestoreFileVersionResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.RestoreFileVersion(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) ListRecycleStream(req *storageproviderv0alphapb.ListRecycleStreamRequest, ss storageproviderv0alphapb.StorageProviderService_ListRecycleStreamServer) error {
	return errors.New("unimplemented")
}

func (s *svc) ListRecycle(ctx context.Context, req *storageproviderv0alphapb.ListRecycleRequest) (*storageproviderv0alphapb.ListRecycleResponse, error) {
	// TODO(labkode): query all available storage providers to get unified list as the request does not come
	// with ref information to target only one storage provider.
	res := &storageproviderv0alphapb.ListRecycleResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) RestoreRecycleItem(ctx context.Context, req *storageproviderv0alphapb.RestoreRecycleItemRequest) (*storageproviderv0alphapb.RestoreRecycleItemResponse, error) {
	res := &storageproviderv0alphapb.RestoreRecycleItemResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) PurgeRecycle(ctx context.Context, req *storageproviderv0alphapb.PurgeRecycleRequest) (*storageproviderv0alphapb.PurgeRecycleResponse, error) {
	res := &storageproviderv0alphapb.PurgeRecycleResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) ListGrants(ctx context.Context, req *storageproviderv0alphapb.ListGrantsRequest) (*storageproviderv0alphapb.ListGrantsResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.ListGrantsResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.ListGrantsResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.ListGrantsResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.ListGrants(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) AddGrant(ctx context.Context, req *storageproviderv0alphapb.AddGrantRequest) (*storageproviderv0alphapb.AddGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.AddGrantResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.AddGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.AddGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.AddGrant(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) UpdateGrant(ctx context.Context, req *storageproviderv0alphapb.UpdateGrantRequest) (*storageproviderv0alphapb.UpdateGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.UpdateGrantResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.UpdateGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.UpdateGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.UpdateGrant(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) RemoveGrant(ctx context.Context, req *storageproviderv0alphapb.RemoveGrantRequest) (*storageproviderv0alphapb.RemoveGrantResponse, error) {
	log := appctx.GetLogger(ctx)
	pi, err := s.find(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error finding storage provider")

		if _, ok := err.(notFoundError); ok {
			return &storageproviderv0alphapb.RemoveGrantResponse{
				Status: &rpcpb.Status{
					Code: rpcpb.Code_CODE_NOT_FOUND,
				},
			}, nil
		}

		return &storageproviderv0alphapb.RemoveGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	log.Info().Str("address", pi.Address).Str("ref", req.Ref.String()).Msg("storage provider found")

	// TODO(labkode): check for capabilities here
	c, err := pool.GetStorageProviderServiceClient(pi.Address)
	if err != nil {
		log.Err(err).Msg("gatewaysvc: error getting storage provider client")
		return &storageproviderv0alphapb.RemoveGrantResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	res, err := c.RemoveGrant(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling Stat")
	}

	return res, nil
}

func (s *svc) GetQuota(ctx context.Context, req *storageproviderv0alphapb.GetQuotaRequest) (*storageproviderv0alphapb.GetQuotaResponse, error) {
	res := &storageproviderv0alphapb.GetQuotaResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *svc) find(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storagetypespb.ProviderInfo, error) {
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

	if res.Status.Code == rpcpb.Code_CODE_OK {
		return res.Provider, nil
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		return nil, notFoundError("gatewaysvc: storage provider not found for reference:" + ref.String())
	}

	return nil, errors.New("gatewaysvc: error finding a storage provider")
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func (e notFoundError) IsNotFound()   {}
