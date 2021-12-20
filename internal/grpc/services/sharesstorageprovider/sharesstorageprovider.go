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
	"path/filepath"
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
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
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
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
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
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadata: req.ArbitraryMetadata,
	})
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
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
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got InitiateFileDownload request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.InitiateFileDownloadResponse{
			Status: rpcStatus,
		}, nil
	}
	gwres, err := s.gateway.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
		Ref: &provider.Reference{
			ResourceId: receivedShare.Share.ResourceId,
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

func (s *service) InitiateFileUpload(ctx context.Context, req *provider.InitiateFileUploadRequest) (*provider.InitiateFileUploadResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got InitiateFileUpload request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.InitiateFileUploadResponse{
			Status: rpcStatus,
		}, nil
	}
	gwres, err := s.gateway.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Opaque: req.Opaque,
		Ref: &provider.Reference{
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
		Options: req.Options,
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

// ListStorageSpaces returns a list storage spaces with type "share" the current user has acces to.
// Do owners of shares see type "shared"? Do they see andyhing? They need to if the want a fast lookup of shared with others
// -> but then a storage sprovider has to do everything? not everything but permissions (= shares) related operations, yes
// The root node of every storag space is the (spaceid, nodeid) of the shared node.
// Since real space roots have (spaceid=nodeid) shares can be correlated with the space using the (spaceid, ) part of the reference.

// However, when the space registry tries
// to find a storage provider for a specific space it returns an empty list, so the actual storage provider
// should be found.

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {

	spaceTypes := []string{}
	res := &provider.ListStorageSpacesResponse{
		Status: status.NewOK(ctx),
	}
	var fetchShares bool
	appendTypes := []string{}
	var spaceID *provider.ResourceId
	for _, f := range req.Filters {
		switch f.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			spaceType := f.GetSpaceType()
			// do we need to fetch the shares?
			if spaceType == "mountpoint" || spaceType == "grant" {
				spaceTypes = append(spaceTypes, spaceType)
				fetchShares = true
			}
			if spaceType == "+mountpoint" || spaceType == "+grant" {
				appendTypes = append(appendTypes, strings.TrimPrefix(spaceType, "+"))
				fetchShares = true
			}
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			spaceID = &provider.ResourceId{}
			spaceID.StorageId, spaceID.OpaqueId = utils.SplitStorageSpaceID(f.GetId().OpaqueId)
			if spaceID.StorageId == "" || spaceID.OpaqueId == "" {
				res.Status = status.NewInvalid(ctx, "invalid space id")
				return res, nil
			}
		}
	}

	if len(spaceTypes) == 0 {
		spaceTypes = []string{"virtual"}
		fetchShares = true
	}

	spaceTypes = append(spaceTypes, appendTypes...)

	var receivedShares []*collaboration.ReceivedShare
	if fetchShares {
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{
			// FIXME filter by received shares for resource id - listing all shares is tooo expensive!
		})
		if err != nil {
			return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		receivedShares = lsRes.Shares
	}

	for i := range spaceTypes {
		switch spaceTypes[i] {
		case "virtual":
			virtualRootID := &provider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  utils.ShareStorageProviderID,
			}
			if spaceID == nil || utils.ResourceIDEqual(virtualRootID, spaceID) {

				space := &provider.StorageSpace{
					Id: &provider.StorageSpaceId{
						OpaqueId: virtualRootID.StorageId + "!" + virtualRootID.OpaqueId,
					},
					SpaceType: "virtual",
					//Owner:     &userv1beta1.User{Id: receivedShare.Share.Owner}, // FIXME actually, the mount point belongs to the recipient
					// the sharesstorageprovider keeps track of mount points
					Root: virtualRootID,
					Name: "Shares Jail",
				}
				res.StorageSpaces = append(res.StorageSpaces, space)
			}
		case "grant":
			for _, receivedShare := range receivedShares {
				root := receivedShare.Share.ResourceId
				// do we filter by id?
				if spaceID != nil && !utils.ResourceIDEqual(spaceID, root) {
					// none of our business
					continue
				}
				// we know a grant for this resource
				space := &provider.StorageSpace{
					Id: &provider.StorageSpaceId{
						OpaqueId: root.StorageId + "!" + root.OpaqueId,
					},
					SpaceType: "grant",
					Owner:     &userv1beta1.User{Id: receivedShare.Share.Owner},
					// the sharesstorageprovider keeps track of mount points
					Root: root,
				}

				res.StorageSpaces = append(res.StorageSpaces, space)
			}
		case "mountpoint":
			for _, receivedShare := range receivedShares {
				if receivedShare.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
					continue
				}
				root := &provider.ResourceId{
					StorageId: utils.ShareStorageProviderID,
					OpaqueId:  receivedShare.Share.Id.OpaqueId,
					//OpaqueId: utils.ShareStorageProviderID,
				}
				// do we filter by id
				if spaceID != nil {
					switch {
					case utils.ResourceIDEqual(spaceID, root):
						// we have a virtual node
					case utils.ResourceIDEqual(spaceID, receivedShare.Share.ResourceId):
						// we have a mount point
						root = receivedShare.Share.ResourceId
					default:
						// none of our business
						continue
					}
				}
				space := &provider.StorageSpace{
					Id: &provider.StorageSpaceId{
						OpaqueId: root.StorageId + "!" + root.OpaqueId,
					},
					SpaceType: "mountpoint",
					Owner:     &userv1beta1.User{Id: receivedShare.Share.Owner}, // FIXME actually, the mount point belongs to the recipient
					// the sharesstorageprovider keeps track of mount points
					Root: root,
				}

				// TODO in the future the spaces registry will handle the alias for share spaces.
				// for now use the name from the share to override the name determined by stat
				if receivedShare.MountPoint != nil {
					space.Name = receivedShare.MountPoint.Path
				}

				// what if we don't have a name?
				res.StorageSpaces = append(res.StorageSpaces, space)
			}
		}
	}
	return res, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) CreateContainer(ctx context.Context, req *provider.CreateContainerRequest) (*provider.CreateContainerResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
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
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got Delete request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.DeleteResponse{
			Status: rpcStatus,
		}, nil
	}

	// the root of a share always has the path "."
	if req.Ref.ResourceId.StorageId == utils.ShareStorageProviderID && req.Ref.Path == "." {
		err := s.rejectReceivedShare(ctx, receivedShare)
		if err != nil {
			return &provider.DeleteResponse{
				Status: status.NewInternal(ctx, "sharesstorageprovider: error rejecting share"),
			}, nil
		}
		return &provider.DeleteResponse{
			Status: status.NewOK(ctx),
		}, nil
	}

	return s.gateway.Delete(ctx, &provider.DeleteRequest{
		Ref: &provider.Reference{
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {

	appctx.GetLogger(ctx).Debug().
		Interface("source", req.Source).
		Interface("destination", req.Destination).
		Msg("sharesstorageprovider: Got Move request")

	// TODO moving inside a shared tree should just be a forward of the move
	//      but when do we rename a mounted share? Does that request even hit us?
	//      - the registry needs to invalidate the alias
	//      - the rhe share manager needs to change the name
	//      ... but which storageprovider will receive the move request???
	srcReceivedShare, rpcStatus, err := s.resolveReference(ctx, req.Source)
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.MoveResponse{
			Status: rpcStatus,
		}, nil
	}

	// can we do a rename
	if utils.ResourceIDEqual(req.Source.ResourceId, req.Destination.ResourceId) &&
		// only if we are responsible for the space
		req.Source.ResourceId.StorageId == utils.ShareStorageProviderID &&
		// only if the source path has no path segment
		req.Source.Path == "." &&
		// only if the destination is a dot followed by a single path segment, e.g. './new'
		len(strings.SplitN(req.Destination.Path, "/", 3)) == 2 {

		// Change the MountPoint of the share, it has no relative prefix
		srcReceivedShare.MountPoint = &provider.Reference{
			// FIXME actually it does have a resource id: the one of the sharesstorageprovider
			Path: filepath.Base(req.Destination.Path),
		}

		_, err = s.sharesProviderClient.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
			Share:      srcReceivedShare,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state", "mount_point"}},
		})
		if err != nil {
			return &provider.MoveResponse{
				Status: status.NewInternal(ctx, "sharesstorageprovider: can not change mountpoint of share"),
			}, nil
		}
		return &provider.MoveResponse{
			Status: status.NewOK(ctx),
		}, nil
	}

	dstReceivedShare, rpcStatus, err2 := s.resolveReference(ctx, req.Destination)
	if err2 != nil {
		return nil, err2
	}
	if rpcStatus != nil {
		return &provider.MoveResponse{
			Status: rpcStatus,
		}, nil
	}
	if srcReceivedShare.Share.ResourceId.StorageId != dstReceivedShare.Share.ResourceId.StorageId {
		return &provider.MoveResponse{
			Status: status.NewInvalid(ctx, "sharesstorageprovider: can not move between shares on different storages"),
		}, nil
	}

	return s.gateway.Move(ctx, &provider.MoveRequest{
		Source: &provider.Reference{
			ResourceId: srcReceivedShare.Share.ResourceId,
			Path:       req.Source.Path,
		},
		Destination: &provider.Reference{
			ResourceId: dstReceivedShare.Share.ResourceId,
			Path:       req.Destination.Path,
		},
	})
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	if isVirtualRoot(req.Ref.ResourceId) {
		if req.Ref.Path == "" || req.Ref.Path == "." {
			// The root is empty, it is filled by mountpoints
			return &provider.StatResponse{
				Status: status.NewOK(ctx),
				Info: &provider.ResourceInfo{
					Opaque: &typesv1beta1.Opaque{
						Map: map[string]*typesv1beta1.OpaqueEntry{
							"root": {
								Decoder: "plain",
								Value:   []byte(utils.ShareStorageProviderID),
							},
						},
					},
					Id: &provider.ResourceId{
						StorageId: utils.ShareStorageProviderID,
						OpaqueId:  utils.ShareStorageProviderID,
					},
					Type:          provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Mtime:         &typesv1beta1.Timestamp{},
					Path:          "/",
					MimeType:      "httpd/unix-directory",
					Size:          0,
					PermissionSet: &provider.ResourcePermissions{
						// TODO
					},
				},
			}, nil
		}
		// we need to check if a child with that name exists
	}
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got Stat request")
	if err != nil {
		return nil, err
	}
	if rpcStatus != nil {
		return &provider.StatResponse{
			Status: rpcStatus,
		}, nil
	}
	if receivedShare.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
		return &provider.StatResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND},
			// not mounted yet
		}, nil
	}

	return &provider.StatResponse{
		Status: status.NewOK(ctx),
		Info: &provider.ResourceInfo{
			Type: provider.ResourceType_RESOURCE_TYPE_REFERENCE,
			Id: &provider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  receivedShare.Share.Id.OpaqueId,
			},
			Path:   req.Ref.Path,
			Target: "cs3:" + receivedShare.Share.ResourceId.StorageId + "/" + receivedShare.Share.ResourceId.OpaqueId,
		},
	}, nil
	/*
		sRes, err := s.gateway.Stat(ctx, &provider.StatRequest{
			Opaque: req.Opaque,
			Ref: &provider.Reference{
				ResourceId: receivedShare.Share.ResourceId,
				Path:       req.Ref.Path, // TODO can path ever be something else than ""?
			},
			ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
		})

		if err == nil && sRes.Status.Code == rpc.Code_CODE_OK {
			if sRes.Info.Opaque == nil {
				sRes.Info.Opaque = &typesv1beta1.Opaque{
					Map: map[string]*typesv1beta1.OpaqueEntry{},
				}
			} else if sRes.Info.Opaque.Map == nil {
				sRes.Info.Opaque.Map = map[string]*typesv1beta1.OpaqueEntry{}
			}
			// set root to the sharesstorageprovider
			sRes.Info.Opaque.Map["root"] = &typesv1beta1.OpaqueEntry{
				Decoder: "plain",
				Value:   []byte(utils.ShareStorageProviderID),
			}
			// overwrite id to make subsequent stat calls use the mount point
			// of the sharesstorageprovider to build absolute paths
			sRes.Info.Id = &provider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  receivedShare.Share.Id.OpaqueId,
			}
		}

		return sRes, err
	*/
}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func isVirtualRoot(id *provider.ResourceId) bool {
	return utils.ResourceIDEqual(id, &provider.ResourceId{
		StorageId: utils.ShareStorageProviderID,
		OpaqueId:  utils.ShareStorageProviderID,
	})
}

