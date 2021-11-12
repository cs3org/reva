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
	"fmt"
	"strings"

	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	gstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	revactx "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

//go:generate mockery -name GatewayClient -name SharesProviderClient

// GatewayClient describe the interface of a gateway client
type GatewayClient interface {
	Stat(ctx context.Context, in *provider.StatRequest, opts ...grpc.CallOption) (*provider.StatResponse, error)
	Move(ctx context.Context, in *provider.MoveRequest, opts ...grpc.CallOption) (*provider.MoveResponse, error)
	Delete(ctx context.Context, in *provider.DeleteRequest, opts ...grpc.CallOption) (*provider.DeleteResponse, error)
	CreateContainer(ctx context.Context, in *provider.CreateContainerRequest, opts ...grpc.CallOption) (*provider.CreateContainerResponse, error)
	ListContainer(ctx context.Context, in *provider.ListContainerRequest, opts ...grpc.CallOption) (*provider.ListContainerResponse, error)
	ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest, opts ...grpc.CallOption) (*provider.ListFileVersionsResponse, error)
	RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest, opts ...grpc.CallOption) (*provider.RestoreFileVersionResponse, error)
	InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest, opts ...grpc.CallOption) (*gateway.InitiateFileDownloadResponse, error)
	InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest, opts ...grpc.CallOption) (*gateway.InitiateFileUploadResponse, error)
	SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest, opts ...grpc.CallOption) (*provider.SetArbitraryMetadataResponse, error)
	UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest, opts ...grpc.CallOption) (*provider.UnsetArbitraryMetadataResponse, error)
}

// SharesProviderClient provides methods for listing and modifying received shares
type SharesProviderClient interface {
	ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest, opts ...grpc.CallOption) (*collaboration.ListReceivedSharesResponse, error)
	UpdateReceivedShare(ctx context.Context, req *collaboration.UpdateReceivedShareRequest, opts ...grpc.CallOption) (*collaboration.UpdateReceivedShareResponse, error)
}

func init() {
	rgrpc.Register("sharesstorageprovider", NewDefault)
}

type config struct {
	GatewayAddr               string `mapstructure:"gateway_addr"`
	UserShareProviderEndpoint string `mapstructure:"usershareprovidersvc"`
}

type service struct {
	gateway              GatewayClient
	sharesProviderClient SharesProviderClient
}
type stattedReceivedShare struct {
	Stat              *provider.ResourceInfo
	ReceivedShare     *collaboration.ReceivedShare
	AllReceivedShares []*collaboration.ReceivedShare
}

type shareNotFoundError struct {
	name string
}

func (e *shareNotFoundError) Error() string {
	return "Unknown share:" + e.name
}

