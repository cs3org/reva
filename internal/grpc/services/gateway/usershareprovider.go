// Copyright 2018-2023 CERN
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

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/storage/utils/grants"
	"github.com/pkg/errors"
)

// TODO(labkode): add multi-phase commit logic when commit share or commit ref is enabled.
func (s *svc) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.CreateShareResponse, error) {
	if s.isSharedFolder(ctx, req.ResourceInfo.GetPath()) {
		return nil, errtypes.AlreadyExists("gateway: can't share the share folder itself")
	}

	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		return &collaboration.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// TODO the user share manager needs to be able to decide if the current user is allowed to create that share (and not eg. incerase permissions)
	// jfd: AFAICT this can only be determined by a storage driver - either the storage provider is queried first or the share manager needs to access the storage using a storage driver
	res, err := c.CreateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateShare")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return res, nil
	}

	// if we don't need to commit we return earlier
	if !s.c.CommitShareToStorageGrant && !s.c.CommitShareToStorageRef {
		return res, nil
	}

	// TODO(labkode): if both commits are enabled they could be done concurrently.
	if s.c.CommitShareToStorageGrant {
		// If the share is a denial we call  denyGrant instead.
		if grants.PermissionsEqual(req.Grant.Permissions.Permissions, &provider.ResourcePermissions{}) {
			denyGrantStatus, err := s.denyGrant(ctx, req.ResourceInfo.Id, req.Grant.Grantee)
			if err != nil {
				return nil, errors.Wrap(err, "gateway: error denying grant in storage")
			}
			if denyGrantStatus.Code != rpc.Code_CODE_OK {
				return &collaboration.CreateShareResponse{
					Status: denyGrantStatus,
				}, err
			}
			return res, nil
		}

		addGrantStatus, err := s.addGrant(ctx, req.ResourceInfo.Id, req.Grant.Grantee, req.Grant.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error adding grant to storage")
		}
		if addGrantStatus.Code != rpc.Code_CODE_OK {
			return &collaboration.CreateShareResponse{
				Status: addGrantStatus,
			}, err
		}
	}

	return res, nil
}