func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	if isVirtualRoot(req.Ref.ResourceId) {
		// The root is empty, it is filled by mountpoints
		return &provider.ListContainerResponse{
			Status: status.NewOK(ctx),
			Infos:  []*provider.ResourceInfo{},
		}, nil
	}
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got ListContainer request")
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
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}
func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
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
			ResourceId: receivedShare.Share.ResourceId,
			Path:       req.Ref.Path,
		},
	})
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	receivedShare, rpcStatus, err := s.resolveReference(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
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
			ResourceId: receivedShare.Share.ResourceId,
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

func (s *service) resolveReference(ctx context.Context, ref *provider.Reference) (*collaboration.ReceivedShare, *rpc.Status, error) {
	// treat absolute id based references as relative ones
	if ref.Path == "" {
		ref.Path = "."
	}
	if utils.IsRelativeReference(ref) {
		if ref.ResourceId.StorageId != utils.ShareStorageProviderID {
			return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
		}
		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{
			// FIXME filter by received shares for resource id - listing all shares is tooo expensive!
		})
		if err != nil {
			return nil, nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
		for _, receivedShare := range lsRes.Shares {
			if receivedShare.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
				continue
			}
			root := &provider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  receivedShare.Share.Id.OpaqueId,
			}
			switch {
			case utils.ResourceIDEqual(ref.ResourceId, root):
				// we have a virtual node
				return receivedShare, nil, nil
			case utils.ResourceIDEqual(ref.ResourceId, receivedShare.Share.ResourceId):
				// we have a mount point
				return receivedShare, nil, nil
			default:
				continue
			}
		}
		return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
	}

	return nil, status.NewInvalidArg(ctx, "sharesstorageprovider: can only handle relative references"), nil
}

func (s *service) rejectReceivedShare(ctx context.Context, receivedShare *collaboration.ReceivedShare) error {
	receivedShare.State = collaboration.ShareState_SHARE_STATE_REJECTED
	receivedShare.MountPoint = nil

	res, err := s.sharesProviderClient.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
		Share:      receivedShare,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state", "mount_point"}},
	})
	if err != nil {
		return err
	}

	return errtypes.NewErrtypeFromStatus(res.Status)
}
