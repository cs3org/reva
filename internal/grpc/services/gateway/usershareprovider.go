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

package gateway

import (
	"context"
	"fmt"
	"path"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	storageregv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v1beta1"
	usershareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

// TODO(labkode): add multi-phase commit logic when commit share or commit ref is enabled.
func (s *svc) CreateShare(ctx context.Context, req *usershareproviderv1beta1pb.CreateShareRequest) (*usershareproviderv1beta1pb.CreateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		return &usershareproviderv1beta1pb.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.CreateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateShare")
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		return res, nil
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	if s.c.CommitShareToStorageGrant {
		grantReq := &storageproviderv1beta1pb.AddGrantRequest{
			Ref: &storageproviderv1beta1pb.Reference{
				Spec: &storageproviderv1beta1pb.Reference_Id{
					Id: req.ResourceInfo.Id,
				},
			},
			Grant: &storageproviderv1beta1pb.Grant{
				Grantee:     req.Grant.Grantee,
				Permissions: req.Grant.Permissions.Permissions,
			},
		}

		c, err := s.findByID(ctx, req.ResourceInfo.Id)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &usershareproviderv1beta1pb.CreateShareResponse{
					Status: status.NewNotFound(ctx, "storage provider not found"),
				}, nil
			}
			return &usershareproviderv1beta1pb.CreateShareResponse{
				Status: status.NewInternal(ctx, err, "error finding storage provider"),
			}, nil
		}

		grantRes, err := c.AddGrant(ctx, grantReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling AddGrant")
		}
		if grantRes.Status.Code != rpcpb.Code_CODE_OK {
			res := &usershareproviderv1beta1pb.CreateShareResponse{
				Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
					"error committing share to storage grant"),
			}
			return res, nil
		}
	}

	return res, nil
}

func (s *svc) RemoveShare(ctx context.Context, req *usershareproviderv1beta1pb.RemoveShareRequest) (*usershareproviderv1beta1pb.RemoveShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		return &usershareproviderv1beta1pb.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// if we need to commit the share, we need the resource it points to.
	var share *usershareproviderv1beta1pb.Share
	if s.c.CommitShareToStorageGrant || s.c.CommitShareToStorageRef {
		getShareReq := &usershareproviderv1beta1pb.GetShareRequest{
			Ref: req.Ref,
		}
		getShareRes, err := c.GetShare(ctx, getShareReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling GetShare")
		}

		if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
			res := &usershareproviderv1beta1pb.RemoveShareResponse{
				Status: status.NewInternal(ctx, status.NewErrorFromCode(getShareRes.Status.Code, "gateway"),
					"error getting share when committing to the storage"),
			}
			return res, nil
		}
		share = getShareRes.Share
	}

	res, err := c.RemoveShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveShare")
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	if s.c.CommitShareToStorageGrant {
		grantReq := &storageproviderv1beta1pb.RemoveGrantRequest{
			Ref: &storageproviderv1beta1pb.Reference{
				Spec: &storageproviderv1beta1pb.Reference_Id{
					Id: share.ResourceId,
				},
			},
			Grant: &storageproviderv1beta1pb.Grant{
				Grantee:     share.Grantee,
				Permissions: share.Permissions.Permissions,
			},
		}

		c, err := s.findByID(ctx, share.ResourceId)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &usershareproviderv1beta1pb.RemoveShareResponse{
					Status: status.NewNotFound(ctx, "storage provider not found"),
				}, nil
			}
			return &usershareproviderv1beta1pb.RemoveShareResponse{
				Status: status.NewInternal(ctx, err, "error finding storage provider"),
			}, nil
		}

		grantRes, err := c.RemoveGrant(ctx, grantReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling RemoveGrant")
		}
		if grantRes.Status.Code != rpcpb.Code_CODE_OK {
			return &usershareproviderv1beta1pb.RemoveShareResponse{
				Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
					"error removing storage grant"),
			}, nil
		}
	}

	return res, nil
}

// TODO(labkode): we need to validate share state vs storage grant and storage ref
// If there are any inconsitencies, the share needs to be flag as invalid and a background process
// or active fix needs to be performed.
func (s *svc) GetShare(ctx context.Context, req *usershareproviderv1beta1pb.GetShareRequest) (*usershareproviderv1beta1pb.GetShareResponse, error) {
	return s.getShare(ctx, req)
}

func (s *svc) getShare(ctx context.Context, req *usershareproviderv1beta1pb.GetShareRequest) (*usershareproviderv1beta1pb.GetShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv1beta1pb.GetShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.GetShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetShare")
	}

	return res, nil
}

