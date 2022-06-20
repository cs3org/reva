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

	"github.com/cs3org/reva/v2/pkg/share"
	"github.com/cs3org/reva/v2/pkg/storagespace"
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
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/rgrpc"
	"github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/sharedconf"
	"github.com/cs3org/reva/v2/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	rgrpc.Register("sharesstorageprovider", NewDefault)
}

type config struct {
	GatewayAddr               string `mapstructure:"gateway_addr"`
	UserShareProviderEndpoint string `mapstructure:"usershareprovidersvc"`
}

type service struct {
	gateway              gateway.GatewayAPIClient
	sharesProviderClient collaboration.CollaborationAPIClient
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
func New(gateway gateway.GatewayAPIClient, c collaboration.CollaborationAPIClient) (rgrpc.Service, error) {
	s := &service{
		gateway:              gateway,
		sharesProviderClient: c,
	}
	return s, nil
}

func (s *service) SetArbitraryMetadata(ctx context.Context, req *provider.SetArbitraryMetadataRequest) (*provider.SetArbitraryMetadataResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got SetArbitraryMetadata request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.SetArbitraryMetadataResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.SetArbitraryMetadata(ctx, &provider.SetArbitraryMetadataRequest{
		Opaque:            req.Opaque,
		Ref:               buildReferenceInShare(req.Ref, receivedShare),
		ArbitraryMetadata: req.ArbitraryMetadata,
	})
}

func (s *service) UnsetArbitraryMetadata(ctx context.Context, req *provider.UnsetArbitraryMetadataRequest) (*provider.UnsetArbitraryMetadataResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got UnsetArbitraryMetadata request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.UnsetArbitraryMetadataResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.UnsetArbitraryMetadata(ctx, &provider.UnsetArbitraryMetadataRequest{
		Opaque:                req.Opaque,
		Ref:                   buildReferenceInShare(req.Ref, receivedShare),
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}

func (s *service) InitiateFileDownload(ctx context.Context, req *provider.InitiateFileDownloadRequest) (*provider.InitiateFileDownloadResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got InitiateFileDownload request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.InitiateFileDownloadResponse{
			Status: rpcStatus,
		}, nil
	}

	gwres, err := s.gateway.InitiateFileDownload(ctx, &provider.InitiateFileDownloadRequest{
		Opaque: req.Opaque,
		Ref:    buildReferenceInShare(req.Ref, receivedShare),
		LockId: req.LockId,
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
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got InitiateFileUpload request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.InitiateFileUploadResponse{
			Status: rpcStatus,
		}, nil
	}
	gwres, err := s.gateway.InitiateFileUpload(ctx, &provider.InitiateFileUploadRequest{
		Opaque:  req.Opaque,
		Ref:     buildReferenceInShare(req.Ref, receivedShare),
		LockId:  req.LockId,
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
	// TODO: Needs to find a path for a given resourceID
	// It should
	// - getPath of the resourceID - probably requires owner permissions -> needs machine auth
	// - getPath of every received share on the same space - needs also owner permissions -> needs machine auth
	// - find the shortest root path that is a prefix of the resource path
	// alternatively implement this on storageprovider - it needs to know about grants to do so
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
	for i, f := range req.Filters {
		if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
			_, id := storagespace.SplitStorageID(f.GetId().GetOpaqueId())
			req.Filters[i].Term = &provider.ListStorageSpacesRequest_Filter_Id{Id: &provider.StorageSpaceId{OpaqueId: id}}
			break
		}
	}

	spaceTypes := map[string]struct{}{}
	var exists = struct{}{}
	var fetchShares bool
	appendTypes := []string{}
	var spaceID *provider.ResourceId
	for _, f := range req.Filters {
		switch f.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			spaceType := f.GetSpaceType()
			// do we need to fetch the shares?
			if spaceType == "+mountpoint" || spaceType == "+grant" {
				appendTypes = append(appendTypes, strings.TrimPrefix(spaceType, "+"))
				fetchShares = true
				continue
			}
			if spaceType == "mountpoint" || spaceType == "grant" {
				fetchShares = true
			}
			spaceTypes[spaceType] = exists
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
			spaceid, shareid, err := storagespace.SplitID(f.GetId().OpaqueId)
			if err != nil {
				continue
			}
			if spaceid != utils.ShareStorageProviderID {
				return &provider.ListStorageSpacesResponse{
					// a specific id was requested, return not found instead of empty list
					Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND},
				}, nil
			}

			spaceID = &provider.ResourceId{StorageId: spaceid, OpaqueId: shareid}
		}
	}

	if len(spaceTypes) == 0 {
		spaceTypes["virtual"] = exists
		spaceTypes["mountpoint"] = exists
		fetchShares = true
	}

	for _, s := range appendTypes {
		spaceTypes[s] = exists
	}

	var receivedShares []*collaboration.ReceivedShare
	var shareMd map[string]share.Metadata
	var err error
	if fetchShares {
		receivedShares, shareMd, err = s.fetchShares(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}
	}

	res := &provider.ListStorageSpacesResponse{
		Status: status.NewOK(ctx),
	}
	for k := range spaceTypes {
		switch k {
		case "virtual":
			virtualRootID := &provider.ResourceId{
				StorageId: storagespace.FormatStorageID(utils.ShareStorageProviderID, utils.ShareStorageSpaceID),
				OpaqueId:  utils.ShareStorageProviderID,
			}
			if spaceID == nil || isShareJailRoot(spaceID) {
				earliestShare, atLeastOneAccepted := findEarliestShare(receivedShares, shareMd)
				var opaque *typesv1beta1.Opaque
				var mtime *typesv1beta1.Timestamp
				if earliestShare != nil {
					if md, ok := shareMd[earliestShare.Id.OpaqueId]; ok {
						mtime = md.Mtime
						opaque = utils.AppendPlainToOpaque(opaque, "etag", md.ETag)
					}
				}
				// only display the shares jail if we have accepted shares
				if atLeastOneAccepted {
					opaque = utils.AppendPlainToOpaque(opaque, "spaceAlias", "virtual/shares")
					space := &provider.StorageSpace{
						Opaque: opaque,
						Id: &provider.StorageSpaceId{
							OpaqueId: storagespace.FormatResourceID(*virtualRootID),
						},
						SpaceType: "virtual",
						//Owner:     &userv1beta1.User{Id: receivedShare.Share.Owner}, // FIXME actually, the mount point belongs to the recipient
						// the sharesstorageprovider keeps track of mount points
						Root:  virtualRootID,
						Name:  "Shares Jail",
						Mtime: mtime,
					}
					res.StorageSpaces = append(res.StorageSpaces, space)
				}
			}
		case "grant":
			for _, receivedShare := range receivedShares {
				root := receivedShare.Share.ResourceId
				// do we filter by id?
				if spaceID != nil && !utils.ResourceIDEqual(spaceID, root) {
					// none of our business
					continue
				}
				var opaque *typesv1beta1.Opaque
				if md, ok := shareMd[receivedShare.Share.Id.OpaqueId]; ok {
					opaque = utils.AppendPlainToOpaque(opaque, "etag", md.ETag)
				}
				// we know a grant for this resource
				space := &provider.StorageSpace{
					Opaque: opaque,
					Id: &provider.StorageSpaceId{
						OpaqueId: storagespace.FormatResourceID(*root),
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
				var opaque *typesv1beta1.Opaque
				if md, ok := shareMd[receivedShare.Share.Id.OpaqueId]; ok {
					opaque = utils.AppendPlainToOpaque(opaque, "etag", md.ETag)
				} else {
					// we could not stat the share, skip it
					continue
				}
				// add the resourceID for the grant
				if receivedShare.Share.ResourceId != nil {
					opaque = utils.AppendPlainToOpaque(opaque, "grantStorageID", receivedShare.Share.ResourceId.StorageId)
					opaque = utils.AppendPlainToOpaque(opaque, "grantOpaqueID", receivedShare.Share.ResourceId.OpaqueId)
				}

				// prefix storageid if we are responsible
				if root.StorageId == utils.ShareStorageSpaceID {
					root.StorageId = storagespace.FormatStorageID(utils.ShareStorageProviderID, root.StorageId)
				}

				space := &provider.StorageSpace{
					Opaque: opaque,
					Id: &provider.StorageSpaceId{
						OpaqueId: storagespace.FormatResourceID(*root),
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
					space.Opaque = utils.AppendPlainToOpaque(space.Opaque, "spaceAlias", space.SpaceType+"/"+strings.ReplaceAll(strings.ToLower(space.Name), " ", "-"))
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
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Msg("sharesstorageprovider: Got CreateContainer request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.CreateContainerResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.CreateContainer(ctx, &provider.CreateContainerRequest{
		Opaque: req.Opaque,
		Ref:    buildReferenceInShare(req.Ref, receivedShare),
	})
}

func (s *service) Delete(ctx context.Context, req *provider.DeleteRequest) (*provider.DeleteResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got Delete request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
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
		Opaque: req.Opaque,
		Ref:    buildReferenceInShare(req.Ref, receivedShare),
	})
}

func (s *service) Move(ctx context.Context, req *provider.MoveRequest) (*provider.MoveResponse, error) {
	if req.Source.GetResourceId() != nil {
		_, req.Source.ResourceId.StorageId = storagespace.SplitStorageID(req.Source.ResourceId.StorageId)
	}
	if req.Destination.GetResourceId() != nil {
		_, req.Destination.ResourceId.StorageId = storagespace.SplitStorageID(req.Destination.ResourceId.StorageId)
	}

	appctx.GetLogger(ctx).Debug().
		Interface("source", req.Source).
		Interface("destination", req.Destination).
		Msg("sharesstorageprovider: Got Move request")

	// TODO moving inside a shared tree should just be a forward of the move
	//      but when do we rename a mounted share? Does that request even hit us?
	//      - the registry needs to invalidate the alias
	//      - the rhe share manager needs to change the name
	//      ... but which storageprovider will receive the move request???
	srcReceivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Source)
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.MoveResponse{
			Status: rpcStatus,
		}, nil
	}

	// we can do a rename
	if isRename(req.Source, req.Destination) {

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

	dstReceivedShare, rpcStatus, err2 := s.resolveAcceptedShare(ctx, req.Destination)
	if err2 != nil {
		return nil, err2
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
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
		Opaque:      req.Opaque,
		Source:      buildReferenceInShare(req.Source, srcReceivedShare),
		Destination: buildReferenceInShare(req.Destination, dstReceivedShare),
	})
}

// SetLock puts a lock on the given reference
func (s *service) SetLock(ctx context.Context, req *provider.SetLockRequest) (*provider.SetLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// GetLock returns an existing lock on the given reference
func (s *service) GetLock(ctx context.Context, req *provider.GetLockRequest) (*provider.GetLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// RefreshLock refreshes an existing lock on the given reference
func (s *service) RefreshLock(ctx context.Context, req *provider.RefreshLockRequest) (*provider.RefreshLockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// Unlock removes an existing lock from the given reference
func (s *service) Unlock(ctx context.Context, req *provider.UnlockRequest) (*provider.UnlockResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

func (s *service) Stat(ctx context.Context, req *provider.StatRequest) (*provider.StatResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	if isVirtualRoot(req.Ref) {
		receivedShares, shareMd, err := s.fetchShares(ctx)
		if err != nil {
			return nil, err
		}
		earliestShare, _ := findEarliestShare(receivedShares, shareMd)
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
				Mtime:         shareMd[earliestShare.Id.OpaqueId].Mtime,
				Path:          "/",
				MimeType:      "httpd/unix-directory",
				Size:          0,
				PermissionSet: &provider.ResourcePermissions{
					// TODO
				},
				Etag: shareMd[earliestShare.Id.OpaqueId].ETag,
			},
		}, nil
	}
	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got Stat request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
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

	// TODO return reference?
	return s.gateway.Stat(ctx, &provider.StatRequest{
		Opaque:                req.Opaque,
		Ref:                   buildReferenceInShare(req.Ref, receivedShare),
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})

	// FIXME when stating a share jail child we need to rewrite the id and use the share
	// jail space id as the mountpoint has a different id than the grant
	// but that might be problematic for eg. wopi because it needs the correct id? ...
	// ... but that should stat the grant anyway

	// FIXME when navigating via /dav/spaces/a0ca6a90-a365-4782-871e-d44447bbc668 the web ui seems
	// to continue navigating based on the id of resources, causing the path to change. Is that related to WOPI?

}

func (s *service) ListContainerStream(req *provider.ListContainerStreamRequest, ss provider.ProviderAPI_ListContainerStreamServer) error {
	return gstatus.Errorf(codes.Unimplemented, "method not implemented")
}
func (s *service) ListContainer(ctx context.Context, req *provider.ListContainerRequest) (*provider.ListContainerResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	if isVirtualRoot(req.Ref) {
		// The root is empty, it is filled by mountpoints
		// so, when accessing the root via /dav/spaces, we need to list the accepted shares with their mountpoint

		receivedShares, _, err := s.fetchShares(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
		}

		infos := []*provider.ResourceInfo{}
		for _, share := range receivedShares {
			if share.GetState() != collaboration.ShareState_SHARE_STATE_ACCEPTED {
				continue
			}

			statRes, err := s.gateway.Stat(ctx, &provider.StatRequest{
				Opaque: req.Opaque,
				Ref: &provider.Reference{
					ResourceId: share.Share.ResourceId,
					Path:       ".",
				},
				ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
			})
			switch {
			case err != nil:
				appctx.GetLogger(ctx).Error().
					Err(err).
					Interface("share", share).
					Msg("sharesstorageprovider: could not make stat request when listing virtual root, skipping")
				continue
			case statRes.Status.Code != rpc.Code_CODE_OK:
				appctx.GetLogger(ctx).Debug().
					Interface("share", share).
					Interface("status", statRes.Status).
					Msg("sharesstorageprovider: could not stat share when listing virtual root, skipping")
				continue
			}

			// override info
			info := statRes.Info
			info.Id = &provider.ResourceId{
				StorageId: utils.ShareStorageProviderID,
				OpaqueId:  share.Share.Id.OpaqueId,
			}
			info.Path = filepath.Base(share.MountPoint.Path)

			infos = append(infos, info)
		}
		return &provider.ListContainerResponse{
			Status: status.NewOK(ctx),
			Infos:  infos,
		}, nil
	}
	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got ListContainer request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.ListContainerResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.ListContainer(ctx, &provider.ListContainerRequest{
		Opaque:                req.Opaque,
		Ref:                   buildReferenceInShare(req.Ref, receivedShare),
		ArbitraryMetadataKeys: req.ArbitraryMetadataKeys,
	})
}
func (s *service) ListFileVersions(ctx context.Context, req *provider.ListFileVersionsRequest) (*provider.ListFileVersionsResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got ListFileVersions request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.ListFileVersionsResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.ListFileVersions(ctx, &provider.ListFileVersionsRequest{
		Opaque: req.Opaque,
		Ref:    buildReferenceInShare(req.Ref, receivedShare),
	})
}

func (s *service) RestoreFileVersion(ctx context.Context, req *provider.RestoreFileVersionRequest) (*provider.RestoreFileVersionResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	receivedShare, rpcStatus, err := s.resolveAcceptedShare(ctx, req.Ref)
	appctx.GetLogger(ctx).Debug().
		Interface("ref", req.Ref).
		Interface("received_share", receivedShare).
		Err(err).
		Msg("sharesstorageprovider: Got RestoreFileVersion request")
	if err != nil {
		return nil, err
	}
	if rpcStatus.Code != rpc.Code_CODE_OK {
		return &provider.RestoreFileVersionResponse{
			Status: rpcStatus,
		}, nil
	}

	return s.gateway.RestoreFileVersion(ctx, &provider.RestoreFileVersionRequest{
		Opaque: req.Opaque,
		Ref:    buildReferenceInShare(req.Ref, receivedShare),
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

func (s *service) TouchFile(ctx context.Context, req *provider.TouchFileRequest) (*provider.TouchFileResponse, error) {
	return nil, gstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// GetQuota returns 0 free quota. It is virtual ... the shares may have a different quota ...
func (s *service) GetQuota(ctx context.Context, req *provider.GetQuotaRequest) (*provider.GetQuotaResponse, error) {
	if req.Ref.GetResourceId() != nil {
		_, req.Ref.ResourceId.StorageId = storagespace.SplitStorageID(req.Ref.ResourceId.StorageId)
	}

	// FIXME use req.Ref to get real quota
	return &provider.GetQuotaResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) resolveAcceptedShare(ctx context.Context, ref *provider.Reference) (*collaboration.ReceivedShare, *rpc.Status, error) {
	// treat absolute id based references as relative ones
	if ref.Path == "" {
		ref.Path = "."
	}
	if !utils.IsRelativeReference(ref) {
		return nil, status.NewInvalidArg(ctx, "sharesstorageprovider: can only handle relative references"), nil
	}

	if ref.ResourceId.StorageId != utils.ShareStorageProviderID {
		return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
	}

	// we can get the share if the reference carries a share id
	if ref.ResourceId.OpaqueId != utils.ShareStorageProviderID {
		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{
			Ref: &collaboration.ShareReference{
				Spec: &collaboration.ShareReference_Id{
					Id: &collaboration.ShareId{
						OpaqueId: ref.ResourceId.OpaqueId,
					},
				},
			},
		})

		if err != nil {
			return nil, nil, errors.Wrap(err, "sharesstorageprovider: error calling GetReceivedShare")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, lsRes.Status, nil
		}
		if lsRes.Share.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
			return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
		}
		return lsRes.Share, lsRes.Status, nil
	}

	// we currently need to list all shares and match the path if the request is relative to the share jail root
	if ref.ResourceId.OpaqueId == utils.ShareStorageProviderID && ref.Path != "." {
		// we need to list accepted shares and match the path

		// look up share for this resourceid
		lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{
			Filters: []*collaboration.Filter{
				// FIXME filter by accepted ... and by mountpoint?
			},
		})
		if err != nil {
			return nil, nil, errors.Wrap(err, "sharesstorageprovider: error calling GetReceivedShare")
		}
		if lsRes.Status.Code != rpc.Code_CODE_OK {
			return nil, lsRes.Status, nil
		}
		for _, receivedShare := range lsRes.Shares {
			if receivedShare.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
				continue
			}
			if strings.HasPrefix(strings.TrimPrefix(ref.Path, "./"), receivedShare.MountPoint.Path) {
				return receivedShare, lsRes.Status, nil
			}
		}

	}

	return nil, status.NewNotFound(ctx, "sharesstorageprovider: not found "+ref.String()), nil
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

func (s *service) fetchShares(ctx context.Context) ([]*collaboration.ReceivedShare, map[string]share.Metadata, error) {
	lsRes, err := s.sharesProviderClient.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{
		// FIXME filter by received shares for resource id - listing all shares is tooo expensive!
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "sharesstorageprovider: error calling ListReceivedSharesRequest")
	}
	if lsRes.Status.Code != rpc.Code_CODE_OK {
		return nil, nil, fmt.Errorf("sharesstorageprovider: error calling ListReceivedSharesRequest")
	}

	shareMetaData := make(map[string]share.Metadata, len(lsRes.Shares))
	for _, rs := range lsRes.Shares {
		// only stat accepted shares
		if rs.State != collaboration.ShareState_SHARE_STATE_ACCEPTED {
			continue
		}
		sRes, err := s.gateway.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: rs.Share.ResourceId}})
		if err != nil {
			appctx.GetLogger(ctx).Error().
				Err(err).
				Interface("resourceID", rs.Share.ResourceId).
				Msg("ListRecievedShares: failed to make stat call")
			continue
		}
		if sRes.Status.Code != rpc.Code_CODE_OK {
			appctx.GetLogger(ctx).Debug().
				Interface("resourceID", rs.Share.ResourceId).
				Interface("status", sRes.Status).
				Msg("ListRecievedShares: failed to stat the resource")
			continue
		}
		shareMetaData[rs.Share.Id.OpaqueId] = share.Metadata{ETag: sRes.Info.Etag, Mtime: sRes.Info.Mtime}
	}

	return lsRes.Shares, shareMetaData, nil
}

func findEarliestShare(receivedShares []*collaboration.ReceivedShare, shareMd map[string]share.Metadata) (earliestShare *collaboration.Share, atLeastOneAccepted bool) {
	for _, rs := range receivedShares {
		var hasCurrentMd bool
		var hasEarliestMd bool

		current := rs.Share
		if rs.State == collaboration.ShareState_SHARE_STATE_ACCEPTED {
			atLeastOneAccepted = true
		}

		// We cannot assume that every share has metadata
		if current.Id != nil {
			_, hasCurrentMd = shareMd[current.Id.OpaqueId]
		}
		if earliestShare != nil && earliestShare.Id != nil {
			_, hasEarliestMd = shareMd[earliestShare.Id.OpaqueId]
		}

		switch {
		case earliestShare == nil:
			earliestShare = current
		// ignore if one of the shares has no metadata
		case !hasEarliestMd || !hasCurrentMd:
			continue
		case shareMd[current.Id.OpaqueId].Mtime.Seconds > shareMd[earliestShare.Id.OpaqueId].Mtime.Seconds:
			earliestShare = current
		case shareMd[current.Id.OpaqueId].Mtime.Seconds == shareMd[earliestShare.Id.OpaqueId].Mtime.Seconds &&
			shareMd[current.Id.OpaqueId].Mtime.Nanos > shareMd[earliestShare.Id.OpaqueId].Mtime.Nanos:
			earliestShare = current
		}
	}
	return earliestShare, atLeastOneAccepted
}

func buildReferenceInShare(ref *provider.Reference, s *collaboration.ReceivedShare) *provider.Reference {
	path := ref.Path
	if isShareJailRoot(ref.ResourceId) {
		// we need to cut off the mountpoint from the path in the request reference
		path = utils.MakeRelativePath(strings.TrimPrefix(strings.TrimPrefix(path, "./"), s.MountPoint.Path))
	}
	return &provider.Reference{
		ResourceId: s.Share.ResourceId,
		Path:       path,
	}
}

// isRename checks if the two references lie in the responsibility of the sharesstorageprovider and if a rename occurs
func isRename(s, d *provider.Reference) bool {
	// if the source is a share jail child where the path is .
	return ((isShareJailChild(s.ResourceId) && s.Path == ".") ||
		// or if the source is the share jail with a single path segment, e.g. './old'
		(isShareJailRoot(s.ResourceId) && len(strings.SplitN(s.Path, "/", 3)) == 2)) &&
		// and if the destination is the share jail a single path segment, e.g. './new'
		isShareJailRoot(d.ResourceId) && len(strings.SplitN(d.Path, "/", 3)) == 2
}

func isShareJailChild(id *provider.ResourceId) bool {
	_, space := storagespace.SplitStorageID(id.StorageId)
	return space == utils.ShareStorageProviderID && id.OpaqueId != utils.ShareStorageProviderID
}

func isShareJailRoot(id *provider.ResourceId) bool {
	_, space := storagespace.SplitStorageID(id.StorageId)
	return space == utils.ShareStorageProviderID && id.OpaqueId == utils.ShareStorageProviderID
}

func isVirtualRoot(ref *provider.Reference) bool {
	return isShareJailRoot(ref.ResourceId) && (ref.Path == "" || ref.Path == "." || ref.Path == "./")
}