func isShareNotFoundError(e error) bool {
	_, ok := e.(*shareNotFoundError)
	return ok
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

// NewDefault returns a new instance of the SharesStorageProvider service with default dependencies
func NewDefault(m map[string]interface{}, _ *grpc.Server) (rgrpc.Service, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	// TODO use
	gateway, err := pool.GetGatewayServiceClient(sharedconf.GetGatewaySVC(c.GatewayAddr))
	if err != nil {
		return nil, err
	}

	client, err := pool.GetUserShareProviderClient(sharedconf.GetGatewaySVC(c.UserShareProviderEndpoint))
	if err != nil {
		return nil, errors.Wrap(err, "sharesstorageprovider: error getting UserShareProvider client")
	}

	return New(gateway, client)
}

// New returns a new instance of the SharesStorageProvider service
func New(gateway GatewayClient, c SharesProviderClient) (rgrpc.Service, error) {
	s := &service{
		gateway:              gateway,
		sharesProviderClient: c,
	}
	return s, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got SetArbitraryMetadata request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.SetArbitraryMetadataResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadata: req.ArbitraryMetadata,
	})
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got UnsetArbitraryMetadata request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.UnsetArbitraryMetadataResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.UnsetArbitraryMetadata(ctx, &provider.UnsetArbitraryMetadataRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	if utils.IsRelativeReference(req.Ref) {
		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
		if err != nil {
			return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		for _, rs := range lsRes.Shares {
			// match the opaque id
			if utils.ResourceIDEqual(rs.Share.ResourceId, req.Ref.ResourceId) {
				gwres, err := s.gateway.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
					Ref: &provider.Reference{
						ResourceId: req.Ref.ResourceId,
						Path:       req.Ref.Path,
					},
					Opaque: req.Opaque,
				})
				if err != nil {
					return nil, err
				}
				if gwres.Status.Code != rpc.Code_CODE_OK {
					return &provider.InitiateFileDownloadResponse{
						Status: gwres.Status,
					}, nil
				}

				protocols := []*provider.FileDownloadProtocol{}
				for p := range gwres.Protocols {
					if !strings.HasSuffix(gwres.Protocols[p].DownloadEndpoint, "/") {
						gwres.Protocols[p].DownloadEndpoint += "/"
					}
					gwres.Protocols[p].DownloadEndpoint += gwres.Protocols[p].Token

					protocols = append(protocols, &provider.FileDownloadProtocol{
						Opaque:           gwres.Protocols[p].Opaque,
						Protocol:         gwres.Protocols[p].Protocol,
						DownloadEndpoint: gwres.Protocols[p].DownloadEndpoint,
						Expose:           true, // the gateway already has encoded the upload endpoint
					})
				}

				return &provider.InitiateFileDownloadResponse{
					Status:    gwres.Status,
					Protocols: protocols,
				}, nil
			}
		}
		return &provider.InitiateFileDownloadResponse{
			Status: status.NewNotFound(ctx, "sharesstorageprovider: not found "+req.Ref.String()),
		}, nil
	}

	return &provider.InitiateFileDownloadResponse{
		Status: status.NewInvalidArg(ctx, "sharesstorageprovider: can only handle relative references"),
	}, nil

}

func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	if utils.IsRelativeReference(req.Ref) { // FIXME also allow absolute?
		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
		if err != nil {
			return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		for _, rs := range lsRes.Shares {
			// match the opaqueid
			if utils.ResourceIDEqual(rs.Share.ResourceId, req.Ref.ResourceId) {
				// use the resource id from the share, it contains the real spaceid and opaqueid
				gwres, err := s.gateway.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
					Ref: &provider.Reference{
						ResourceId: req.Ref.ResourceId,
						Path:       req.Ref.Path,
					},
					Opaque: req.Opaque,
				})
				if err != nil {
					return nil, err
				}
				if gwres.Status.Code != rpc.Code_CODE_OK {
					return &provider.InitiateFileUploadResponse{
						Status: gwres.Status,
					}, nil
				}

				protocols := []*provider.FileUploadProtocol{}
				for p := range gwres.Protocols {
					if !strings.HasSuffix(gwres.Protocols[p].UploadEndpoint, "/") {
						gwres.Protocols[p].UploadEndpoint += "/"
					}
					gwres.Protocols[p].UploadEndpoint += gwres.Protocols[p].Token

					protocols = append(protocols, &provider.FileUploadProtocol{
						Opaque:             gwres.Protocols[p].Opaque,
						Protocol:           gwres.Protocols[p].Protocol,
						UploadEndpoint:     gwres.Protocols[p].UploadEndpoint,
						AvailableChecksums: gwres.Protocols[p].AvailableChecksums,
						Expose:             true, // the gateway already has encoded the upload endpoint
					})
				}
				return &provider.InitiateFileUploadResponse{
					Status:    gwres.Status,
					Protocols: protocols,
				}, nil
			}
		}
		return &provider.InitiateFileUploadResponse{
			Status: status.NewNotFound(ctx, "sharesstorageprovider: not found "+req.Ref.String()),
		}, nil
	}

	return &provider.InitiateFileUploadResponse{
		Status: status.NewInvalidArg(ctx, "sharesstorageprovider: can only handle relative references"),
	}, nil
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

