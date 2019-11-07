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
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	storageregv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

// TODO(labkode): add multi-phase commit logic when commit share or commit ref is enabled.
func (s *svc) CreateShare(ctx context.Context, req *usershareproviderv0alphapb.CreateShareRequest) (*usershareproviderv0alphapb.CreateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		return &usershareproviderv0alphapb.CreateShareResponse{
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
		grantReq := &storageproviderv0alphapb.AddGrantRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Id{
					Id: req.ResourceInfo.Id,
				},
			},
			Grant: &storageproviderv0alphapb.Grant{
				Grantee:     req.Grant.Grantee,
				Permissions: req.Grant.Permissions.Permissions,
			},
		}

		c, err := s.findByID(ctx, req.ResourceInfo.Id)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &usershareproviderv0alphapb.CreateShareResponse{
					Status: status.NewNotFound(ctx, "storage provider not found"),
				}, nil
			}
			return &usershareproviderv0alphapb.CreateShareResponse{
				Status: status.NewInternal(ctx, err, "error finding storage provider"),
			}, nil
		}

		grantRes, err := c.AddGrant(ctx, grantReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling AddGrant")
		}
		if grantRes.Status.Code != rpcpb.Code_CODE_OK {
			res := &usershareproviderv0alphapb.CreateShareResponse{
				Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
					"error committing share to storage grant"),
			}
			return res, nil
		}
	}

	return res, nil
}

func (s *svc) RemoveShare(ctx context.Context, req *usershareproviderv0alphapb.RemoveShareRequest) (*usershareproviderv0alphapb.RemoveShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		return &usershareproviderv0alphapb.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// if we need to commit the share, we need the resource it points to.
	var share *usershareproviderv0alphapb.Share
	if s.c.CommitShareToStorageGrant || s.c.CommitShareToStorageRef {
		getShareReq := &usershareproviderv0alphapb.GetShareRequest{
			Ref: req.Ref,
		}
		getShareRes, err := c.GetShare(ctx, getShareReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling GetShare")
		}

		if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
			res := &usershareproviderv0alphapb.RemoveShareResponse{
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
		grantReq := &storageproviderv0alphapb.RemoveGrantRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Id{
					Id: share.ResourceId,
				},
			},
			Grant: &storageproviderv0alphapb.Grant{
				Grantee:     share.Grantee,
				Permissions: share.Permissions.Permissions,
			},
		}

		c, err := s.findByID(ctx, share.ResourceId)
		if err != nil {
			if _, ok := err.(errtypes.IsNotFound); ok {
				return &usershareproviderv0alphapb.RemoveShareResponse{
					Status: status.NewNotFound(ctx, "storage provider not found"),
				}, nil
			}
			return &usershareproviderv0alphapb.RemoveShareResponse{
				Status: status.NewInternal(ctx, err, "error finding storage provider"),
			}, nil
		}

		grantRes, err := c.RemoveGrant(ctx, grantReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling RemoveGrant")
		}
		if grantRes.Status.Code != rpcpb.Code_CODE_OK {
			return &usershareproviderv0alphapb.RemoveShareResponse{
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
func (s *svc) GetShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	return s.getShare(ctx, req)
}

func (s *svc) getShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv0alphapb.GetShareResponse{
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
func (s *svc) ListShares(ctx context.Context, req *usershareproviderv0alphapb.ListSharesRequest) (*usershareproviderv0alphapb.ListSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv0alphapb.ListSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateShareRequest) (*usershareproviderv0alphapb.UpdateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv0alphapb.UpdateShareResponse{
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
			getShareReq := &usershareproviderv0alphapb.GetShareRequest{
				Ref: req.Ref,
			}
			getShareRes, err := c.GetShare(ctx, getShareReq)
			if err != nil {
				return nil, errors.Wrap(err, "gateway: error calling GetShare")
			}

			if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
				return &usershareproviderv0alphapb.UpdateShareResponse{
					Status: status.NewInternal(ctx, status.NewErrorFromCode(getShareRes.Status.Code, "gateway"),
						"error getting share when committing to the share"),
				}, nil
			}

			grantReq := &storageproviderv0alphapb.UpdateGrantRequest{
				Ref: &storageproviderv0alphapb.Reference{
					Spec: &storageproviderv0alphapb.Reference_Id{
						Id: getShareRes.Share.ResourceId,
					},
				},
				Grant: &storageproviderv0alphapb.Grant{
					Grantee:     getShareRes.Share.Grantee,
					Permissions: getShareRes.Share.Permissions.Permissions,
				},
			}
				grantRes, err := s.UpdateGrant(ctx, grantReq)
				if err != nil {
					return nil, errors.Wrap(err, "gateway: error calling UpdateGrant")
				}
				if grantRes.Status.Code != rpcpb.Code_CODE_OK {
					return &usershareproviderv0alphapb.UpdateShareResponse{
						Status: status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
							"error updating storage grant"),
					}, nil
				}
		}
	*/

	return res, nil
}

// TODO(labkode): listing received shares just goes to the user share manager and gets the list of
// received shares. The display name of the shares should be the a friendly name, like the basename
// of the original file.
func (s *svc) ListReceivedShares(ctx context.Context, req *usershareproviderv0alphapb.ListReceivedSharesRequest) (*usershareproviderv0alphapb.ListReceivedSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv0alphapb.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.ListReceivedShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListReceivedShares")
	}
	return res, nil
}

