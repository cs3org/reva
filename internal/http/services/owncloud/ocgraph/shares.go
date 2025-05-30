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
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/utils"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

func (s *svc) getSharedWithMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resShares, err := gw.ListExistingReceivedShares(ctx, &collaborationv1beta1.ListReceivedSharesRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error getting received shares")
		w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// We extract the inode and storage ID from the request
	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
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
		handleError(err, w)
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
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// From this, we first extract the requested role, which we translate into permissions
	roles := invite.Roles
	if len(roles) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Reva expects exaclty one role"))
		return
	}
	role, ok := UnifiedRoleIDToDefinition(roles[0])
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid role"))
		return
	}
	perms := PermissionsToCS3ResourcePermissions(role.RolePermissions)

	// Then we also set an expiry, if needed
	var exp *types.Timestamp
	if invite.ExpirationDateTime != nil {
		exp = &types.Timestamp{
			Seconds: uint64(invite.ExpirationDateTime.Unix()),
		}
	}

	// TODO: validate that user is allowed to do this? Or handled by interceptor?

	// We keep a list of users to who we have sent the
	identitySet := &libregraph.SharePointIdentitySet{}

	// Finally, we create the actual share for every requested recipient
	for _, recepient := range invite.Recipients {
		grantee, err := toGrantee(*recepient.LibreGraphRecipientType, *recepient.ObjectId)
		if err != nil {
			log.Error().Err(err).Msg("invalid recipient type passed")
			w.WriteHeader(http.StatusBadRequest)
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
			},
			Grant: &collaborationv1beta1.ShareGrant{
				Grantee:    grantee,
				Expiration: exp,
				Permissions: &collaborationv1beta1.SharePermissions{
					Permissions: perms,
				},
			},
		}

		resp, err := gw.CreateShare(ctx, createShareRequest)
		if err != nil {
			handleError(err, w)
			return
		}
		if resp.Status.Code != rpcv1beta1.Code_CODE_OK {
			handleRpcStatus(ctx, resp.Status, w)
			return
		}

		if grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
			identitySet.SetUser(*libregraph.NewIdentity(grantee.GetUserId().OpaqueId))
		} else if grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			identitySet.SetGroup(*libregraph.NewIdentity(grantee.GetGroupId().OpaqueId))
		}

	}

	lgPerm := libregraph.Permission{
		Roles:       roles,
		GrantedToV2: identitySet,
	}
	_ = json.NewEncoder(w).Encode(&ListResponse{
		Value: lgPerm,
	})

}

func (s *svc) createLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	// First we get the gateway client
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// We extract the inode and storage ID from the request
	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
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
		handleError(err, w)
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
		w.WriteHeader(http.StatusBadRequest)
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

	// TODO: validate that user is allowed to do this? Or handled by interceptor?

	if linkRequest.Type == nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Must pass a link type"))
		return
	}

	req := &link.CreatePublicShareRequest{
		ResourceInfo: statRes.Info,
		Grant: &link.Grant{
			Expiration: exp,
			Password:   password,
			Permissions: &link.PublicSharePermissions{
				Permissions: LinkTypeToPermissions(*linkRequest.Type),
			},
		},
	}

	resp, err := gw.CreatePublicShare(ctx, req)
	if err != nil {
		handleError(err, w)
		return
	}
	if resp.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, resp.Status, w)
		return
	}

	lgPerm := s.libreGraphPermissionFromCS3PublicShare(resp.GetShare())
	if lgPerm == nil {
		log.Error().Err(err).Any("link", resp.GetShare()).Any("lgPerm", lgPerm).Msg("error converting created link to permissions")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(lgPerm)

}

