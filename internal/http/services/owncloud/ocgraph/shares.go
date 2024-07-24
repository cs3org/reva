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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
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

	shares := make([]*libregraph.DriveItem, 0, len(resShares.Shares))
	for _, share := range resShares.Shares {
		drive, err := s.cs3ReceivedShareToDriveItem(ctx, share)
		if err != nil {
			log.Error().Err(err).Msg("error getting received shares")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		shares = append(shares, drive)
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": shares,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling shares as json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func encodeSpaceIDForShareJail(res *provider.ResourceInfo) string {
	return spaces.EncodeSpaceID(res.Id.StorageId, res.Path)
}

func (s *svc) cs3ReceivedShareToDriveItem(ctx context.Context, share *gateway.SharedResourceInfo) (*libregraph.DriveItem, error) {
	createdTime := utils.TSToTime(share.Share.Share.Ctime)

	creator, err := s.getUserByID(ctx, share.Share.Share.Creator)
	if err != nil {
		return nil, err
	}

	grantee, err := s.cs3GranteeToSharePointIdentitySet(ctx, share.Share.Share.Grantee)
	if err != nil {
		return nil, err
	}

	d := &libregraph.DriveItem{
		UIHidden:          libregraph.PtrBool(share.Share.Hidden),
		ClientSynchronize: libregraph.PtrBool(true),
		CreatedBy: &libregraph.IdentitySet{
			User: &libregraph.Identity{
				DisplayName: creator.DisplayName,
				Id:          libregraph.PtrString(creator.Id.OpaqueId),
			},
		},
		ETag:                 &share.ResourceInfo.Etag,
		Id:                   libregraph.PtrString(libregraphShareID(share.Share.Share.Id)),
		LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(share.ResourceInfo.Mtime)),
		Name:                 libregraph.PtrString(share.ResourceInfo.Name),
		ParentReference: &libregraph.ItemReference{
			DriveId:   libregraph.PtrString(fmt.Sprintf("%s$%s", shareJailID, shareJailID)),
			DriveType: libregraph.PtrString("virtual"),
			Id:        libregraph.PtrString(fmt.Sprintf("%s$%s!%s", shareJailID, shareJailID, shareJailID)),
		},
		RemoteItem: &libregraph.RemoteItem{
			CreatedBy: &libregraph.IdentitySet{
				User: &libregraph.Identity{
					DisplayName: creator.DisplayName,
					Id:          libregraph.PtrString(creator.Id.OpaqueId),
				},
			},
			ETag: &share.ResourceInfo.Etag,
			File: &libregraph.OpenGraphFile{
				MimeType: &share.ResourceInfo.MimeType,
			},
			Id:                   libregraph.PtrString(encodeSpaceIDForShareJail(share.ResourceInfo)),
			LastModifiedDateTime: libregraph.PtrTime(utils.TSToTime(share.ResourceInfo.Mtime)),
			Name:                 libregraph.PtrString(share.ResourceInfo.Name),
			// ParentReference: &libregraph.ItemReference{
			// 	DriveId:   libregraph.PtrString(spaces.EncodeResourceID(share.ResourceInfo.ParentId)),
			// 	DriveType: nil, // FIXME: no way to know it unless we hardcode it
			// },
			Permissions: []libregraph.Permission{
				{
					CreatedDateTime: *libregraph.NewNullableTime(&createdTime),
					GrantedToV2:     grantee,
					Id:              nil, // TODO: what is this??
					Invitation: &libregraph.SharingInvitation{
						InvitedBy: &libregraph.IdentitySet{
							User: &libregraph.Identity{
								DisplayName: creator.DisplayName,
								Id:          libregraph.PtrString(creator.Id.OpaqueId),
							},
						},
					},
					Roles: []string{"2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"}, // TODO: find a way to not hardcode it
					// TODO: roles are missing, but which is the id???
					// "roles": [
					//     "2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"
					// ]
				},
			},
			Size: libregraph.PtrInt64(int64(share.ResourceInfo.Size)),
		},
		Size: libregraph.PtrInt64(int64(share.ResourceInfo.Size)),
	}

	if share.ResourceInfo.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		d.Folder = libregraph.NewFolder()
	} else {
		d.File = &libregraph.OpenGraphFile{
			MimeType: &share.ResourceInfo.MimeType,
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