// ListStorageSpaces ruturns a list storage spaces with type share. However, when the space registry tries
// to find a storage provider for a specific space it returns an empty list, so the actual storage provider
// should be found.
func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
	}
	if lsRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
	}

	res := &provider.ListStorageSpacesResponse{}
	for i := range lsRes.Shares {
		space := &provider.StorageSpace{
			Id: &provider.StorageSpaceId{
				// Do we need a unique spaceid for every share?
				// we are going to use the opaque id of the resource as the spaceid
				OpaqueId: lsRes.Shares[i].Share.ResourceId.OpaqueId,
			},
			SpaceType: "share",
			Owner:     &userv1beta1.User{Id: lsRes.Shares[i].Share.Owner},
			// return the actual resource id
			Root: lsRes.Shares[i].Share.ResourceId,
		}
		if lsRes.Shares[i].MountPoint != nil {
			space.Name = lsRes.Shares[i].MountPoint.Path
		}

		info, st, err := s.statResource(ctx, lsRes.Shares[i].Share.ResourceId, "")
		if err != nil {
			return nil, err
		}
		if st.Code != rpc.Code_CODE_OK {
			continue
		}
		space.Mtime = info.Mtime

		// what if we don't have a name?
		res.StorageSpaces = append(res.StorageSpaces, space)
	}
	res.Status = status.NewOK(ctx)

	return res, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got CreateContainer request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.CreateContainerResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.CreateContainer(ctx, &provider.CreateContainerRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got Delete request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.DeleteResponse{
			Status: rpcStatus,
		}, nil
	}
	// TODO unshare?
	return s.gateway.Delete(ctx, &provider.DeleteRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {

	return &provider.MoveResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_UNIMPLEMENTED},
	}, nil
	// TODO moving inside a shared tree should just be a forward of the move
	//      but when do we rename a mounted share? Does that request even hit us?
	//      - the registry needs to invalidate the alias
	//      - the rhe share manager needs to change the name
	//      ... but which storageprovider will receive the move request???
	/*
		srcResource, srcPath, err := s.resolveReference(ctx, req.Source)
		dstResource, destinationPath, err2 := s.resolveReference(ctx, req.Destination)
		if err != nil {
			appctx.GetLogger(ctx).Debug().
				Interface("srcRef", req.Source).
				Interface("srcRes", srcResource).
				Interface("reqDest", req.Destination).
				Err(err).
				Msg("sharesstorageprovider: Got Move request")
			return nil, err
		}
		if err2 != nil {
			appctx.GetLogger(ctx).Debug().
				Interface("reqRef", req.Source).
				Interface("reqDest", req.Destination).
				Err(err2).
				Msg("sharesstorageprovider: Got Move request")
			return nil, err2
		}
		appctx.GetLogger(ctx).Debug().
			Str("reqPath", reqPath).
			Str("reqShare", reqShare).
			Str("destinationPath", destinationPath).
			Str("destinationShare", destinationShare).
			Err(err).
			Msg("sharesstorageprovider: Got Move request")

		stattedShare, err := s.statShare(ctx, reqShare)
		if err != nil {
			if isShareNotFoundError(err) {
				return &provider.MoveResponse{
					Status: status.NewNotFound(ctx, "sharesstorageprovider: file not found"),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "sharesstorageprovider: error stating share"),
			}, nil
		}

		if reqShare != destinationShare && reqPath == "" {
			// Change the MountPoint of the share
			stattedShare.ReceivedShare.MountPoint = &provider.Reference{Path: destinationShare}

			_, err = s.sharesProviderClient.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
				Share:      stattedShare.ReceivedShare,
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state", "mount_point"}},
			})
			if err != nil {
				return &provider.MoveResponse{
					Status: status.NewInternal(ctx, err, "sharesstorageprovider: can not change mountpoint of share"),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewOK(ctx),
			}, nil
		}

		dstStattedShare, err := s.statShare(ctx, destinationShare)
		if err != nil {
			if isShareNotFoundError(err) {
				return &provider.MoveResponse{
					Status: status.NewNotFound(ctx, "sharesstorageprovider: file not found"),
				}, nil
			}
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "sharesstorageprovider: error stating share"),
			}, nil
		}

		if stattedShare.Stat.Id.StorageId != dstStattedShare.Stat.Id.StorageId {
			return &provider.MoveResponse{
				Status: status.NewInvalid(ctx, "sharesstorageprovider: can not move between shares on different storages"),
			}, nil
		}

		gwres, err := s.gateway.Move(ctx, &provider.MoveRequest{
			Source: &provider.Reference{
				Path: filepath.Join(stattedShare.Stat.Path, reqPath),
			},
			Destination: &provider.Reference{
				Path: filepath.Join(dstStattedShare.Stat.Path, destinationPath),
			},
		})

		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, err, "sharesstorageprovider: error calling Move"),
			}, nil
		}

		if gwres.Status.Code != rpc.Code_CODE_OK {
			return &provider.MoveResponse{
				Status: gwres.Status,
			}, nil
		}

		return gwres, nil
	*/
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got Stat request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.StatResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.Stat(ctx, &provider.StatRequest{
		Opaque: req.Opaque,
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got Stat request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.ListContainerResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.ListContainer(ctx, &provider.ListContainerRequest{
		Opaque: req.Opaque,
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}
func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Msg("sharesstorageprovider: Got ListFileVersions request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.ListFileVersionsResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.ListFileVersions(ctx, &provider.ListFileVersionsRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	resource, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("res", resource).
		Err(err).
		Msg("sharesstorageprovider: Got RestoreFileVersion request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.RestoreFileVersionResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.RestoreFileVersion(ctx, &provider.RestoreFileVersionRequest{
		Ref: &provider.Reference{
			ResourceId: resource,
			Path:       req.Ref.Path,
		},
	})
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

func (s *service) DenyGrant(ctx context.Context, ref *provider.DenyGrantRequest) (*provider.DenyGrantResponse, error) {
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

// GetQuota returns 0 free quota. It is virtual ... the shares may have a different quota ...
func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	// FIXME use req.Ref to get real quota
	return &provider.GetQuotaResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) resolveReference(ctx context.Context, ref *provider.Reference) (*provider.ResourceId, *rpc.Status, error) {
	// treat absolute id based references as relative ones
	if ref.Path == "" {
		ref.Path = "."
	}
	if utils.IsRelativeReference(ref) {
		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
		if err != nil {
			return nil, nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		for _, rs := range lsRes.Shares {
			// match the opaque id
			if utils.ResourceIDEqual(rs.Share.ResourceId, ref.ResourceId) {
				return rs.Share.ResourceId, nil, nil
			}
		}
		return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
	}

	return nil, status.NewInvalidArg(ctx, "sharesstorageprovider: can only handle relative references"), nil
}

func (s *service) statShare(ctx context.Context, share string) (*stattedReceivedShare, error) {
	_, ok := revactx.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("sharesstorageprovider: shares requested for empty user")
	}

	shares, err := s.getReceivedShares(ctx)
	if err != nil {
		return nil, fmt.Errorf("sharesstorageprovider: error getting received shares")
	}
	stattedShare, ok := shares[share]
	if !ok {
		for _, ss := range shares {
			if ss.ReceivedShare.MountPoint != nil && ss.ReceivedShare.MountPoint.Path == share {
				stattedShare, ok = ss, true
			}
		}
	}
	if !ok {
		return nil, &shareNotFoundError{name: share}
	}
	return stattedShare, nil
}

func (s *service) getReceivedShares(ctx context.Context) (map[string]*stattedReceivedShare, error) {
	ret := map[string]*stattedReceivedShare{}
	lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
	}
	if lsRes.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
	}
	appctx.GetLogger(ctx).Debug().
		Interface("lsRes.Shares", lsRes.Shares).
		Msg("sharesstorageprovider: Preparing statted share")

	for _, rs := range lsRes.Shares {
		if rs.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
			continue
		}

		info, st, err := s.statResource(ctx, rs.Share.ResourceId, "")
		if err != nil || st.Code != rpc.Code_CODE_OK {
			appctx.GetLogger(ctx).Debug().
				Interface("info", info).
				Interface("state", st).
				Err(err).
				Msg("sharesstorageprovider: skipping statted share")
			continue
		}

		name := rs.GetMountPoint().GetPath()
		if _, ok := ret[name]; !ok {
			ret[name] = &stattedReceivedShare{
				ReceivedShare:     rs,
				AllReceivedShares: []*collaboration.ReceivedShare{rs},
				Stat:              info,
			}
			ret[name].Stat.PermissionSet = rs.Share.Permissions.Permissions
		} else {
			ret[name].Stat.PermissionSet = s.mergePermissions(ret[name].Stat.PermissionSet, rs.Share.Permissions.Permissions)
			ret[name].AllReceivedShares = append(ret[name].AllReceivedShares, rs)
		}
	}

	appctx.GetLogger(ctx).Debug().
		Interface("ret", ret).
		Msg("sharesstorageprovider: Returning statted share")
	return ret, nil
}

