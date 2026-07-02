// Copyright 2018-2024 CERN
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
	"strings"

	"github.com/alitto/pond/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/cs3org/reva/v3/pkg/utils/resourceid"

	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	"github.com/pkg/errors"
)

func (s *svc) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.CreateShareResponse, error) {
	if s.isSharedFolder(ctx, req.ResourceInfo.GetPath()) {
		return nil, errtypes.AlreadyExists("gateway: can't share the share folder itself")
	}

	log := appctx.GetLogger(ctx)

	shareClient, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		return &collaboration.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	// First we ping the db
	// --------------------
	// See ADR-REVA-003
	_, err = shareClient.GetShare(ctx, &collaboration.GetShareRequest{
		Ref: &collaboration.ShareReference{
			Spec: &collaboration.ShareReference_Id{
				Id: &collaboration.ShareId{
					OpaqueId: "0",
				},
			},
		},
	})

	// We expect a "not found" error when querying ID 0
	// error checking is kind of ugly, because we lose the original error object over grpc
	if !strings.HasSuffix(err.Error(), errtypes.NotFound("0").Error()) {
		return nil, errtypes.InternalError("ShareManager is not online")
	}

	// Hierarchy check (ADR-GENERAL-005)
	// (https://gitlab.cern.ch/cernbox/adr/-/blob/master/decisions/general/0005-sharing.md)
	// ---------------------------------
	spaceId := req.ResourceInfo.Id.SpaceId
	if spaceId == "" {
		if stat, statErr := s.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: req.ResourceInfo.Id}}); statErr == nil && stat.Status.Code == rpc.Code_CODE_OK && stat.Info.Id.SpaceId != "" {
			spaceId = stat.Info.Id.SpaceId
			req.ResourceInfo.Id.SpaceId = spaceId
			log.Debug().Str("spaceId", spaceId).Msg("sharehierarchy: populated missing spaceId via stat")
		} else {
			log.Error().Msg("sharehierarchy: spaceId missing and stat could not resolve it")
			return nil, errors.New("SpaceID missing when creating share")
		}
	}

	// Force means we delete child shares, instead of returning a conflict error
	force := parseForce(req.Opaque)

	existingShares, err := s.listSharesForGranteeInSpace(ctx, shareClient, spaceId, req.Grant.Grantee)
	if err != nil {
		return &collaboration.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error listing shares for hierarchy check"),
		}, nil
	}

	checker := &sharehierarchy.Checker{GetPath: s.getPathForResourceId}
	result, err := checker.CheckGrantConsistency(ctx, req.ResourceInfo.Path, req.Grant.Permissions.Permissions, existingShares)
	if err != nil {
		conflictErr := err.(*sharehierarchy.HierarchyConflictError)
		return &collaboration.CreateShareResponse{
			Status: status.NewConflict(ctx, conflictErr, conflictErr.MarshalToJSON()),
		}, nil
	}
	// If we don't force, we show a warning to the user that shares will be deleted
	if !force && len(result.ToDelete) > 0 {
		conflictErr := sharehierarchy.NewChildConflictError(sharehierarchy.ChildConflictMessage(result.ToDelete), result.ToDelete)
		return &collaboration.CreateShareResponse{
			Status: status.NewConflict(ctx, conflictErr, conflictErr.MarshalToJSON()),
		}, nil
	}

	// If we commit to the storage, first we apply to the new resource, then we clean up
	toDelete := sharehierarchy.Shares(result.ToDelete)
	toReapply := sharehierarchy.Shares(result.ToReapply)
	if s.c.CommitShareToStorageGrant {
		if grantStatus, err := s.applyGrant(ctx, req.ResourceInfo.Id, req.Grant.Grantee, req.Grant.Permissions.Permissions); err != nil || grantStatus.Code != rpc.Code_CODE_OK {
			if err != nil {
				return nil, errors.Wrap(err, "gateway: error applying grant to storage")
			}
			return &collaboration.CreateShareResponse{Status: grantStatus}, nil
		}
		// Remove grants for child shares made redundant by the new share.
		if st, err := s.removeChildGrants(ctx, toDelete); err != nil || st.Code != rpc.Code_CODE_OK {
			if err != nil {
				return nil, err
			}
			return &collaboration.CreateShareResponse{Status: st}, nil
		}
		// Re-apply grants for children that retain higher permissions (N=R, C=RW).
		// ToReapply is already sorted shallowest-first by CheckGrantConsistency.
		s.reapplyChildGrants(ctx, toReapply)
	}

	// Finally, we write to the db
	res, err := shareClient.CreateShare(ctx, req)
	if err != nil {
		log.Error().Str("ResourceInfo", req.ResourceInfo.String()).Str("Grantee", req.Grant.Grantee.String()).Msg("Failed to Create Share but ACLs are already set")
		return nil, errors.Wrap(err, "gateway: error calling CreateShare")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("ShareClient returned error: " + res.Status.Code.String() + ": " + res.Status.Message)
	}

	// And we remove from the db the deleted shares made redundant by the new share.
	s.removeChildShareRecords(ctx, shareClient, toDelete)

	return res, nil

}

