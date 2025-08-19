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
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils"
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

	resShares, err := gw.ListExistingReceivedShares(ctx, &collaborationv1beta1.ListReceivedSharesRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error getting received shares")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	shares := make([]*libregraph.DriveItem, 0, len(resShares.ShareInfos))
	for _, share := range resShares.ShareInfos {
		drive, err := s.cs3ReceivedShareToDriveItem(ctx, share)
		if err != nil {
			log.Error().Err(err).Any("share", share).Msg("error parsing received share, ignoring")
		} else {
			shares = append(shares, drive)
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
	if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, statRes.Status, w)
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

		grantee, err := s.toGrantee(ctx, *recipient.LibreGraphRecipientType, *recipient.ObjectId)
		if err != nil {
			log.Error().Err(err).Msg("invalid recipient type passed")
			handleError(ctx, err, http.StatusBadRequest, w)
			return
		}

		createShareRequest := &collaborationv1beta1.CreateShareRequest{
			ResourceInfo: &provider.ResourceInfo{
				Id: &provider.ResourceId{
					StorageId: storageID,
					OpaqueId:  itemID,
				},
				Path:  path,
				Owner: owner,
				Type:  statRes.Info.Type,
			},
			Grant: &collaborationv1beta1.ShareGrant{
				Grantee:    grantee,
				Expiration: exp,
				Permissions: &collaborationv1beta1.SharePermissions{
					Permissions: requestedPerms,
				},
			},
		}

		resp, err := gw.CreateShare(ctx, createShareRequest)
		if err != nil {
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		if resp.Status.Code != rpcv1beta1.Code_CODE_OK {
			handleRpcStatus(ctx, resp.Status, w)
			return
		}

		lgPerm, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
			shareType: "share",
			share:     resp.GetShare(),
			ID:        resp.GetShare().GetId().GetOpaqueId(),
		})
		if err != nil || lgPerm == nil {
			log.Error().Err(err).Any("share", resp.GetShare()).Err(err).Any("lgPerm", lgPerm).Msg("error converting created share to permissions")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}

		response = append(response, lgPerm)

	}

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
	if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, statRes.Status, w)
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
	if resp.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, resp.Status, w)
		return
	}

	lgPerm, err := s.shareToLibregraphPerm(ctx, &ShareOrLink{
		shareType: "link",
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

func (s *svc) cs3ReceivedShareToDriveItem(ctx context.Context, rsi *gateway.ReceivedShareResourceInfo) (*libregraph.DriveItem, error) {
	createdTime := utils.TSToTime(rsi.ReceivedShare.Share.Ctime)

	creator, err := s.getUserByID(ctx, rsi.ReceivedShare.Share.Creator)
	if err != nil {
		return nil, err
	}

	grantee, err := s.cs3GranteeToSharePointIdentitySet(ctx, rsi.ReceivedShare.Share.Grantee)
	if err != nil {
		return nil, err
	}

	relativePath, err := spaces.PathRelativeToSpaceRoot(rsi.ResourceInfo)
	if err != nil {
		return nil, err
	}

	roles := make([]string, 0, 1)
	role := CS3ResourcePermissionsToUnifiedRole(ctx, rsi.ResourceInfo.PermissionSet)
	if role != nil {
		roles = append(roles, *role.Id)
	}

	d := &libregraph.DriveItem{
		UIHidden:          libregraph.PtrBool(rsi.ReceivedShare.Hidden),
		ClientSynchronize: libregraph.PtrBool(true),
		CreatedBy: &libregraph.IdentitySet{
			User: &libregraph.Identity{
				DisplayName: creator.DisplayName,
				Id:          libregraph.PtrString(creator.Id.OpaqueId),
			},
		},
		ETag:                 &rsi.ResourceInfo.Etag,
		Id:                   libregraph.PtrString(libregraphShareID(rsi.ReceivedShare.Share.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(rsi.ResourceInfo.Mtime)),
		Name:                 libregraph.PtrString(rsi.ResourceInfo.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId:   libregraph.PtrString(spaces.ConcatStorageSpaceID(ShareJailID, ShareJailID)),
			DriveType: libregraph.PtrString("virtual"),
			Id:        libregraph.PtrString(spaces.EncodeResourceID(&provider.ResourceId{OpaqueId: ShareJailID, StorageId: ShareJailID, SpaceId: ShareJailID})),
		},
		RemoteItem: &libregraph.RemoteItem{
			CreatedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					DisplayName: creator.DisplayName,
					Id:          libregraph.PtrString(creator.Id.OpaqueId),
				},
			},
			ETag: &rsi.ResourceInfo.Etag,
			File: &libregraph.OpenGraphFile{
				MimeType: &rsi.ResourceInfo.MimeType,
			},
			Id:                   libregraph.PtrString(encodeSpaceIDForShareJail(rsi.ResourceInfo)),
			LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(rsi.ResourceInfo.Mtime)),
			Name:                 libregraph.PtrString(rsi.ResourceInfo.Name),
			Path:                 libregraph.PtrString(relativePath),
			WebUrl:               libregraph.PtrString(filepath.Join(s.c.WebBase, rsi.ResourceInfo.Path)),
			// ParentReference: &libregraph.ItemReference{
			// 	DriveId:   libregraph.PtrString(spaces.EncodeResourceID(share.ResourceInfo.ParentId)),
			// 	DriveType: nil, // FIXME: no way to know it unless we hardcode it
			// },
			Permissions: []libregraph.Permission{
				{
					CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
					GrantedToV2:     grantee,
					Invitation: &libregraph.SharingInvitation{
						InvitedBy: &libregraph.IdentitySet{
							User: &libregraph.Identity{
								DisplayName: creator.DisplayName,
								Id:          libregraph.PtrString(creator.Id.OpaqueId),
							},
						},
					},
					Roles: roles,
				},
			},
			Size: libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
		},
		Size: libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
	}

	if rsi.ResourceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	} else {
		d.File = &libregraph.OpenGraphFile{
			MimeType: &rsi.ResourceInfo.MimeType,
		}
	}

	return d, nil
}

