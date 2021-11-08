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

package spacesprovider

import (
	"context"
	"path/filepath"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	rgrpc.Register("spacesprovider", New)
}

type config struct {
	GatewayAddr string `mapstructure:"gateway_addr"`
}

func (c *config) init() {

}

type service struct {
	conf    *config
	gateway gateway.GatewayAPIClient
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string { return []string{} }

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new storage provider svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	gateway, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		gateway: gateway,
	}

	return service, nil
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	// 1. lazily fetch spaces for the user from gateway / registry
	// the requests is path based, so we need to find the correct alias in the sea of spaces
	// examples:
	//   /         should list? all different types of spaces. TODO aliases for types? allow space types with slashes, eg 'eos/users?'
	//   /personal should list? this is the type, list all spaces of this type
	//   /personal/einstein should list? -> just forward to correct storage provider with relative reference: spaceid+relative path
	//   /shares/someshare should list? -> forward to correct storage provider with relative reference: spaceid+relative path
	//   /projects/atlas
	//   /projects/foobar

	// what should the data structure for this look like?
	// is a flat kv store good enough?
	// 1. lazily fetch spaces for the user from gateway / registry
	lSSRes, err := s.gateway.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return nil, err
	}
	if lSSRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.StatResponse{
			Status: lSSRes.Status,
		}, nil
	}
	// TODO cache the result

	// collect all types
	spaces := map[string][]*provider.StorageSpace{}
	for i := range lSSRes.StorageSpaces {
		_, ok := spaces[lSSRes.StorageSpaces[i].SpaceType]
		if !ok {
			spaces[lSSRes.StorageSpaces[i].SpaceType] = []*provider.StorageSpace{}
		}
		spaces[lSSRes.StorageSpaces[i].SpaceType] = append(spaces[lSSRes.StorageSpaces[i].SpaceType], lSSRes.StorageSpaces[i])
	}

	return &provider.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info: &provider.ResourceInfo{
			Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			Path:  "/spaces",
			Mtime: &typesv1beta1.Timestamp{},
		},
	}, nil

}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	// 1. lazily fetch spaces for the user from gateway / registry
	lSSRes, err := s.gateway.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return nil, err
	}
	if lSSRes.Status.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: lSSRes.Status,
		}, nil
	}
	// TODO cache the result

	// collect all types
	spaces := map[string][]*provider.StorageSpace{}
	for i := range lSSRes.StorageSpaces {
		_, ok := spaces[lSSRes.StorageSpaces[i].SpaceType]
		if !ok {
			spaces[lSSRes.StorageSpaces[i].SpaceType] = []*provider.StorageSpace{}
		}
		spaces[lSSRes.StorageSpaces[i].SpaceType] = append(spaces[lSSRes.StorageSpaces[i].SpaceType], lSSRes.StorageSpaces[i])
	}

	infos := make([]*provider.ResourceInfo, len(spaces))
	for t := range spaces {
		infos = append(infos, &provider.ResourceInfo{
			Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
			//Id: ?,
			Path: filepath.Join("spaces", t),
			// Etag: ?,
			Mtime: &typesv1beta1.Timestamp{},
		})
	}
	// TODO ... we need to aggregate the root metadata for a list of spaces, because a type can have multiple spaces

	return &provider.ListContainerResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Infos:  infos,
	}, nil
}

func (s *service) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycleStream(req *provider.ListRecycleStreamRequest, ss provider.ProviderAPI_ListRecycleStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) AddGrant(ctx context.Context, req *provider.AddGrantRequest) (*provider.AddGrantResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DenyGrant(ctx context.Context, req *provider.DenyGrantRequest) (*provider.DenyGrantResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateReference(ctx context.Context, req *provider.CreateReferenceRequest) (*provider.CreateReferenceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateGrant(ctx context.Context, req *provider.UpdateGrantRequest) (*provider.UpdateGrantResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RemoveGrant(ctx context.Context, req *provider.RemoveGrantRequest) (*provider.RemoveGrantResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}
func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}
