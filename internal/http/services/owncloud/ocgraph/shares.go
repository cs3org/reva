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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/spaces/

package ocgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/spaces"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

func (s *svc) getSharedWithMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	recvSharesResp, err := gw.ListExistingReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error getting received shares")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	if recvSharesResp.Status == nil || recvSharesResp.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, recvSharesResp.Status, "ocgraph: failed to perform ListExistingReceivedShares ", w)
	}

	shares := make([]*libregraph.DriveItem, 0)
	for _, share := range recvSharesResp.ShareInfos {
		drive, err := s.cs3ReceivedShareToDriveItem(ctx, share)
		if err != nil {
			log.Error().Err(err).Any("share", share).Msg("error parsing received share, ignoring")
		} else {
			shares = append(shares, drive)
		}
	}

	if s.c.OCMEnabled {
		// include ocm shares in the response
		ocmShareResp, err := gw.ListReceivedOCMShares(ctx, &ocm.ListReceivedOCMSharesRequest{})
		if err != nil {
			//handleError(ctx, err, http.StatusInternalServerError, w)
			log.Fatal().Err(err).Msg("ListReceivedOCMShares returned error - user will not be able to see their OCM shares")
		} else if ocmShareResp != nil {
			if ocmShareResp.Status == nil || ocmShareResp.Status.Code != rpc.Code_CODE_OK {
				handleRpcStatus(ctx, ocmShareResp.Status, "ocgraph: failed to perform ListReceivedOCMShares ", w)
			}

			for _, share := range ocmShareResp.Shares {
				drive, err := s.OCMReceivedShareToDriveItem(ctx, share)
				if err != nil {
					log.Error().Err(err).Any("share", share).Msg("error parsing received share, ignoring")
				} else {
					shares = append(shares, drive)
				}
				log.Debug().Any("share", share).Msg("processing received ocm share")
			}
		}
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shares,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling shares as json")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
}

func (s *svc) createLocalShare(ctx context.Context, gw gateway.GatewayAPIClient, storageID, itemID, path string, owner *userpb.UserId, resourceType provider.ResourceType, recipientType string, recipientID string, exp *types.Timestamp, requestedPerms *provider.ResourcePermissions) (*collaboration.CreateShareResponse, error) {
	grantee, err := s.toGrantee(ctx, recipientType, recipientID)
	if err != nil {
		return nil, err
	}

	createShareRequest := &collaboration.CreateShareRequest{
		ResourceInfo: &provider.ResourceInfo{
			Id: &provider.ResourceId{
				StorageId: storageID,
				OpaqueId:  itemID,
			},
			Path:  path,
			Owner: owner,
			Type:  resourceType,
		},
		Grant: &collaboration.ShareGrant{
			Grantee:    grantee,
			Expiration: exp,
			Permissions: &collaboration.SharePermissions{
				Permissions: requestedPerms,
			},
		},
	}

	resp, err := gw.CreateShare(ctx, createShareRequest)
	if err != nil {
		return nil, err
	}
	if resp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("failed to create share: " + resp.Status.Message)
	}

	return resp, nil
}

func (s *svc) createOCMShare(ctx context.Context, gw gateway.GatewayAPIClient, resourceId *provider.ResourceId, recipientID string, idp string, role string) (*ocm.CreateOCMShareResponse, error) {
	recipientProviderInfo, err := gw.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: idp,
	})

	if err != nil {
		return nil, errors.New("error sending a grpc get invite by domain info request" + recipientProviderInfo.Status.Message)
	}
	if recipientProviderInfo.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("error sending a grpc get invite by domain info request" + recipientProviderInfo.Status.Message)
	}

	perm, viewMode := UnifiedRoleToOCMPermissions(role)
	resp, err := gw.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
		ResourceId: resourceId,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					Idp:      idp,
					OpaqueId: recipientID,
					Type:     userpb.UserType_USER_TYPE_FEDERATED,
				},
			},
		},
		RecipientMeshProvider: recipientProviderInfo.ProviderInfo,
		AccessMethods: []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(perm, []string{}),
			share.NewWebappAccessMethod(viewMode),
		},
	})

	if err != nil {
		return nil, err
	}

	if resp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("failed to create remote share: " + resp.Status.Message)
	}
	return resp, nil
}

func (s *svc) decomposeOCMAddress(recipientID string) (string, string) {
	var username, idp string
	if strings.Contains(recipientID, "@") {
		// split the string into a user and an idp
		parts := strings.SplitN(recipientID, "@", 2)
		username = parts[0]
		idp = parts[1]
		return username, idp
	}
	return "", ""
}

