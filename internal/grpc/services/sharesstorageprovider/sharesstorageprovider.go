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

package sharesstorageprovider

import (
	"context"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

//go:generate mockery -name GatewayClient
type GatewayClient interface {
	Stat(ctx context.Context, in *provider.StatRequest, opts ...grpc.CallOption) (*provider.StatResponse, error)
	ListContainer(ctx context.Context, in *provider.ListContainerRequest, opts ...grpc.CallOption) (*provider.ListContainerResponse, error)
}

func init() {
	rgrpc.Register("sharesstorageprovider", NewDefault)
}

type config struct {
	MountPath   string `mapstructure:"mount_path"`
	GatewayAddr string `mapstructure:"gateway_addr"`

	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	mountPath string
	sm        share.Manager
	gateway   GatewayClient
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterProviderAPIServer(ss, s)
}

func NewDefault(m map[string]interface{}, _ *grpc.Server) (rgrpc.Service, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	smFactory, found := registry.NewFuncs[c.Driver]
	if !found {
		return nil, errtypes.NotFound("driver not found: " + c.Driver)
	}
	sm, err := smFactory(c.Drivers[c.Driver])
	if err != nil {
		return nil, err
	}

	gateway, err := pool.GetGatewayServiceClient(c.GatewayAddr)
	if err != nil {
		return nil, err
	}

	return New(c.MountPath, gateway, sm)
}

func New(mountpath string, gateway GatewayClient, sm share.Manager) (rgrpc.Service, error) {
	s := &service{
		mountPath: mountpath,
		gateway:   gateway,
		sm:        sm,
	}
	return s, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}
func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetPath(ctx context.Context, req *provider.GetPathRequest) (*provider.GetPathResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetHome(ctx context.Context, req *provider.GetHomeRequest) (*provider.GetHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateHome(ctx context.Context, req *provider.CreateHomeRequest) (*provider.CreateHomeResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	reqUser, reqShare, reqPath := resolvePath(req.Ref.GetPath())
	appctx.GetLogger(ctx).Debug().
		Interface("reqPath", reqPath).
		Interface("reqShare", reqShare).
		Interface("reqUser", reqUser).
		Msg("sharesstorageprovider.Stat: Got Stat request")

	shares, err := s.sm.ListReceivedShares(ctx)
	if err != nil {
		return &provider.StatResponse{
			Status: status.NewInternal(ctx, err, "sharestorageprovider: error listing received shares"),
		}, nil
	}

	res := &provider.StatResponse{
		Info: &provider.ResourceInfo{
			Path: filepath.Join(s.mountPath, reqUser),
			Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		},
	}
	for _, rs := range shares {
		if rs.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
			continue
		}

		gwres, err := s.gateway.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Id{
					Id: rs.Share.ResourceId,
				},
			},
		})
		if err != nil {
			return &provider.StatResponse{
				Status: status.NewInternal(ctx, err, "sharestorageprovider: error getting stat from gateway"),
			}, nil
		}

		if reqShare != "" && filepath.Base(gwres.Info.Path) == reqShare {
			if reqPath != "" {
				gwres, err = s.gateway.Stat(ctx, &provider.StatRequest{
					Ref: &provider.Reference{
						Spec: &provider.Reference_Path{
							Path: filepath.Join(gwres.Info.Path, reqPath),
						},
					},
				})
				if err != nil {
					return &provider.StatResponse{
						Status: status.NewInternal(ctx, err, "sharestorageprovider: error getting stat from gateway"),
					}, nil
				}
			}

			relPath := strings.SplitAfterN(gwres.Info.Path, reqShare, 2)[1]
			gwres.Info.Path = filepath.Join(s.mountPath, reqUser, reqShare, relPath)
			return gwres, nil
		} else if reqShare == "" {
			res.Info.Size += gwres.Info.Size
		}
	}

	res.Status = status.NewOK(ctx)
	return res, nil
}
func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	reqUser, reqShare, reqPath := resolvePath(req.Ref.GetPath())
	appctx.GetLogger(ctx).Debug().
		Interface("reqPath", reqPath).
		Interface("reqShare", reqShare).
		Interface("reqUser", reqUser).
		Msg("sharesstorageprovider.ListContainer: Got ListContainer request")

	// contextUser, ok := ctxuser.ContextGetUser(ctx)
	// if !ok {
	// 	return &provider.ListContainerResponse{
	// 		Status: status.NewInternal(ctx, nil, "error retrieving current user"),
	// 	}, nil
	// }

	shares, err := s.sm.ListReceivedShares(ctx)
	if err != nil {
		return &provider.ListContainerResponse{
			Status: status.NewInternal(ctx, err, "sharestorageprovider: error listing received shares"),
		}, nil
	}

	res := &provider.ListContainerResponse{}
	for _, rs := range shares {
		if rs.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
			continue
		}

		gwres, err := s.gateway.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Id{
					Id: rs.Share.ResourceId,
				},
			},
		})
		if err != nil {
			return &provider.ListContainerResponse{
				Status: status.NewInternal(ctx, err, "sharestorageprovider: error getting stats from gateway"),
			}, nil
		}

		if reqShare != "" && filepath.Base(gwres.Info.Path) == reqShare {
			gwListRes, err := s.gateway.ListContainer(ctx, &provider.ListContainerRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Path{Path: filepath.Join(filepath.Dir(gwres.Info.Path), reqShare, reqPath)},
				},
			})
			if err != nil {
				return &provider.ListContainerResponse{
					Status: status.NewInternal(ctx, err, "sharestorageprovider: error getting listing from gateway"),
				}, nil
			}
			for _, info := range gwListRes.Infos {
				relPath := strings.SplitAfterN(info.Path, reqShare, 2)[1]
				info.Path = filepath.Join(s.mountPath, reqUser, reqShare, relPath)
			}
			return gwListRes, nil
		} else if reqShare == "" {
			gwres.Info.Path = filepath.Join(s.mountPath, reqUser, filepath.Base(gwres.Info.Path))
			res.Infos = append(res.Infos, gwres.Info)
		}
	}
	res.Status = status.NewOK(ctx)

	return res, nil
}
func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycleStream(req *provider.ListRecycleStreamRequest, ss provider.ProviderAPI_ListRecycleStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListRecycle(ctx context.Context, req *provider.ListRecycleRequest) (*provider.ListRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RestoreRecycleItem(ctx context.Context, req *provider.RestoreRecycleItemRequest) (*provider.RestoreRecycleItemResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) PurgeRecycle(ctx context.Context, req *provider.PurgeRecycleRequest) (*provider.PurgeRecycleResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListGrants(ctx context.Context, req *provider.ListGrantsRequest) (*provider.ListGrantsResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) AddGrant(ctx context.Context, req *provider.AddGrantRequest) (*provider.AddGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateReference(ctx context.Context, req *provider.CreateReferenceRequest) (*provider.CreateReferenceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateSymlink(ctx context.Context, req *provider.CreateSymlinkRequest) (*provider.CreateSymlinkResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) UpdateGrant(ctx context.Context, req *provider.UpdateGrantRequest) (*provider.UpdateGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) RemoveGrant(ctx context.Context, req *provider.RemoveGrantRequest) (*provider.RemoveGrantResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func resolvePath(path string) (string, string, string) {
	// /shares/<username>/foo/bar
	parts := strings.SplitN(strings.TrimLeft(path, "/"), "/", 4)
	var reqUser, reqShare, reqPath string
	if len(parts) >= 4 {
		reqPath = parts[3]
	}
	if len(parts) >= 3 {
		reqShare = parts[2]
	}
	if len(parts) >= 2 {
		reqUser = parts[1]
	}
	return reqUser, reqShare, reqPath
}