func (s *svc) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest) (*collaboration.RemoveShareResponse, error) {
	log := appctx.GetLogger(ctx)

	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		return &collaboration.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error getting user share provider client"),
		}, nil
	}

	getShareRes, err := c.GetShare(ctx, &collaboration.GetShareRequest{Ref: req.Ref})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetShare")
	}
	if getShareRes.Status.Code != rpc.Code_CODE_OK {
		return &collaboration.RemoveShareResponse{
			Status: status.NewInternal(ctx, status.NewErrorFromCode(getShareRes.Status.Code, "gateway"),
				"error getting share to be removed"),
		}, nil
	}
	share := getShareRes.Share

	checker := &sharehierarchy.Checker{GetPath: s.getPathForResourceId}

	if share.ResourceId.SpaceId == "" {
		if stat, statErr := s.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: share.ResourceId}}); statErr == nil && stat.Status.Code == rpc.Code_CODE_OK && stat.Info.Id.SpaceId != "" {
			share.ResourceId.SpaceId = stat.Info.Id.SpaceId
			log.Debug().Str("spaceId", share.ResourceId.SpaceId).Msg("sharehierarchy: populated missing spaceId for DeleteShare via stat")
		} else {
			log.Warn().Msg("sharehierarchy: spaceId missing on DeleteShare and stat could not resolve it")
		}
	}

	// Resolve parent / child shares so we can reapply the hierarchy (ADR-GENERAL-005).
	existingShares, listErr := s.listSharesForGranteeInSpace(ctx, c, share.ResourceId.SpaceId, share.Grantee)
	if listErr != nil {
		return &collaboration.RemoveShareResponse{
			Status: status.NewInternal(ctx, listErr, "error listing shares for hierarchy reapply"),
		}, nil
	}
	reapply := checker.GrantsToReapplyAfterRemove(ctx, share.Id.OpaqueId, share.ResourceId, existingShares)

	if s.c.CommitShareToStorageGrant {
		removeGrantStatus, err := s.removeGrant(ctx, share.ResourceId, share.Grantee, share.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error removing grant from storage")
		}
		if removeGrantStatus.Code != rpc.Code_CODE_OK {
			return &collaboration.RemoveShareResponse{Status: removeGrantStatus}, nil
		}

		// Re-apply the closest parent's permissions to the now-unshared node (if any).
		if reapply.ParentGrant != nil {
			if _, err := s.addGrant(ctx, share.ResourceId, share.Grantee, reapply.ParentGrant.Permissions.Permissions); err != nil {
				log.Error().Err(err).Str("shareId", share.Id.OpaqueId).Msg("error reapplying parent grant after RemoveShare")
			}
		}

		// Re-apply descendant grants shallowest-first so child permissions are not overridden.
		// (and the results are already pre-sorted to be shallowest-first)
		s.reapplyChildGrants(ctx, reapply.ChildGrants)
	}

	res, err := c.RemoveShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling RemoveShare")
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