func (s *svc) share(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	// First we get the gateway client
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	// We extract the inode and storage ID from the request
	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		handleError(ctx, errors.New("error decoding resource id"), http.StatusBadRequest, w)
		return
	}
	var resourceId *provider.ResourceId = &provider.ResourceId{
		StorageId: storageID,
		OpaqueId:  itemID,
	}

	// We use this to fetch the path and the owner
	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: resourceId,
		},
	})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, statRes.Status, fmt.Sprintf("ocgraph: failed to stat resource '%s' passed to share", resourceID), w)
		return
	}
	path := statRes.Info.Path
	owner := statRes.Info.Owner

	// Now we decode the request body
	invite := &libregraph.DriveItemInvite{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err = dec.Decode(invite); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		handleError(ctx, err, http.StatusBadRequest, w)
		return
	}

	// From this, we first extract the requested role, which we translate into permissions
	roles := invite.Roles
	if len(roles) != 1 {
		handleError(ctx, errors.New("exactly one role is expected"), http.StatusBadRequest, w)
		return
	}
	role, ok := UnifiedRoleIDToDefinition(roles[0])
	if !ok {
		handleError(ctx, errors.New("invalid role"), http.StatusBadRequest, w)
		return
	}
	requestedPerms := PermissionsToCS3ResourcePermissions(role.RolePermissions)

	// Then we also set an expiry, if needed
	var exp *types.Timestamp
	if invite.ExpirationDateTime != nil {
		exp = &types.Timestamp{
			Seconds: uint64(invite.ExpirationDateTime.Unix()),
		}
	}

	// Check that the user has share permissions
	if !conversions.RoleFromResourcePermissions(statRes.Info.PermissionSet).OCSPermissions().Contain(conversions.PermissionShare) {
		handleError(ctx, errors.New("user does not have share permissions"), http.StatusUnauthorized, w)
		return
	}

	// And we keep a list of share responses
	response := make([]*libregraph.Permission, 0, len(invite.Recipients))

	// Finally, we create the actual share for every requested recipient
	for _, recipient := range invite.Recipients {
		// We check if the sharee exists
		if recipient.ObjectId == nil {
			handleError(ctx, errors.New("missing recipient data"), http.StatusBadRequest, w)
		}
		// If the recipient is a user or a group, we create a local share
		switch *recipient.LibreGraphRecipientType {
		case "user", "group":
			resp, err := s.createLocalShare(ctx, gw, storageID, itemID, path, owner, statRes.Info.Type, *recipient.LibreGraphRecipientType, *recipient.ObjectId, exp, requestedPerms)
			if err != nil {
				log.Error().Err(err).Msg("")
				handleError(ctx, err, http.StatusInternalServerError, w)
				return
			}
			share := resp.GetShare()
			lgPerm, err := s.shareToLibregraphPerm(ctx, &GenericShare{
				shareType: ShareTypeShare,
				share:     share,
				ID:        share.GetId().GetOpaqueId(),
			})

			if err != nil || lgPerm == nil {
				log.Error().Err(err).Any("share", share).Err(err).Any("lgPerm", lgPerm).Msg("error converting created share to permissions")
				handleError(ctx, err, http.StatusInternalServerError, w)
				return
			}
			response = append(response, lgPerm)
		case "remote":
			username, idp := s.decomposeOCMAddress(*recipient.ObjectId)
			if username == "" || idp == "" {
				handleError(ctx, errors.New("invalid remote recipient address, must be remote_user_id@remote_ocm_fqdn"), http.StatusBadRequest, w)
				return
			}
			resp, err := s.createOCMShare(ctx, gw, resourceId, username, idp, roles[0])
			if err != nil {
				log.Error().Err(err).Msg("")
				handleError(ctx, err, http.StatusInternalServerError, w)
				return
			}
			ocmshare := resp.GetShare()
			if ocmshare == nil {
				log.Error().Any("response", resp).Msg("share is nil")
				handleError(ctx, errors.New("share is nil"), http.StatusInternalServerError, w)
				return
			}
			lgPerm, err := s.shareToLibregraphPerm(ctx, &GenericShare{
				shareType: ShareTypeOCMShare,
				ocmshare:  ocmshare,
				ID:        ocmshare.GetId().GetOpaqueId(),
			})
			if err != nil || lgPerm == nil {
				log.Error().Err(err).Any("ocmshare", ocmshare).Err(err).Any("lgPerm", lgPerm).Msg("error converting created share to permissions")
				handleError(ctx, err, http.StatusInternalServerError, w)
				return
			}
			response = append(response, lgPerm)
		}
	}
	log.Debug().Any("response", response).Msg("created shares successfully")

	_ = json.NewEncoder(w).Encode(&ListResponse{
		Value: response,
	})
}

