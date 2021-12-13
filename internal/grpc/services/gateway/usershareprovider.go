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

package gateway

import (
	"context"
	"path"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"

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
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.ResourceInfo.Id)
	return res, nil
}

func (s *svc) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest) (*collaboration.RemoveShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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

	s.removeReference(ctx, share.ResourceId)

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

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), share.ResourceId)
	return res, nil
}

// TODO(labkode): we need to validate share state vs storage grant and storage ref
// If there are any inconsistencies, the share needs to be flag as invalid and a background process
// or active fix needs to be performed.
func (s *svc) GetShare(ctx context.Context, req *collaboration.GetShareRequest) (*collaboration.GetShareResponse, error) {
	return s.getShare(ctx, req)
}

func (s *svc) getShare(ctx context.Context, req *collaboration.GetShareRequest) (*collaboration.GetShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), res.Share.ResourceId)
	return res, nil
}

// TODO(labkode): listing received shares just goes to the user share manager and gets the list of
// received shares. The display name of the shares should be the a friendly name, like the basename
// of the original file.
func (s *svc) ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest) (*collaboration.ListReceivedSharesResponse, error) {
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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
	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
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
//   1) if received share is mounted: we also do a rename in the storage
//   2) if received share is not mounted: we only rename in user share provider.
func (s *svc) UpdateReceivedShare(ctx context.Context, req *collaboration.UpdateReceivedShareRequest) (*collaboration.UpdateReceivedShareResponse, error) {
	t := rtrace.Provider.Tracer("reva")
	ctx, span := t.Start(ctx, "Gateway.UpdateReceivedShare")
	defer span.End()

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

	c, err := pool.GetUserShareProviderClient(s.c.UserShareProviderEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	RemoveFromCache(s.statCache, ctxpkg.ContextMustGetUser(ctx), req.Share.Share.ResourceId)
	return c.UpdateReceivedShare(ctx, req)
}

func (s *svc) removeReference(ctx context.Context, resourceID *provider.ResourceId) *rpc.Status {
	log := appctx.GetLogger(ctx)

	idReference := &provider.Reference{ResourceId: resourceID}
	storageProvider, _, err := s.find(ctx, idReference)
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

	sharePathRef := &provider.Reference{Path: sharePath}
	homeProvider, providerInfo, err := s.find(ctx, sharePathRef)
	if err != nil {
		if _, ok := err.(errtypes.IsNotFound); ok {
			return status.NewNotFound(ctx, "storage provider not found")
		}
		return status.NewInternal(ctx, err, "error finding storage provider")
	}

	spaceID := ""
	mountPath := providerInfo.ProviderPath
	var root *provider.ResourceId

	spacePaths := decodeSpacePaths(providerInfo.Opaque)
	if len(spacePaths) == 0 {
		spacePaths[""] = mountPath
	}
	for spaceID, mountPath = range spacePaths {
		rootSpace, rootNode := utils.SplitStorageSpaceID(spaceID)
		root = &provider.ResourceId{
			StorageId: rootSpace,
			OpaqueId:  rootNode,
		}
	}

	ref := unwrap(sharePathRef, mountPath, root)

	deleteReq := &provider.DeleteRequest{
		Opaque: &typesv1beta1.Opaque{
			Map: map[string]*typesv1beta1.OpaqueEntry{
				// This signals the storageprovider that we want to delete the share reference and not the underlying file.
				"deleting_shared_resource": {},
			},
		},
		Ref: ref,
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

func (s *svc) denyGrant(ctx context.Context, id *provider.ResourceId, g *provider.Grantee) (*rpc.Status, error) {
	ref := &provider.Reference{
		ResourceId: id,
	}

	grantReq := &provider.DenyGrantRequest{
		Ref:     ref,
		Grantee: g,
	}

	c, _, err := s.find(ctx, ref)
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

	c, _, err := s.find(ctx, ref)
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

	c, _, err := s.find(ctx, ref)
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

	c, _, err := s.find(ctx, ref)
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