func (s *svc) ListExistingShares(ctx context.Context, req *collaboration.ListSharesRequest) (*gateway.ListExistingSharesResponse, error) {
	shares, err := s.ListShares(ctx, req)
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling ListExistingShares")
		return &gateway.ListExistingSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing shares"),
		}, nil
	}

	sharesCh := make(chan *gateway.ShareResourceInfo, len(shares.Shares))
	pool := pond.NewPool(50)

	for _, share := range shares.Shares {
		pool.SubmitErr(func() error {
			key := resourceid.OwnCloudResourceIDWrap(share.ResourceId)
			var resourceInfo *provider.ResourceInfo
			if res, err := s.resourceInfoCache.Get(key); err == nil && res != nil {
				resourceInfo = res
			} else {
				stat, err := s.Stat(ctx, &provider.StatRequest{
					Ref: &provider.Reference{
						ResourceId: share.ResourceId,
					},
				})
				if err != nil {
					return err
				}
				if stat.Status.Code != rpc.Code_CODE_OK {
					return errors.New("An error occurred: " + stat.Status.Message)
				}
				resourceInfo = stat.Info
				if s.resourceInfoCacheTTL > 0 {
					_ = s.resourceInfoCache.SetWithExpire(key, resourceInfo, s.resourceInfoCacheTTL)
				}
			}

			sharesCh <- &gateway.ShareResourceInfo{
				ResourceInfo: resourceInfo,
				Share:        share,
			}

			return nil
		})
	}

	sris := make([]*gateway.ShareResourceInfo, 0, len(shares.Shares))
	done := make(chan struct{})
	go func() {
		for s := range sharesCh {
			sris = append(sris, s)
		}
		done <- struct{}{}
	}()
	err = pool.Stop().Wait()
	close(sharesCh)
	<-done
	close(done)

	if err != nil {
		return &gateway.ListExistingSharesResponse{
			ShareInfos: sris,
			Status:     status.NewInternal(ctx, err, "An error occured listing existing shares"),
		}, err
	}

	return &gateway.ListExistingSharesResponse{
		ShareInfos: sris,
		Status:     status.NewOK(ctx),
	}, nil
}

func (s *svc) UpdateShare(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.UpdateShareResponse, error) {
	c, err := pool.GetUserShareProviderClient(pool.Endpoint(s.c.UserShareProviderEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetUserShareProviderClient")
		return &collaboration.UpdateShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share provider client"),
		}, nil
	}

	// Fetch the current share before updating so we have its path and space.
	getRes, err := c.GetShare(ctx, &collaboration.GetShareRequest{Ref: req.Ref})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetShare for UpdateShare")
	}
	if getRes.Status.Code != rpc.Code_CODE_OK {
		return &collaboration.UpdateShareResponse{
			Status: status.NewInternal(ctx, status.NewErrorFromCode(getRes.Status.Code, "gateway"), "error getting share for update"),
		}, nil
	}
	currentShare := getRes.Share

	_, isPermUpdate := req.Field.GetField().(*collaboration.UpdateShareRequest_UpdateField_Permissions)
	newPerms := req.Field.GetPermissions().GetPermissions()

	if isPermUpdate && currentShare.ResourceId.SpaceId == "" {
		if stat, statErr := s.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{ResourceId: currentShare.ResourceId}}); statErr == nil && stat.Status.Code == rpc.Code_CODE_OK && stat.Info.Id.SpaceId != "" {
			currentShare.ResourceId.SpaceId = stat.Info.Id.SpaceId
			appctx.GetLogger(ctx).Debug().Str("spaceId", currentShare.ResourceId.SpaceId).Msg("sharehierarchy: populated missing spaceId for UpdateShare via stat")
		} else {
			appctx.GetLogger(ctx).Warn().Msg("sharehierarchy: spaceId missing on UpdateShare and stat could not resolve it")
		}
	}

	// Hierarchy check (ADR-GENERAL-005) — only for permission updates when SpaceId is available.
	// Returns early on conflict; on success, populates toDelete/toReapply for use below.
	checker := &sharehierarchy.Checker{GetPath: s.getPathForResourceId}
	var toDelete, toReapply []*collaboration.Share
	if isPermUpdate && currentShare.ResourceId.SpaceId != "" {
		force := parseForce(req.Opaque)

		existingShares, listErr := s.listSharesForGranteeInSpace(ctx, c, currentShare.ResourceId.SpaceId, currentShare.Grantee)
		if listErr != nil {
			return &collaboration.UpdateShareResponse{
				Status: status.NewInternal(ctx, listErr, "error listing shares for hierarchy check"),
			}, nil
		}
		existingShares = filterOutShare(existingShares, currentShare.Id.OpaqueId)

		currentPath, pathErr := s.getPathForResourceId(ctx, currentShare.ResourceId)
		if pathErr != nil {
			return &collaboration.UpdateShareResponse{
				Status: status.NewInternal(ctx, pathErr, "error resolving share path for hierarchy check"),
			}, nil
		}

		result, checkErr := checker.CheckGrantConsistency(ctx, currentPath, newPerms, existingShares)
		if checkErr != nil {
			conflictErr := checkErr.(*sharehierarchy.HierarchyConflictError)
			return &collaboration.UpdateShareResponse{
				Status: status.NewConflict(ctx, conflictErr, conflictErr.MarshalToJSON()),
			}, nil
		}
		if !force && len(result.ToDelete) > 0 {
			conflictErr := sharehierarchy.NewChildConflictError(sharehierarchy.ChildConflictMessage(result.ToDelete), result.ToDelete)
			return &collaboration.UpdateShareResponse{
				Status: status.NewConflict(ctx, conflictErr, conflictErr.MarshalToJSON()),
			}, nil
		}
		toDelete = sharehierarchy.Shares(result.ToDelete)
		toReapply = sharehierarchy.Shares(result.ToReapply)
	}

	if isPermUpdate && s.c.CommitShareToStorageGrant {
		if grantStatus, err := s.updateGrant(ctx, currentShare.ResourceId, currentShare.Grantee, newPerms); err != nil || grantStatus.Code != rpc.Code_CODE_OK {
			if err != nil {
				return nil, errors.Wrap(err, "gateway: error calling updateGrant")
			}
			return &collaboration.UpdateShareResponse{Status: grantStatus}, nil
		}

		// Remove grants for child shares made redundant by the updated permissions.
		if st, err := s.removeChildGrants(ctx, toDelete); err != nil || st.Code != rpc.Code_CODE_OK {
			if err != nil {
				return nil, err
			}
			return &collaboration.UpdateShareResponse{Status: st}, nil
		}

		// Re-apply grants for children that retain higher permissions.
		// toReapply is already sorted shallowest-first by CheckGrantConsistency.
		s.reapplyChildGrants(ctx, toReapply)
	}

	res, err := c.UpdateShare(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateShare")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return res, nil
	}
	// Remove child share records made redundant by the updated permissions.
	s.removeChildShareRecords(ctx, c, toDelete)

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