func encodeSpaceIDForShareJail(res *provider.ResourceInfo) string {
	return spaces.EncodeResourceID(res.Id)
	//return spaces.EncodeStorageSpaceID(res.Id.StorageId, res.Path)
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

	roles := make([]string, 0, 1)
	role := CS3ResourcePermissionsToUnifiedRole(rsi.ResourceInfo.PermissionSet)
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
			DriveId:   libregraph.PtrString(fmt.Sprintf("%s$%s", ShareJailID, ShareJailID)),
			DriveType: libregraph.PtrString("virtual"),
			Id:        libregraph.PtrString(fmt.Sprintf("%s$%s!%s", ShareJailID, ShareJailID, ShareJailID)),
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
			Path:                 libregraph.PtrString(spaces.RelativePathToSpaceID(rsi.ResourceInfo)),
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

func (s *svc) getUserByID(ctx context.Context, u *userv1beta1.UserId) (*userv1beta1.User, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetUser(ctx, &userv1beta1.GetUserRequest{
		UserId: u,
	})
	if err != nil {
		return nil, err
	}

	return res.User, nil
}

func (s *svc) getGroupByID(ctx context.Context, g *groupv1beta1.GroupId) (*groupv1beta1.Group, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}

	res, err := client.GetGroup(ctx, &groupv1beta1.GetGroupRequest{
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

type share struct {
	share  *collaborationv1beta1.Share
	public *link.PublicShare
}

func groupByResourceID(shares []*gateway.ShareResourceInfo, publicShares []*gateway.PublicShareResourceInfo) (map[string][]*share, map[string]*provider.ResourceInfo) {
	grouped := make(map[string][]*share, len(shares)+len(publicShares)) // at most we have the sum of both lists
	infos := make(map[string]*provider.ResourceInfo, len(shares)+len(publicShares))

	for _, s := range shares {
		id := spaces.ResourceIdToString(s.Share.ResourceId)
		grouped[id] = append(grouped[id], &share{
			share: s.Share,
		})
		infos[id] = s.ResourceInfo // all shares of the same resource are assumed to have the same ResourceInfo payload, here we take the last
	}

	for _, s := range publicShares {
		id := spaces.ResourceIdToString(s.PublicShare.ResourceId)
		grouped[id] = append(grouped[id], &share{
			public: s.PublicShare,
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	shares, err := gw.ListExistingShares(ctx, &collaborationv1beta1.ListSharesRequest{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	publicShares, err := gw.ListExistingPublicShares(ctx, &link.ListPublicSharesRequest{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		shareDrives = append(shareDrives, drive)
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shareDrives,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling shares as json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *svc) cs3ShareToDriveItem(ctx context.Context, info *provider.ResourceInfo, shares []*share) (*libregraph.DriveItem, error) {

	parentRelativePath := path.Dir(spaces.RelativePathToSpaceID(info))

	permissions, err := s.cs3sharesToPermissions(ctx, shares)
	if err != nil {
		return nil, err
	}

	d := &libregraph.DriveItem{
		ETag:                 libregraph.PtrString(info.Etag),
		Id:                   libregraph.PtrString(spaces.EncodeResourceID(info.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(info.Mtime)),
		Name:                 libregraph.PtrString(info.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId: libregraph.PtrString(spaces.EncodeStorageSpaceID(info.Id.StorageId, info.Id.SpaceId)),
			// DriveType: libregraph.PtrString(info.Space.SpaceType),
			Id:   libregraph.PtrString(spaces.EncodeResourceID(info.ParentId)),
			Name: libregraph.PtrString(path.Base(parentRelativePath)),
			Path: libregraph.PtrString(parentRelativePath),
		},
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

func (s *svc) cs3sharesToPermissions(ctx context.Context, shares []*share) ([]libregraph.Permission, error) {
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
			role := CS3ResourcePermissionsToUnifiedRole(e.share.Permissions.Permissions)
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
		} else if e.public != nil {
			createdTime := utils.TSToTime(e.public.Ctime)
			linktype, _ := SharingLinkTypeFromCS3Permissions(e.public.Permissions)

			permissions = append(permissions, libregraph.Permission{
				CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
				HasPassword:     libregraph.PtrBool(e.public.PasswordProtected),
				Id:              libregraph.PtrString(e.public.Token),
				Link: &libregraph.SharingLink{
					LibreGraphDisplayName: libregraph.PtrString(e.public.DisplayName),
					LibreGraphQuickLink:   libregraph.PtrBool(e.public.Quicklink),
					PreventsDownload:      libregraph.PtrBool(false),
					Type:                  linktype,
					// WebUrl:                libregraph.PtrString(""),
				},
			})
		}
	}

	return permissions, nil
}

func toGrantee(recipientType string, id string) (*provider.Grantee, error) {
	switch recipientType {
	case "user":
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: id}},
		}, nil
	case "group":
		return &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			Id:   &provider.Grantee_GroupId{GroupId: &groupv1beta1.GroupId{OpaqueId: id}},
		}, nil
	default:
		return nil, errors.New(recipientType + " is not a valid granteetype")
	}
}