func (s *svc) getUserByID(ctx context.Context, u *userpb.UserId) (*userpb.User, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId: u,
	})
	if err != nil {
		return nil, err
	}

	return res.User, nil
}

func (s *svc) getGroupByID(ctx context.Context, g *grouppb.GroupId) (*grouppb.Group, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetGroup(ctx, &grouppb.GetGroupRequest{
		GroupId: g,
	})
	if err != nil {
		return nil, err
	}

	return res.Group, nil
}

func (s *svc) cs3GranteeToSharePointIdentitySet(ctx context.Context, grantee *provider.Grantee) (*libregraph.SharePointIdentitySet, error) {
	p := &libregraph.SharePointIdentitySet{}

	if u := grantee.GetUserId(); u != nil {
		user, err := s.getUserByID(ctx, u)
		if err != nil {
			return nil, err
		}
		p.User = &libregraph.Identity{
			DisplayName: user.DisplayName,
			Id:          libregraph.PtrString(u.OpaqueId),
		}
	} else if g := grantee.GetGroupId(); g != nil {
		group, err := s.getGroupByID(ctx, g)
		if err != nil {
			return nil, err
		}
		p.Group = &libregraph.Identity{
			DisplayName: group.DisplayName,
			Id:          libregraph.PtrString(g.OpaqueId),
		}
	}

	return p, nil
}