func (s *svc) ListExistingReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest) (*gateway.ListExistingReceivedSharesResponse, error) {
	rshares, err := s.ListReceivedShares(ctx, req)
	if err != nil {
		err := errors.Wrap(err, "gateway: error calling ListExistingReceivedShares")
		return &gateway.ListExistingReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing received shares"),
		}, nil
	}

	if rshares.Status == nil || rshares.Status.Code != rpc.Code_CODE_OK {
		return &gateway.ListExistingReceivedSharesResponse{
			Status: rshares.Status,
		}, nil
	}

	sharesCh := make(chan *gateway.ReceivedShareResourceInfo, len(rshares.Shares))
	pool := pond.NewPool(50)
	for _, rs := range rshares.Shares {
		pool.SubmitErr(func() error {
			if rs.State == collaboration.ShareState_SHARE_STATE_INVALID {
				return errors.New("Invalid Share State")
			}

			key := resourceid.OwnCloudResourceIDWrap(rs.Share.ResourceId)
			var resourceInfo *provider.ResourceInfo
			if res, err := s.resourceInfoCache.Get(key); err == nil && res != nil {
				resourceInfo = res
			} else {
				stat, err := s.Stat(ctx, &provider.StatRequest{
					Ref: &provider.Reference{
						ResourceId: rs.Share.ResourceId,
					},
				})
				if err != nil {
					return err
				}
				if stat.Status.Code != rpc.Code_CODE_OK {
					return errors.New("An error occurred: " + stat.Status.Message)
				}
				resourceInfo = stat.Info
				if s.resourceInfoCacheTTL > 0 {
					_ = s.resourceInfoCache.SetWithExpire(key, resourceInfo, s.resourceInfoCacheTTL)
				}
			}
			sharesCh <- &gateway.ReceivedShareResourceInfo{
				ResourceInfo:  resourceInfo,
				ReceivedShare: rs,
			}
			return nil
		})
	}

	sris := make([]*gateway.ReceivedShareResourceInfo, 0, len(rshares.Shares))
	done := make(chan struct{})
	go func() {
		for s := range sharesCh {
			sris = append(sris, s)
		}
		done <- struct{}{}
	}()
	err = pool.Stop().Wait()
	close(sharesCh)
	<-done
	close(done)

	if err != nil {
		return &gateway.ListExistingReceivedSharesResponse{
			ShareInfos: sris,
			Status:     status.NewInternal(ctx, err, "An error occured listing received shares"),
		}, err
	}

	return &gateway.ListExistingReceivedSharesResponse{
		ShareInfos: sris,
		Status:     status.NewOK(ctx),
	}, nil
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

	return res, nil
}

// ---------------------------------------------------------------------------
// Hierarchy helpers (ADR-0005-P01)
// ---------------------------------------------------------------------------