func (s *svc) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest) (*collaboration.RemoveShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		return &collaboration.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// if we need to commit the share, we need the resource it points to.
	var share *collaboration.Share
	if s.c.CommitShareToStorageGrant || s.c.CommitShareToStorageRef {
		getShareReq := &collaboration.GetShareRequest{
			Ref: req.Ref,
		}
		getShareRes, err := c.GetShare(ctx, getShareReq)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling GetShare")
		}

		if getShareRes.Status.Code != rpc.Code_CODE_OK {
			res := &collaboration.RemoveShareResponse{
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
		removeGrantStatus, err := s.removeGrant(ctx, share.ResourceId, share.Grantee, share.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error removing grant from storage")
		}
		if removeGrantStatus.Code != rpc.Code_CODE_OK {
			return &collaboration.RemoveShareResponse{
				Status: removeGrantStatus,
			}, err
		}
	}

	return res, nil
}

// TODO(labkode): we need to validate share state vs storage grant and storage ref
// If there are any inconsistencies, the share needs to be flag as invalid and a background process
// or active fix needs to be performed.
func (s *svc) GetShare(ctx context.Context, req *collaboration.GetShareRequest) (*collaboration.GetShareResponse, error) {
	return s.getShare(ctx, req)
}

func (s *svc) getShare(ctx context.Context, req *collaboration.GetShareRequest) (*collaboration.GetShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.GetShareResponse{
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
func (s *svc) ListShares(ctx context.Context, req *collaboration.ListSharesRequest) (*collaboration.ListSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.ListSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	res, err := c.ListShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListShares")
	}

	return res, nil
}

func (s *svc) UpdateShare(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.UpdateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.UpdateShareResponse{
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

	if s.c.CommitShareToStorageGrant {
		updateGrantStatus, err := s.updateGrant(ctx, res.GetShare().GetResourceId(),
			res.GetShare().GetGrantee(),
			res.GetShare().GetPermissions().GetPermissions())

		if err != nil {
			return nil, errors.Wrap(err, "gateway: error calling updateGrant")
		}

		if updateGrantStatus.Code != rpc.Code_CODE_OK {
			return &collaboration.UpdateShareResponse{
				Status: updateGrantStatus,
				Share:  res.Share,
			}, nil
		}
	}

	return res, nil
}

// TODO(labkode): listing received shares just goes to the user share manager and gets the list of
// received shares. The display name of the shares should be the a friendly name, like the basename
// of the original file.
func (s *svc) ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest) (*collaboration.ListReceivedSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.ListReceivedShares(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListReceivedShares")
	}
	return res, nil
}

func (s *svc) GetReceivedShare(ctx context.Context, req *collaboration.GetReceivedShareRequest) (*collaboration.GetReceivedShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err := errors.Wrap(err, "gateway: error getting user share provider client")
		return &collaboration.GetReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res, err := c.GetReceivedShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetReceivedShare")
	}

	return res, nil
}

// When updating a received share:
// if the update contains update for displayName:
//  1. if received share is mounted: we also do a rename in the storage
//  2. if received share is not mounted: we only rename in user share provider.
func (s *svc) UpdateReceivedShare(ctx context.Context, req *collaboration.UpdateReceivedShareRequest) (*collaboration.UpdateReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)

	// sanity checks
	switch {
	case req.GetShare() == nil:
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInvalidArg(ctx, "updating requires a received share object"),
		}, nil
	case req.GetShare().GetShare() == nil:
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInvalidArg(ctx, "share missing"),
		}, nil
	case req.GetShare().GetShare().GetId() == nil:
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInvalidArg(ctx, "share id missing"),
		}, nil
	case req.GetShare().GetShare().GetId().GetOpaqueId() == "":
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInvalidArg(ctx, "share id empty"),
		}, nil
	}

	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	res, err := c.UpdateReceivedShare(ctx, req)
	if err != nil {
		log.Err(err).Msg("gateway: error calling UpdateReceivedShare")
		return &collaboration.UpdateReceivedShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	// error failing to update share state.
	if res.Status.Code != rpc.Code_CODE_OK {
		return res, nil
	}

	// if we don't need to create/delete references then we return early.
	if !s.c.CommitShareToStorageRef ||
		appctx.ContextMustGetUser(ctx).Id.Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT ||
		appctx.ContextMustGetUser(ctx).Id.Type == userpb.UserType_USER_TYPE_FEDERATED {
		return res, nil
	}

	// check if we have a resource id in the update response that we can use to update references
	if res.GetShare().GetShare().GetResourceId() == nil {
		log.Err(err).Msg("gateway: UpdateReceivedShare must return a ResourceId")
		return &collaboration.UpdateReceivedShareResponse{
			Status: &rpc.Status{
				Code: rpc.Code_CODE_INTERNAL,
			},
		}, nil
	}

	// properties are updated in the order they appear in the field mask
	// when an error occurs the request ends and no further fields are updated
	for i := range req.UpdateMask.Paths {
		switch req.UpdateMask.Paths[i] {
		case "state":
			switch req.GetShare().GetState() {
			case collaboration.ShareState_SHARE_STATE_ACCEPTED:
				rpcStatus := s.createReference(ctx, res.GetShare().GetShare().GetResourceId())
				if rpcStatus.Code != rpc.Code_CODE_OK {
					return &collaboration.UpdateReceivedShareResponse{Status: rpcStatus}, nil
				}
			case collaboration.ShareState_SHARE_STATE_REJECTED:
				rpcStatus := s.removeReference(ctx, res.GetShare().GetShare().ResourceId)
				if rpcStatus.Code != rpc.Code_CODE_OK && rpcStatus.Code != rpc.Code_CODE_NOT_FOUND {
					return &collaboration.UpdateReceivedShareResponse{Status: rpcStatus}, nil
				}
			}
		case "mount_point":
			// TODO(labkode): implementing updating mount point
			err = errtypes.NotSupported("gateway: update of mount point is not yet implemented")
			return &collaboration.UpdateReceivedShareResponse{
				Status: status.NewUnimplemented(ctx, err, "error updating received share"),
			}, nil
		default:
			return nil, errtypes.NotSupported("updating " + req.UpdateMask.Paths[i] + " is not supported")
		}
	}
	return res, nil
}