func (s *service) statResource(ctx context.Context, res *provider.ResourceId, path string) (*provider.ResourceInfo, *rpc.Status, error) {
	statRes, err := s.gateway.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: res,
			Path:       path,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return statRes.Info, statRes.Status, nil
}
func (s *service) listResource(ctx context.Context, res *provider.ResourceId, path string) ([]*provider.ResourceInfo, *rpc.Status, error) {
	listRes, err := s.gateway.ListContainer(ctx, &provider.ListContainerRequest{
		Ref: &provider.Reference{
			ResourceId: res,
			Path:       path,
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return listRes.Infos, listRes.Status, nil
}

func (s *service) rejectReceivedShare(ctx context.Context, share string) error {
	stattedShare, err := s.statShare(ctx, share)
	if err != nil {
		return err
	}

	stattedShare.ReceivedShare.State = collaboration.ShareState_SHARE_STATE_REJECTED

	_, err = s.sharesProviderClient.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
		Share:      stattedShare.ReceivedShare,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	return err
}

func (s *service) mergePermissions(a, b *provider.ResourcePermissions) *provider.ResourcePermissions {
	a.AddGrant = a.AddGrant || b.AddGrant
	a.CreateContainer = a.CreateContainer || b.CreateContainer
	a.Delete = a.Delete || b.Delete
	a.GetPath = a.GetPath || b.GetPath
	a.GetQuota = a.GetQuota || b.GetQuota
	a.InitiateFileDownload = a.InitiateFileDownload || b.InitiateFileDownload
	a.InitiateFileUpload = a.InitiateFileUpload || b.InitiateFileUpload
	a.ListGrants = a.ListGrants || b.ListGrants
	a.ListContainer = a.ListContainer || b.ListContainer
	a.ListFileVersions = a.ListFileVersions || b.ListFileVersions
	a.ListRecycle = a.ListRecycle || b.ListRecycle
	a.Move = a.Move || b.Move
	a.RemoveGrant = a.RemoveGrant || b.RemoveGrant
	a.PurgeRecycle = a.PurgeRecycle || b.PurgeRecycle
	a.RestoreFileVersion = a.RestoreFileVersion || b.RestoreFileVersion
	a.RestoreRecycleItem = a.RestoreRecycleItem || b.RestoreRecycleItem
	a.Stat = a.Stat || b.Stat
	a.UpdateGrant = a.UpdateGrant || b.UpdateGrant
	return a
}