func (s *svc) GetReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.GetReceivedShareRequest) (*usershareproviderv0alphapb.GetReceivedShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err := errors.Wrap(err, "gateway: error getting user share provider client")
		return &usershareproviderv0alphapb.GetReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res, err := c.GetReceivedShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetReceivedShare")
	}

	return res, nil
}

// When updated a received share:
// if the update contains update for displayName:
//   1) if receives share is mounted: we also do a rename in the storage
//   2) if received share is not mounted: we only rename in user share provider.
func (s *svc) UpdateReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateReceivedShareRequest) (*usershareproviderv0alphapb.UpdateReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateReceivedShare(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
		return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
			Status: &rpcpb.Status{
				Code: rpcpb.Code_CODE_INTERNAL,
			},
		}, nil
	}

	// error failing to update share state.
	if res.Status.Code != rpcpb.Code_CODE_OK {
		return res, nil
	}

	// if we don't need to create/delete references then we return early.
	if !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// we don't commit to storage invalid update fields or empty display names.
	if req.Field.GetState() == usershareproviderv0alphapb.ShareState_SHARE_STATE_INVALID && req.Field.GetDisplayName() == "" {
		log.Error().Msg("the update field is invalid, aborting reference manipulation")
		return res, nil

	}

	// TODO(labkode): if update field is displayName we need to do a rename on the storage to align
	// share display name and storage filename.
	if req.Field.GetState() != usershareproviderv0alphapb.ShareState_SHARE_STATE_INVALID {
		if req.Field.GetState() == usershareproviderv0alphapb.ShareState_SHARE_STATE_ACCEPTED {
			// get received share information to obtain the resource it points to.
			getShareReq := &usershareproviderv0alphapb.GetReceivedShareRequest{Ref: req.Ref}
			getShareRes, err := s.GetReceivedShare(ctx, getShareReq)
			if err != nil {
				log.Err(err).Msg("gateway: error calling GetReceivedShare")
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			if getShareRes.Status.Code != rpcpb.Code_CODE_OK {
				log.Error().Msg("gateway: error calling GetReceivedShare")
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
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
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			homeReq := &storageregv0alphapb.GetHomeRequest{}
			homeRes, err := storageRegClient.GetHome(ctx, homeReq)
			if err != nil {
				err := errors.Wrap(err, "gateway: error calling GetHome")
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error updating received share"),
				}, nil
			}

			// reference path is the home path + some name
			// TODO(labkode): where shares should be created, here we can define the folder in the gateway
			// so the target path on the home storage provider will be:
			// CreateReferene(cs3://home/shares/x)
			// CreateReference(cs3://eos/user/g/gonzalhu/.shares/x)
			// CreateReference(cs3://eos/user/.hidden/g/gonzalhu/shares/x)
			// A reference can point to any place, for that reason the namespace starts with cs3://
			// For example, a reference can point also to a dropbox resource:
			// CreateReference(dropbox://x/y/z)
			// It is the responsibility of the gateway to resolve these references and merge the response back
			// from the main request.
			// TODO(labkode): the name of the share should be the filename it points to by default.
			refPath := path.Join(homeRes.Path, req.Ref.String())
			createRefReq := &storageproviderv0alphapb.CreateReferenceRequest{
				Path:      refPath,
				TargetUri: fmt.Sprintf("cs3:%s/%s", share.Share.ResourceId.GetStorageId(), share.Share.ResourceId.GetOpaqueId()),
			}

			c, err := s.findByPath(ctx, refPath)
			if err != nil {
				if _, ok := err.(errtypes.IsNotFound); ok {
					return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
						Status: status.NewNotFound(ctx, "storage provider not found"),
					}, nil
				}
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error finding storage provider"),
				}, nil
			}

			createRefRes, err := c.CreateReference(ctx, createRefReq)
			if err != nil {
				log.Err(err).Msg("gateway: error calling GetHome")
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: &rpcpb.Status{
						Code: rpcpb.Code_CODE_INTERNAL,
					},
				}, nil
			}

			if createRefRes.Status.Code != rpcpb.Code_CODE_OK {
				err := status.NewErrorFromCode(createRefRes.Status.GetCode(), "gateway")
				return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
					Status: status.NewInternal(ctx, err, "error updating received share"),
				}, nil
			}

			return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
				Status: status.NewOK(ctx),
			}, nil
		}
	}

	// TODO(labkode): implementing updating display name
	err = errors.New("gateway: update of display name is not yet implemented")
	return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
		Status: status.NewUnimplemented(ctx, err, "error updating received share"),
	}, nil
}