func (s *svc) removeReference(ctx context.Context, resourceID *provider.ResourceId) *rpc.Status {
	log := appctx.GetLogger(ctx)

	idReference := &provider.Reference{ResourceId: resourceID}
	storageProvider, err := s.find(ctx, idReference)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found")
		}
		return status.NewInternal(ctx, err, "error finding storage provider")
	}

	statRes, err := storageProvider.Stat(ctx, &provider.StatRequest{Ref: idReference})
	if err != nil {
		return status.NewInternal(ctx, err, "gateway: error calling Stat for the share resource id: "+resourceID.String())
	}

	// FIXME how can we delete a reference if the original resource was deleted?
	if statRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(statRes.Status.GetCode(), "gateway")
		return status.NewInternal(ctx, err, "could not delete share reference")
	}

	homeRes, err := s.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling GetHome")
		return status.NewInternal(ctx, err, "could not delete share reference")
	}

	sharePath := path.Join(homeRes.Path, s.c.ShareFolder, path.Base(statRes.Info.Path))
	log.Debug().Str("share_path", sharePath).Msg("remove reference of share")

	homeProvider, err := s.find(ctx, &provider.Reference{Path: sharePath})
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found")
		}
		return status.NewInternal(ctx, err, "error finding storage provider")
	}

	deleteReq := &provider.DeleteRequest{
		Opaque: &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				// This signals the storageprovider that we want to delete the share reference and not the underlying file.
				"deleting_shared_resource": {},
			},
		},
		Ref: &provider.Reference{Path: sharePath},
	}

	deleteResp, err := homeProvider.Delete(ctx, deleteReq)
	if err != nil {
		return status.NewInternal(ctx, err, "could not delete share reference")
	}

	switch deleteResp.Status.Code {
	case rpc.Code_CODE_OK:
		// we can continue deleting the reference
	case rpc.Code_CODE_NOT_FOUND:
		// This is fine, we wanted to delete it anyway
		return status.NewOK(ctx)
	default:
		err := status.NewErrorFromCode(deleteResp.Status.GetCode(), "gateway")
		return status.NewInternal(ctx, err, "could not delete share reference")
	}

	log.Debug().Str("share_path", sharePath).Msg("share reference successfully removed")

	return status.NewOK(ctx)
}

func (s *svc) createReference(ctx context.Context, resourceID *provider.ResourceId) *rpc.Status {
	ref := &provider.Reference{
		ResourceId: resourceID,
	}
	log := appctx.GetLogger(ctx)

	// get the metadata about the share
	c, err := s.find(ctx, ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found")
		}
		return status.NewInternal(ctx, err, "error finding storage provider")
	}

	statReq := &provider.StatRequest{
		Ref: ref,
	}

	statRes, err := c.Stat(ctx, statReq)
	if err != nil {
		return status.NewInternal(ctx, err, "gateway: error calling Stat for the share resource id: "+resourceID.String())
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(statRes.Status.GetCode(), "gateway")
		log.Err(err).Msg("gateway: Stat failed on the share resource id: " + resourceID.String())
		return status.NewInternal(ctx, err, "error updating received share")
	}

	homeRes, err := s.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling GetHome")
		return status.NewInternal(ctx, err, "error updating received share")
	}

	// reference path is the home path + some name
	// CreateReferene(cs3://home/MyShares/x)
	// that can end up in the storage provider like:
	// /eos/user/.shadow/g/gonzalhu/MyShares/x
	// A reference can point to any place, for that reason the namespace starts with cs3://
	// For example, a reference can point also to a dropbox resource:
	// CreateReference(dropbox://x/y/z)
	// It is the responsibility of the gateway to resolve these references and merge the response back
	// from the main request.
	// TODO(labkode): the name of the share should be the filename it points to by default.
	refPath := path.Join(homeRes.Path, s.c.ShareFolder, path.Base(statRes.Info.Path))
	log.Info().Msg("mount path will be:" + refPath)

	createRefReq := &provider.CreateReferenceRequest{
		Ref: &provider.Reference{Path: refPath},
		// cs3 is the Scheme and %s/%s is the Opaque parts of a net.URL.
		TargetUri: fmt.Sprintf("cs3:%s/%s", resourceID.GetStorageId(), resourceID.GetOpaqueId()),
	}

	c, err = s.findByPath(ctx, refPath)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found")
		}
		return status.NewInternal(ctx, err, "error finding storage provider")
	}

	createRefRes, err := c.CreateReference(ctx, createRefReq)
	if err != nil {
		log.Err(err).Msg("gateway: error calling GetHome")
		return &rpc.Status{
			Code: rpc.Code_CODE_INTERNAL,
		}
	}

	if createRefRes.Status.Code != rpc.Code_CODE_OK {
		err := status.NewErrorFromCode(createRefRes.Status.GetCode(), "gateway")
		return status.NewInternal(ctx, err, "error updating received share")
	}

	return status.NewOK(ctx)
}