func (s *svc) createLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// First we get the gateway client
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	// We extract the inode and storage ID from the request
	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		handleError(ctx, errors.New("error decoding resource id"), http.StatusBadRequest, w)
		return
	}

	// We use this to fetch the path and the owner
	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: &provider.ResourceId{
				StorageId: storageID,
				OpaqueId:  itemID,
			},
		},
	})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, statRes.Status, fmt.Sprintf("ocgraph: failed to stat resource '%s' passed to createLink", resourceID), w)
		return
	}

	// Now we decode the request body
	linkRequest := &libregraph.DriveItemCreateLink{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err = dec.Decode(linkRequest); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		handleError(ctx, err, http.StatusBadRequest, w)
		return
	}

	// Then we also set an expiry, if needed
	var exp *types.Timestamp
	if linkRequest.ExpirationDateTime != nil {
		exp = &types.Timestamp{
			Seconds: uint64(linkRequest.ExpirationDateTime.Unix()),
		}
	}

	// And we set a password, if needed
	password := ""
	if linkRequest.Password != nil {
		password = *linkRequest.Password
	}

	// Check that the user has share permissions
	if !conversions.RoleFromResourcePermissions(statRes.Info.PermissionSet).OCSPermissions().Contain(conversions.PermissionShare) {
		handleError(ctx, errors.New("user does not have the necessary permissions"), http.StatusUnauthorized, w)
		return
	}

	if linkRequest.Type == nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		handleError(ctx, errors.New("must pass a link type"), http.StatusBadRequest, w)
		return
	}

	req := &link.CreatePublicShareRequest{
		ResourceInfo: statRes.Info,
		Grant: &link.Grant{
			Expiration: exp,
			Password:   password,
			Permissions: &link.PublicSharePermissions{
				Permissions: LinkTypeToPermissions(*linkRequest.Type, statRes.Info.Type),
			},
		},
	}

	resp, err := gw.CreatePublicShare(ctx, req)
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	if resp.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, resp.Status, "ocgraph: failed to create public share", w)
		return
	}

	lgPerm, err := s.shareToLibregraphPerm(ctx, &GenericShare{
		shareType: ShareTypeLink,
		ID:        resp.GetShare().GetId().GetOpaqueId(),
		link:      resp.GetShare(),
	})
	if err != nil || lgPerm == nil {
		log.Error().Err(err).Any("link", resp.GetShare()).Err(err).Any("lgPerm", lgPerm).Msg("error converting created link to permissions")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	_ = json.NewEncoder(w).Encode(lgPerm)
}

func encodeSpaceIDForShareJail(res *provider.ResourceInfo) string {
	return spaces.EncodeResourceID(res.Id)
}

func (s *svc) getUserByID(ctx context.Context, u *userpb.UserId) (*userpb.User, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 u,
		SkipFetchingUserGroups: true,
	})
	if err != nil {
		return nil, err
	}
	if res.Status == nil {
		return nil, errors.New("Did not get status from getUserByID")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("failed to get user by ID, with code %s and message %s", res.Status.Code, res.Status.Message)
	}

	return res.User, nil
}

func (s *svc) getGroupByID(ctx context.Context, g *grouppb.GroupId) (*grouppb.Group, error) {
	if g == nil {
		return nil, fmt.Errorf("must pass a non-nil group id to getGroupByID")
	}

	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetGroup(ctx, &grouppb.GetGroupRequest{
		GroupId:             g,
		SkipFetchingMembers: true,
	})
	if err != nil {
		return nil, err
	}
	if res.Status == nil {
		return nil, errors.New("Did not get status from getGroupByID")
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, fmt.Errorf("failed to get group by ID, with code %s and message %s", res.Status.Code, res.Status.Message)
	}

	return res.Group, nil
}

func groupByResourceID(shares []*gateway.ShareResourceInfo, publicShares []*gateway.PublicShareResourceInfo) (map[string][]*GenericShare, map[string]*provider.ResourceInfo) {
	grouped := make(map[string][]*GenericShare, len(shares)+len(publicShares)) // at most we have the sum of both lists
	infos := make(map[string]*provider.ResourceInfo, len(shares)+len(publicShares))

	for _, s := range shares {
		id := spaces.ResourceIdToString(s.Share.ResourceId)
		grouped[id] = append(grouped[id], &GenericShare{
			shareType: ShareTypeShare,
			ID:        s.Share.Id.OpaqueId,
			share:     s.Share,
		})
		infos[id] = s.ResourceInfo // all shares of the same resource are assumed to have the same ResourceInfo payload, here we take the last
	}

	for _, s := range publicShares {
		id := spaces.ResourceIdToString(s.PublicShare.ResourceId)
		grouped[id] = append(grouped[id], &GenericShare{
			shareType: ShareTypeLink,
			ID:        s.PublicShare.Id.OpaqueId,
			link:      s.PublicShare,
		})
		infos[id] = s.ResourceInfo
	}

	return grouped, infos
}