// parseForce reads the "force" flag from the request opaque map.
func parseForce(opaque *typesv1beta1.Opaque) bool {
	if opaque == nil {
		return false
	}
	v, ok := opaque.Map["force"]
	return ok && string(v.Value) == "true"
}

// getPathForResourceId resolves a ResourceId to its current filesystem path.
func (s *svc) getPathForResourceId(ctx context.Context, id *provider.ResourceId) (string, error) {
	res, err := s.GetPath(ctx, &provider.GetPathRequest{ResourceId: id})
	if err != nil {
		return "", errors.Wrap(err, "gateway: error calling GetPath")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return "", errors.New("gateway: GetPath failed: " + res.Status.Message)
	}
	return res.Path, nil
}

// listSharesForGranteeInSpace returns all active shares for the given grantee in the given space.
func (s *svc) listSharesForGranteeInSpace(ctx context.Context, c collaboration.CollaborationAPIClient, spaceId string, grantee *provider.Grantee) ([]*collaboration.Share, error) {
	if spaceId == "" {
		appctx.GetLogger(ctx).Warn().Msg("sharehierarchy: spaceId is empty, skipping hierarchy check")
		return nil, nil
	}

	res, err := c.ListShares(ctx, &collaboration.ListSharesRequest{
		Filters: []*collaboration.Filter{
			revashare.SpaceIDFilter(spaceId),
			revashare.GranteeFilter(grantee),
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error listing shares for grantee in space")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("gateway: ListShares for space returned: " + res.Status.Message)
	}
	return res.Shares, nil
}

// applyGrant sets an ACL on EOS for the given resource, calling denyGrant for empty permissions
// or addGrant for normal permissions.
func (s *svc) applyGrant(ctx context.Context, id *provider.ResourceId, grantee *provider.Grantee, perms *provider.ResourcePermissions) (*rpc.Status, error) {
	if grants.PermissionsEqual(perms, &provider.ResourcePermissions{}) {
		return s.denyGrant(ctx, id, grantee)
	}
	return s.addGrant(ctx, id, grantee, perms)
}

// removeChildGrants removes the storage grants for the given child shares.
// It aborts on the first failure, returning a non-OK status (when the storage
// rejected the call) or an error (when the call itself failed), mirroring the
// caller's expectation that a redundant child must be cleaned up before we proceed.
func (s *svc) removeChildGrants(ctx context.Context, children []*collaboration.Share) (*rpc.Status, error) {
	for _, child := range children {
		st, err := s.removeGrant(ctx, child.ResourceId, child.Grantee, child.Permissions.Permissions)
		if err != nil {
			return nil, errors.Wrap(err, "gateway: error removing child grant")
		}
		if st.Code != rpc.Code_CODE_OK {
			return st, nil
		}
	}
	return status.NewOK(ctx), nil
}

// reapplyChildGrants re-applies the storage grants for the given child shares on a
// best-effort basis. Children must be pre-sorted shallowest-first so that deeper,
// more specific grants are not overridden. Failures are logged but do not abort the caller.
func (s *svc) reapplyChildGrants(ctx context.Context, children []*collaboration.Share) {
	log := appctx.GetLogger(ctx)
	for _, child := range children {
		if _, err := s.addGrant(ctx, child.ResourceId, child.Grantee, child.Permissions.Permissions); err != nil {
			log.Error().Err(err).Str("shareId", child.Id.OpaqueId).Msg("error re-applying child grant")
		}
	}
}

// removeChildShareRecords deletes the share DB records for the given child shares
// on a best-effort basis. Failures are logged but do not abort the caller.
func (s *svc) removeChildShareRecords(ctx context.Context, c collaboration.CollaborationAPIClient, children []*collaboration.Share) {
	log := appctx.GetLogger(ctx)
	for _, child := range children {
		if _, err := c.RemoveShare(ctx, &collaboration.RemoveShareRequest{
			Ref: &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: child.Id}},
		}); err != nil {
			log.Error().Err(err).Str("shareId", child.Id.OpaqueId).Msg("error removing child share from DB")
		}
	}
}

// filterOutShare returns shares with the given opaqueId excluded.
func filterOutShare(shares []*collaboration.Share, opaqueId string) []*collaboration.Share {
	result := make([]*collaboration.Share, 0, len(shares))
	for _, s := range shares {
		if s.Id.OpaqueId != opaqueId {
			result = append(result, s)
		}
	}
	return result
}

// ---------------------------------------------------------------------------

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
		err = errors.Wrap(err, "gateway: error calling AddGrant")
		return status.NewInternal(ctx, err, "error committing share to storage grant"), err
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