// TODO(labkode): read GetShare comment.
func (s *svc) ListShares(ctx context.Context, req *usershareproviderv1beta1pb.ListSharesRequest) (*usershareproviderv1beta1pb.ListSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv1beta1pb.ListSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateShare(ctx context.Context, req *usershareproviderv1beta1pb.UpdateShareRequest) (*usershareproviderv1beta1pb.UpdateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv1beta1pb.UpdateShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateShare")
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	/*
		if s.c.CommitShareToStorageGrant {
			getShareReq := &usershareproviderv1beta1pb.GetShareRequest{
				Ref: req.Ref,
			}
			getShareRes, err := c.GetShare(ctx, getShareReq)
			if err != nil {
				return nil, errors.Wrap(err, "gateway: error calling GetShare")
			}

			if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
				return &usershareproviderv1beta1pb.UpdateShareResponse{
					Status: status.NewInternal(ctx, status.NewErrorFromCode(getShareRes.Status.Code, "gateway"),
						"error getting share when committing to the share"),
				}, nil
			}

			grantReq := &storageproviderv1beta1pb.UpdateGrantRequest{
				Ref: &storageproviderv1beta1pb.Reference{
					Spec: &storageproviderv1beta1pb.Reference_Id{
						Id: getShareRes.Share.ResourceId,
					},
				},
				Grant: &storageproviderv1beta1pb.Grant{
					Grantee:     getShareRes.Share.Grantee,
					Permissions: getShareRes.Share.Permissions.Permissions,
				},
			}
				grantRes, err := s.UpdateGrant(ctx, grantReq)
				if err != nil {
					return nil, errors.Wrap(err, "gateway: error calling UpdateGrant")
				}
				if grantRes.Status.Code != rpcpb.Code_CODE_OK {
					return &usershareproviderv1beta1pb.UpdateShareResponse{
						Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
							"error updating storage grant"),
					}, nil
				}
		}
	*/

	return res, nil
}

func (s *svc) ListReceivedShares(ctx context.Context, req *usershareproviderv1beta1pb.ListReceivedSharesRequest) (*usershareproviderv1beta1pb.ListReceivedSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv1beta1pb.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.ListReceivedShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListReceivedShares")
	}

	return res, nil
}

func (s *svc) GetReceivedShare(ctx context.Context, req *usershareproviderv1beta1pb.GetReceivedShareRequest) (*usershareproviderv1beta1pb.GetReceivedShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err := errors.Wrap(err, "gateway: error getting user share provider client")
		return &usershareproviderv1beta1pb.GetReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res, err := c.GetReceivedShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetReceivedShare")
	}

	return res, nil
}

func (s *svc) UpdateReceivedShare(ctx context.Context, req *usershareproviderv1beta1pb.UpdateReceivedShareRequest) (*usershareproviderv1beta1pb.UpdateReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateReceivedShare(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
		return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	if !s.c.CommitShareToStorageRef {
		return res, nil
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		return res, nil
	}

	// we don't commit to storage invalid update fields.
	if req.Field.GetState() == usershareproviderv1beta1pb.ShareState_SHARE_STATE_INVALID && req.Field.GetDisplayName() == "" {
		return res, nil

	}

	// TODO(labkode): if update field is displayName we need to do a rename on the storage to align
	// share display name and storage filename.
	if req.Field.GetState() != usershareproviderv1beta1pb.ShareState_SHARE_STATE_INVALID {
		if req.Field.GetState() == usershareproviderv1beta1pb.ShareState_SHARE_STATE_ACCEPTED {
			// get received share information to obtain the resource it points to.
			getShareReq := &usershareproviderv1beta1pb.GetReceivedShareRequest{Ref: req.Ref}
			getShareRes, err := s.GetReceivedShare(ctx, getShareReq)
			if err != nil {
				log.Err(err).Msg("gateway: error calling GetReceivedShare")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
				log.Error().Msg("gateway: error calling GetReceivedShare")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			share := getShareRes.Share

			// get user home
			storageRegClient, err := pool.GetStorageRegistryClient(s.c.StorageRegistryEndpoint)
			if err != nil {
				log.Err(err).Msg("gateway: error getting storage registry client")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			homeReq := &storageregv1beta1pb.GetHomeRequest{}
			homeRes, err := storageRegClient.GetHome(ctx, homeReq)
			if err != nil {
				err := errors.Wrap(err, "gateway: error calling GetHome")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error updating received share"),
				}, nil
			}

			// reference path is the home path + some name
			refPath := path.Join(homeRes.Path, req.Ref.String()) // TODO(labkode): the name of the share should be the filename it points to by default.
			createRefReq := &storageproviderv1beta1pb.CreateReferenceRequest{
				Path:      refPath,
				TargetUri: fmt.Sprintf("cs3:%s/%s", share.Share.ResourceId.GetStorageId(), share.Share.ResourceId.GetOpaqueId()),
			}

			c, err := s.findByPath(ctx, refPath)
			if err != nil {
				if _, ok := err.(errtypes.IsNotFound); ok {
					return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
						Status: status.NewNotFound(ctx, "storage provider not found"),
					}, nil
				}
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error finding storage provider"),
				}, nil
			}

			createRefRes, err := c.CreateReference(ctx, createRefReq)
			if err != nil {
				log.Err(err).Msg("gateway: error calling GetHome")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			if createRefRes.Status.Code != rpcpb.Code_CODE_OK {
				err := status.NewErrorFromCode(createRefRes.Status.GetCode(), "gateway")
				return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error updating received share"),
				}, nil
			}

			return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
				Status: status.NewOK(ctx),
			}, nil
		}
	}

	// TODO(labkode): implementing updating display name
	err = errors.New("gateway: update of display name is not yet implemented")
	return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
		Status: status.NewUnimplemented(ctx, err, "error updaring received share"),
	}, nil
}