func (s *svc) getSharedByMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		handleError(ctx, fmt.Errorf("No user in context"), http.StatusUnauthorized, w)
		return
	}

	shares, err := gw.ListExistingShares(ctx, &collaboration.ListSharesRequest{
		Filters: []*collaboration.Filter{
			{
				Type: collaboration.Filter_TYPE_CREATOR,
				Term: &collaboration.Filter_Creator{
					Creator: user.Id,
				},
			},
		},
	})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	publicShares, err := gw.ListExistingPublicShares(ctx, &link.ListPublicSharesRequest{
		Filters: []*link.ListPublicSharesRequest_Filter{
			{
				Type: link.ListPublicSharesRequest_Filter_TYPE_CREATOR,
				Term: &link.ListPublicSharesRequest_Filter_Creator{
					Creator: user.Id,
				},
			},
		},
	})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	grouped, infos := groupByResourceID(shares.ShareInfos, publicShares.ShareInfos)

	// convert to libregraph share drives
	shareDrives := make([]*libregraph.DriveItem, 0, len(grouped))
	for id, shares := range grouped {
		info := infos[id]
		drive, err := s.cs3ShareToDriveItem(ctx, info, shares)
		if err != nil {
			log.Error().Err(err).Msg("error getting received shares")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		shareDrives = append(shareDrives, drive)
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shareDrives,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling shares as json")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
}

func (s *svc) updateReceivedShare(w http.ResponseWriter, r *http.Request) {
	// :(
	shareJailID := "a0ca6a90-a365-4782-871e-d44447bbc668"

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	// We extract the ShareID from the request
	// Which is wrapped in some ugly form with the ShareJail unfortunately ...
	spaceID := chi.URLParam(r, "space-id")
	spaceID, _ = url.QueryUnescape(spaceID)

	if spaceID != fmt.Sprintf("%s$%s", shareJailID, shareJailID) {
		handleError(ctx, fmt.Errorf("spaceID for this share not found"), http.StatusNotFound, w)
		return
	}

	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)

	parts := strings.Split(resourceID, "!")
	if len(parts) != 2 {
		handleError(ctx, fmt.Errorf("Invalid resource ID"), http.StatusBadRequest, w)
		return
	}

	spaceID, shareID := parts[0], parts[1]
	if spaceID != fmt.Sprintf("%s$%s", shareJailID, shareJailID) {
		handleError(ctx, fmt.Errorf("spaceID for this share not found"), http.StatusNotFound, w)
		return
	}

	// Now we decode the request body
	req := &libregraph.DriveItem{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err = dec.Decode(req); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		handleError(ctx, err, http.StatusBadRequest, w)
		return
	}

	if req.UIHidden == nil {
		handleError(ctx, fmt.Errorf("Must provide @UI.Hidden when updating received share"), http.StatusBadRequest, w)
		return
	}

	shareRequest := &collaboration.UpdateReceivedShareRequest{
		Share: &collaboration.ReceivedShare{
			Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: shareID}},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	}
	if *req.UIHidden {
		shareRequest.Share.State = collaboration.ShareState_SHARE_STATE_REJECTED
	} else {
		shareRequest.Share.State = collaboration.ShareState_SHARE_STATE_ACCEPTED
	}

	shareRes, err := gw.UpdateReceivedShare(ctx, shareRequest)
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	if shareRes.Status == nil || shareRes.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, shareRes.Status, "ocgraph: failed to update received share", w)
		return
	}

	// We also need the ResourceInfo to turn this into a DriveItem for the response
	statRes, err := gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			ResourceId: shareRes.Share.Share.ResourceId,
		},
	})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
	if statRes.Status == nil || statRes.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, statRes.Status, "ocgraph: failed to stat resource behind received share", w)
		return
	}

	drive, err := s.cs3ReceivedShareToDriveItem(ctx, &gateway.ReceivedShareResourceInfo{
		ReceivedShare: shareRes.Share,
		ResourceInfo:  statRes.Info,
	})
	if err != nil {
		handleError(ctx, fmt.Errorf("Error converting ReceivedShare to DriveItem"), http.StatusInternalServerError, w)
		return
	}

	if err := json.NewEncoder(w).Encode(drive); err != nil {
		log.Error().Err(err).Msg("error marshalling ReceivedShare as json")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
}