func groupByResourceID(shares []*gateway.ShareResourceInfo, publicShares []*gateway.PublicShareResourceInfo) (map[string][]*ShareOrLink, map[string]*provider.ResourceInfo) {
	grouped := make(map[string][]*ShareOrLink, len(shares)+len(publicShares)) // at most we have the sum of both lists
	infos := make(map[string]*provider.ResourceInfo, len(shares)+len(publicShares))

	for _, s := range shares {
		id := spaces.ResourceIdToString(s.Share.ResourceId)
		grouped[id] = append(grouped[id], &ShareOrLink{
			shareType: "share",
			ID:        s.Share.Id.OpaqueId,
			share:     s.Share,
		})
		infos[id] = s.ResourceInfo // all shares of the same resource are assumed to have the same ResourceInfo payload, here we take the last
	}

	for _, s := range publicShares {
		id := spaces.ResourceIdToString(s.PublicShare.ResourceId)
		grouped[id] = append(grouped[id], &ShareOrLink{
			shareType: "link",
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

	shares, err := gw.ListExistingShares(ctx, &collaborationv1beta1.ListSharesRequest{})
	if err != nil {
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	publicShares, err := gw.ListExistingPublicShares(ctx, &link.ListPublicSharesRequest{})
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

func (s *svc) cs3ShareToDriveItem(ctx context.Context, info *provider.ResourceInfo, shares []*ShareOrLink) (*libregraph.DriveItem, error) {
	relativePath, err := spaces.PathRelativeToSpaceRoot(info)
	if err != nil {
		return nil, err
	}

	parentRelativePath := path.Dir(relativePath)
	if parentRelativePath == "." {
		parentRelativePath = ""
	}

	permissions, err := s.cs3sharesToPermissions(ctx, shares)
	if err != nil {
		return nil, err
	}

	if info.ParentId.SpaceId == "" {
		info.ParentId.SpaceId = spaces.PathToSpaceID(info.Path)
	}

	d := &libregraph.DriveItem{
		ETag:                 libregraph.PtrString(info.Etag),
		Id:                   libregraph.PtrString(spaces.EncodeResourceID(info.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(info.Mtime)),
		Name:                 libregraph.PtrString(info.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId: libregraph.PtrString(spaces.ConcatStorageSpaceID(info.ParentId.StorageId, info.ParentId.SpaceId)),
			// DriveType: libregraph.PtrString(info.Space.SpaceType),
			Id:   libregraph.PtrString(spaces.EncodeResourceID(info.ParentId)),
			Name: libregraph.PtrString(path.Base(relativePath)),
			Path: libregraph.PtrString(parentRelativePath),
		},
		WebUrl:      libregraph.PtrString(filepath.Join(s.c.WebBase, info.Path)),
		Permissions: permissions,

		Size: libregraph.PtrInt64(int64(info.Size)),
	}

	if info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	} else {
		d.File = &libregraph.OpenGraphFile{
			MimeType: &info.MimeType,
		}
	}

	return d, nil
}

func (s *svc) cs3sharesToPermissions(ctx context.Context, shares []*ShareOrLink) ([]libregraph.Permission, error) {
	permissions := make([]libregraph.Permission, 0, len(shares))

	for _, e := range shares {
		if e.share != nil {
			createdTime := utils.TSToTime(e.share.Ctime)

			creator, err := s.getUserByID(ctx, e.share.Creator)
			if err != nil {
				return nil, err
			}

			grantee, err := s.cs3GranteeToSharePointIdentitySet(ctx, e.share.Grantee)
			if err != nil {
				return nil, err
			}

			roles := make([]string, 0, 1)
			role := CS3ResourcePermissionsToUnifiedRole(ctx, e.share.Permissions.Permissions)
			if role != nil {
				roles = append(roles, *role.Id)
			}
			permissions = append(permissions, libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				GrantedToV2:     grantee,
				Invitation: &libregraph.SharingInvitation{
					InvitedBy: &libregraph.IdentitySet{
						User: &libregraph.Identity{
							DisplayName: creator.DisplayName,
							Id:          libregraph.PtrString(creator.Id.OpaqueId),
						},
					},
				},
				Roles: roles,
			})
		} else if e.link != nil {
			createdTime := utils.TSToTime(e.link.Ctime)
			linktype, _ := SharingLinkTypeFromCS3Permissions(ctx, e.link.Permissions)

			permissions = append(permissions, libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				HasPassword:     libregraph.PtrBool(e.link.PasswordProtected),
				Id:              libregraph.PtrString(e.link.Token),
				Link: &libregraph.SharingLink{
					LibreGraphDisplayName: libregraph.PtrString(e.link.DisplayName),
					LibreGraphQuickLink:   libregraph.PtrBool(e.link.Quicklink),
					PreventsDownload:      libregraph.PtrBool(false),
					Type:                  linktype,
					// WebUrl:                libregraph.PtrString(""),
				},
			})
		}
	}

	return permissions, nil
}

func (s *svc) toGrantee(ctx context.Context, recipientType string, id string) (*provider.Grantee, error) {
	gw, err := s.getClient()
	if err != nil {
		return nil, err
	}

	switch recipientType {
	case "user":
		userRes, err := gw.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
			Claim:                  "username",
			Value:                  id,
			SkipFetchingUserGroups: true,
		})
		if err != nil || userRes.Status == nil || userRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return nil, errors.New("failed to fetch sharee data")
		}
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: userRes.User.GetId()},
		}, nil
	case "group":
		groupRes, err := gw.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
			Claim:               "group_name",
			Value:               id,
			SkipFetchingMembers: true,
		})
		if err != nil || groupRes.Status == nil || groupRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return nil, errors.New("failed to fetch sharee data")
		}
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: groupRes.Group.GetId()},
		}, nil
	default:
		return nil, errors.New(recipientType + " is not a valid granteetype")
	}
}