func (s *svc) denyGrant(ctx context.Context, id *provider.ResourceId, g *provider.Grantee) (*rpc.Status, error) {
	ref := &provider.Reference{
		ResourceId: id,
	}

	grantReq := &provider.DenyGrantRequest{
		Ref:     ref,
		Grantee: g,
	}

	c, err := s.find(ctx, ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found"), nil
		}
		return status.NewInternal(ctx, err, "error finding storage provider"), nil
	}

	grantRes, err := c.DenyGrant(ctx, grantReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling DenyGrant")
	}
	if grantRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
			"error committing share to storage grant"), nil
	}

	return status.NewOK(ctx), nil
}

func (s *svc) addGrant(ctx context.Context, id *provider.ResourceId, g *provider.Grantee, p *provider.ResourcePermissions) (*rpc.Status, error) {
	ref := &provider.Reference{
		ResourceId: id,
	}

	grantReq := &provider.AddGrantRequest{
		Ref: ref,
		Grant: &provider.Grant{
			Grantee:     g,
			Permissions: p,
		},
	}

	c, err := s.find(ctx, ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found"), nil
		}
		return status.NewInternal(ctx, err, "error finding storage provider"), nil
	}

	grantRes, err := c.AddGrant(ctx, grantReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling AddGrant")
	}
	if grantRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
			"error committing share to storage grant"), nil
	}

	return status.NewOK(ctx), nil
}

func (s *svc) updateGrant(ctx context.Context, id *provider.ResourceId, g *provider.Grantee, p *provider.ResourcePermissions) (*rpc.Status, error) {
	ref := &provider.Reference{
		ResourceId: id,
	}
	grantReq := &provider.UpdateGrantRequest{
		Ref: ref,
		Grant: &provider.Grant{
			Grantee:     g,
			Permissions: p,
		},
	}

	c, err := s.find(ctx, ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found"), nil
		}
		return status.NewInternal(ctx, err, "error finding storage provider"), nil
	}

	grantRes, err := c.UpdateGrant(ctx, grantReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateGrant")
	}
	if grantRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
			"error committing share to storage grant"), nil
	}

	return status.NewOK(ctx), nil
}

func (s *svc) removeGrant(ctx context.Context, id *provider.ResourceId, g *provider.Grantee, p *provider.ResourcePermissions) (*rpc.Status, error) {
	ref := &provider.Reference{
		ResourceId: id,
	}

	grantReq := &provider.RemoveGrantRequest{
		Ref: ref,
		Grant: &provider.Grant{
			Grantee:     g,
			Permissions: p,
		},
	}

	c, err := s.find(ctx, ref)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found"), nil
		}
		return status.NewInternal(ctx, err, "error finding storage provider"), nil
	}

	grantRes, err := c.RemoveGrant(ctx, grantReq)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveGrant")
	}
	if grantRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewInternal(ctx, status.NewErrorFromCode(grantRes.Status.Code, "gateway"),
			"error removing storage grant"), nil
	}

	return status.NewOK(ctx), nil
}
