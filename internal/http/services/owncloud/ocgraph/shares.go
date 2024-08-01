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
	"path"
	"strings"

	"github.com/alitto/pond"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
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
	return spaces.EncodeResourceID(res.Id)
	//return spaces.EncodeSpaceID(res.Id.StorageId, res.Path)
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

	roles := make([]string, 0, 1)
	role := CS3ResourcePermissionsToUnifiedRole(share.ResourceInfo.PermissionSet)
	if role != nil {
		roles = append(roles, *role.Id)
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
			Path:                 libregraph.PtrString(relativePathToSpaceID(share.ResourceInfo)),
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
					Roles: roles,
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

type share struct {
	share  *collaborationv1beta1.Share
	public *linkv1beta1.PublicShare
}

func resourceIdToString(id *provider.ResourceId) string {
	return fmt.Sprintf("%s!%s", id.StorageId, id.OpaqueId)
}

func resourceIdFromString(s string) *provider.ResourceId {
	parts := strings.Split(s, "!")
	return &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}
}

func groupByResourceID(shares []*collaborationv1beta1.Share, publicShares []*linkv1beta1.PublicShare) map[string][]*share {
	m := make(map[string][]*share, len(shares)+len(publicShares)) // at most we have the sum of both lists

	for _, s := range shares {
		id := resourceIdToString(s.ResourceId)
		m[id] = append(m[id], &share{
			share: s,
		})
	}

	for _, s := range publicShares {
		id := resourceIdToString(s.ResourceId)
		m[id] = append(m[id], &share{
			public: s,
		})
	}

	return m
}

type pair[T, V any] struct {
	First  T
	Second V
}

func (s *svc) getSharedByMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		// TODO
		return
	}

	shares, err := gw.ListShares(ctx, &collaborationv1beta1.ListSharesRequest{})
	if err != nil {
		// TODO
		return
	}

	publicShares, err := gw.ListPublicShares(ctx, &linkv1beta1.ListPublicSharesRequest{})
	if err != nil {
		// TODO
		return
	}

	grouped := groupByResourceID(shares.Shares, publicShares.Share)

	// resolve all the resource ids
	res := make(chan *pair[string, *provider.ResourceInfo], len(grouped))
	pool := pond.New(50, len(grouped))
	for id := range grouped {
		id := id

		// TODO (gdelmont): we should report any eventual error raised by the goroutines
		pool.Submit(func() {
			stat, err := gw.Stat(ctx, &provider.StatRequest{
				Ref: &provider.Reference{
					ResourceId: resourceIdFromString(id),
				},
			})
			if err != nil {
				return
			}
			if stat.Status.Code != rpcv1beta1.Code_CODE_OK {
				return
			}

			res <- &pair[string, *provider.ResourceInfo]{
				First:  id,
				Second: stat.Info,
			}
		})
	}

	infos := make(map[string]*provider.ResourceInfo)
	done := make(chan struct{})
	go func() {
		for s := range res {
			infos[s.First] = s.Second
		}
		done <- struct{}{}
	}()
	pool.StopAndWait()
	close(res)
	<-done
	close(done)

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

	parentRelativePath := path.Dir(relativePathToSpaceID(info))

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
			DriveId: libregraph.PtrString(spaces.EncodeSpaceID(info.Id.StorageId, info.Id.SpaceId)),
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
				Id:              nil, // TODO: what is this??
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
					LibreGraphDisplayName: libregraph.PtrString("Link"),
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
