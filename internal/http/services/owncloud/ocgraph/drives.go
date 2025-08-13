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
	"path/filepath"
	"strings"
	"time"

	"github.com/CiscoM31/godata"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils/list"
	gomime "github.com/glpatcern/go-mime"
	"github.com/go-chi/chi/v5"
	libregraph "github.com/owncloud/libre-graph-api-go"

	"github.com/pkg/errors"
)

func (s *svc) listMySpaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	odataReq, err := godata.ParseRequest(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get drives: query error")
		handleError(ctx, err, http.StatusBadRequest, w)
		return
	}

	var spaces []*libregraph.Drive
	if isMountpointRequest(odataReq) {
		spaces, err = s.getDrivesForShares(ctx, gw)
		if err != nil {
			log.Error().Err(err).Msg("error getting share spaces")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
	} else {
		filters, err := generateCs3StorageSpaceFilters(odataReq)
		if err != nil {
			log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get drives: error parsing filters")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}

		res, err := gw.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
			Filters: filters,
		})
		if err != nil {
			log.Error().Err(err).Msg("error listing storage spaces")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		if res.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(res.Status.Code)).Str("message", res.Status.Message).Msg("error listing storage spaces")
			handleRpcStatus(ctx, res.Status, w)
			return
		}

		me := appctx.ContextMustGetUser(ctx)
		spaces = list.Map(res.StorageSpaces, func(space *provider.StorageSpace) *libregraph.Drive {
			return s.cs3StorageSpaceToDrive(ctx, me, space)
		})
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": spaces,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling spaces as json")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}
}

func isMountpointRequest(request *godata.GoDataRequest) bool {
	if request.Query.Filter == nil {
		return false
	}
	if request.Query.Filter.Tree.Token.Value != "eq" {
		return false
	}
	return request.Query.Filter.Tree.Children[0].Token.Value == "driveType" && strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'") == "mountpoint"
}

const ShareJailID = "a0ca6a90-a365-4782-871e-d44447bbc668"

func (s *svc) getDrivesForShares(ctx context.Context, gw gateway.GatewayAPIClient) ([]*libregraph.Drive, error) {
	res, err := gw.ListExistingReceivedShares(ctx, &collaborationv1beta1.ListReceivedSharesRequest{})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	spacesRes := make([]*libregraph.Drive, 0, len(res.ShareInfos))
	for _, share := range res.ShareInfos {
		spacesRes = append(spacesRes, s.convertShareToSpace(share))
	}
	return spacesRes, nil
}

func libregraphShareID(shareID *collaborationv1beta1.ShareId) string {
	return fmt.Sprintf("%s$%s!%s", ShareJailID, ShareJailID, shareID.OpaqueId)
}

func (s *svc) convertShareToSpace(rsi *gateway.ReceivedShareResourceInfo) *libregraph.Drive {
	// the prefix of the remote_item.id and rootid
	spacePath, _ := spaces.ResourceToSpacePath(rsi.ResourceInfo)
	resourceRelativePath, _ := spaces.PathRelativeToSpaceRoot(rsi.ResourceInfo)

	return &libregraph.Drive{
		Id:         libregraph.PtrString(libregraphShareID(rsi.ReceivedShare.Share.Id)),
		DriveType:  libregraph.PtrString("mountpoint"),
		DriveAlias: libregraph.PtrString(rsi.ReceivedShare.Share.Id.OpaqueId), // this is not used, but must not be the same alias as the drive item
		Name:       filepath.Base(rsi.ResourceInfo.Path),
		WebUrl:     libregraph.PtrString(fullURL(s.c.WebBase, rsi.ResourceInfo.Path)),
		Quota: &libregraph.Quota{
			Total:     libregraph.PtrInt64(24154390300000),
			Used:      libregraph.PtrInt64(3141592),
			Remaining: libregraph.PtrInt64(24154387158408),
		},
		Root: &libregraph.DriveItem{
			Id:        libregraph.PtrString(fmt.Sprintf("%s$%s!%s", ShareJailID, ShareJailID, rsi.ReceivedShare.Share.Id.OpaqueId)),
			WebDavUrl: libregraph.PtrString(fullURL(s.c.WebDavBase, rsi.ResourceInfo.Path)),
			RemoteItem: &libregraph.RemoteItem{
				// Drive Alias does not contain the first '/'
				DriveAlias: libregraph.PtrString(spacePath[1:]),
				ETag:       libregraph.PtrString(rsi.ResourceInfo.Etag),
				Folder:     &libregraph.Folder{},
				// The Id must correspond to the id in the OCS response, for the time being
				// It is in the form <something>!<something-else>
				Id:                   libregraph.PtrString(spaces.EncodeResourceID(rsi.ResourceInfo.Id)),
				LastModifiedDateTime: libregraph.PtrTime(time.Unix(int64(rsi.ResourceInfo.Mtime.Seconds), int64(rsi.ResourceInfo.Mtime.Nanos))),
				Name:                 libregraph.PtrString(filepath.Base(rsi.ResourceInfo.Path)),
				Path:                 libregraph.PtrString(resourceRelativePath),
				// RootId must have the same token before ! as Id
				// the second part for the time being is not used
				RootId: libregraph.PtrString(fmt.Sprintf("%s$%s!unused_root_id", rsi.ResourceInfo.Id.StorageId, rsi.ResourceInfo.Id.SpaceId)),
				Size:   libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
			},
		},
		Special: []libregraph.DriveItem{},
	}
}

func generateCs3StorageSpaceFilters(request *godata.GoDataRequest) ([]*provider.ListStorageSpacesRequest_Filter, error) {
	var filters spaces.ListStorageSpaceFilter
	if request.Query.Filter != nil {
		if request.Query.Filter.Tree.Token.Value == "eq" {
			switch request.Query.Filter.Tree.Children[0].Token.Value {
			case "driveType":
				spaceType := spaces.SpaceType(strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'"))
				filters = filters.BySpaceType(spaceType)
			case "id":
				id := strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'")
				filters = filters.ByID(&provider.StorageSpaceId{OpaqueId: id})
			}
		} else {
			err := errors.Errorf("unsupported filter operand: %s", request.Query.Filter.Tree.Token.Value)
			return nil, err
		}
	}
	return filters.List(), nil
}

func (s *svc) cs3StorageSpaceToDrive(ctx context.Context, user *userpb.User, space *provider.StorageSpace) *libregraph.Drive {
	log := appctx.GetLogger(ctx)

	drive := &libregraph.Drive{
		DriveAlias: libregraph.PtrString(space.RootInfo.Path[1:]),
		Id:         libregraph.PtrString(space.Id.OpaqueId),
		Name:       space.Name,
		DriveType:  libregraph.PtrString(space.SpaceType),
		Special:    []libregraph.DriveItem{},
	}

	drive.Root = &libregraph.DriveItem{}

	if space.ReadmeId != "" || space.ThumbnailId != "" {
		storageId, _, ok := spaces.DecodeStorageSpaceID(space.Id.OpaqueId)

		gw, err := s.getClient()
		// If an error occurs, we just don't set the readme / thumbnail
		if err == nil && ok {
			if space.ReadmeId != "" {
				res, err := gw.Stat(ctx, &provider.StatRequest{
					Ref: &provider.Reference{
						ResourceId: &provider.ResourceId{
							StorageId: storageId,
							OpaqueId:  space.ReadmeId,
							SpaceId:   space.Id.OpaqueId,
						},
					},
				})
				if err == nil && res.Status.Code == rpcv1beta1.Code_CODE_OK {
					item := s.ResourceInfoToDriveItem(res.Info, "readme")
					drive.Special = append(drive.Special, item)
				} else {
					log.Error().Err(err).Str("spaceid", space.Id.OpaqueId).Any("status", res.Status).Msg("Failed to stat space README")
				}
			}
			if space.ThumbnailId != "" {
				res, err := gw.Stat(ctx, &provider.StatRequest{
					Ref: &provider.Reference{
						ResourceId: &provider.ResourceId{
							StorageId: storageId,
							OpaqueId:  space.ThumbnailId,
							SpaceId:   space.Id.OpaqueId,
						},
					},
				})
				if err == nil && res.Status.Code == rpcv1beta1.Code_CODE_OK {
					drive.Special = append(drive.Special, s.ResourceInfoToDriveItem(res.Info, "image"))
				} else {
					log.Error().Err(err).Str("spaceid", space.Id.OpaqueId).Any("status", res.Status).Msg("Failed to stat space thumbnail")
				}
			}
		} else {
			log.Error().Err(err).Any("spaceID", space.Id).Msg("Failed to get gateway client or failed to decode space ID")
		}
	}

	if space.Description != "" {
		drive.Description = libregraph.PtrString(space.Description)
	}

	if space.SpaceType != "personal" {
		drive.Root = &libregraph.DriveItem{
			Id:          libregraph.PtrString(space.Id.OpaqueId),
			Permissions: cs3PermissionsToLibreGraph(user, space.RootInfo.PermissionSet),
		}
	}

	drive.Root.WebDavUrl = libregraph.PtrString(fullURL(s.c.WebDavBase, space.RootInfo.Path))
	drive.WebUrl = libregraph.PtrString(fullURL(s.c.WebBase, space.RootInfo.Path))

	if space.Owner != nil && space.Owner.Id != nil {
		drive.Owner = &libregraph.IdentitySet{
			User: &libregraph.Identity{
				Id: &space.Owner.Id.OpaqueId,
			},
		}
	}

	if space.Quota != nil {
		drive.Quota = &libregraph.Quota{
			Total:     libregraph.PtrInt64(int64(space.Quota.QuotaMaxBytes)),
			Remaining: libregraph.PtrInt64(int64(space.Quota.RemainingBytes)),
			Used:      libregraph.PtrInt64(int64(space.Quota.QuotaMaxBytes - space.Quota.RemainingBytes)),
		}
	}

	if space.Mtime != nil {
		drive.LastModifiedDateTime = libregraph.PtrTime(time.Unix(int64(space.Mtime.Seconds), int64(space.Mtime.Nanos)))
	}
	return drive
}

func (s *svc) getSpace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	spaceID, _ := router.GetIdFromPath(r.URL.Path)
	if isShareJail(spaceID) {
		// For now we never go through this branch
		// (code will be only for sync clients, which do not yet go through Reva)
		shareRes, err := gw.GetReceivedShare(ctx, &collaborationv1beta1.GetReceivedShareRequest{
			Ref: &collaborationv1beta1.ShareReference{
				Spec: &collaborationv1beta1.ShareReference_Id{
					Id: &collaborationv1beta1.ShareId{
						OpaqueId: shareID(spaceID),
					},
				},
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("error getting received share")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		if shareRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(shareRes.Status.Code)).Str("message", shareRes.Status.Message).Msg("error getting received share")
			handleRpcStatus(ctx, shareRes.Status, w)
			return
		}

		stat, err := gw.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				ResourceId: shareRes.Share.Share.ResourceId,
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("error statting received share")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		if stat.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Interface("stat.Status", stat.Status).Msg("error statting received share")
			handleRpcStatus(ctx, shareRes.Status, w)
			return
		}

		space := s.convertShareToSpace(&gateway.ReceivedShareResourceInfo{
			ResourceInfo:  stat.Info,
			ReceivedShare: shareRes.Share,
		})
		_ = json.NewEncoder(w).Encode(space)
		return
	} else {
		listRes, err := gw.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
			Filters: []*provider.ListStorageSpacesRequest_Filter{
				{
					Type: provider.ListStorageSpacesRequest_Filter_TYPE_ID,
					Term: &provider.ListStorageSpacesRequest_Filter_Id{
						Id: &provider.StorageSpaceId{
							OpaqueId: spaceID,
						},
					},
				},
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("error getting space by id")
			handleError(ctx, err, http.StatusInternalServerError, w)
			return
		}
		if listRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(listRes.Status.Code)).Str("message", listRes.Status.Message).Msg("error getting space by id")
			handleRpcStatus(ctx, listRes.Status, w)
			return
		}

		spaces := listRes.StorageSpaces
		if len(spaces) == 1 {
			user := appctx.ContextMustGetUser(ctx)
			space := s.cs3StorageSpaceToDrive(ctx, user, spaces[0])
			_ = json.NewEncoder(w).Encode(space)
			return
		}
	}

	handleError(ctx, errors.New("space not found"), http.StatusNotFound, w)
}

func (s *svc) patchSpace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	update := &libregraph.DriveUpdate{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(update); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if update.Name == nil {
		handleError(ctx, errors.New("patching a space requires the space name"), http.StatusBadRequest, w)
		return
	}

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		handleError(ctx, err, http.StatusInternalServerError, w)
		return
	}

	spaceId := chi.URLParam(r, "space-id")
	updateRequest := &provider.UpdateStorageSpaceRequest{
		StorageSpace: &provider.StorageSpace{
			Id: &provider.StorageSpaceId{
				OpaqueId: spaceId,
			},
			Name: *update.Name,
		}}

	if len(update.Special) > 0 {
		updateData := update.Special[0]
		if updateData.Id == nil || updateData.SpecialFolder == nil {
			handleError(ctx, errors.New("Unsupported update type"), http.StatusBadRequest, w)
			return
		}

		_, _, id, ok := spaces.DecodeResourceID(*updateData.Id)
		if !ok {
			handleError(ctx, errors.New("ID not in an understandable format"), http.StatusBadRequest, w)
			return
		}
		switch *updateData.SpecialFolder.Name {
		case "readme":
			updateRequest.Field = &provider.UpdateStorageSpaceRequest_UpdateField{
				Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{
					Metadata: &provider.SpaceMetadata{
						Type: provider.SpaceMetadata_TYPE_README,
						Id:   id,
					},
				},
			}
		case "image":
			updateRequest.Field = &provider.UpdateStorageSpaceRequest_UpdateField{
				Field: &provider.UpdateStorageSpaceRequest_UpdateField_Metadata{
					Metadata: &provider.SpaceMetadata{
						Type: provider.SpaceMetadata_TYPE_THUMBNAIL,
						Id:   id,
					},
				},
			}
		default:
			handleError(ctx, errors.New("Unsupported update type"), http.StatusBadRequest, w)
			return
		}
	} else {
		handleError(ctx, errors.New("Unsupported update type"), http.StatusBadRequest, w)
		return
	}

	res, err := gw.UpdateStorageSpace(ctx, updateRequest)

	if err != nil {
		handleError(ctx, errors.New("failed to update storage space"), http.StatusInternalServerError, w)
		return
	}

	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("response", res).Msg("error updating public share")
		handleError(ctx, errors.New("failed to update storage space"), http.StatusInternalServerError, w)
		return
	}

	user := appctx.ContextMustGetUser(ctx)
	space := s.cs3StorageSpaceToDrive(ctx, user, res.StorageSpace)
	_ = json.NewEncoder(w).Encode(space)
}

func isShareJail(spaceID string) bool {
	return spaceID == ShareJailID
}

func shareID(spaceID string) string {
	// TODO
	return ""
}

func fullURL(base, path string) string {
	full, _ := url.JoinPath(base, path)
	return full
}

func cs3PermissionsToLibreGraph(user *userpb.User, perms *provider.ResourcePermissions) []libregraph.Permission {
	var p libregraph.Permission
	// we need to map the permissions to the roles
	switch {
	// having RemoveGrant qualifies you as a manager
	case perms.RemoveGrant:
		p.SetRoles([]string{*NewManagerUnifiedRole().Id})
	// InitiateFileUpload means you are an editor
	case perms.InitiateFileUpload:
		p.SetRoles([]string{*NewSpaceEditorUnifiedRole().Id})
	// Stat permission at least makes you a viewer
	case perms.Stat:
		p.SetRoles([]string{*NewSpaceViewerUnifiedRole().Id})
	}

	identity := &libregraph.Identity{
		DisplayName: user.DisplayName,
		Id:          &user.Id.OpaqueId,
	}

	p.GrantedToIdentities = []libregraph.IdentitySet{
		{
			User: identity,
		},
	}

	p.GrantedToV2 = &libregraph.SharePointIdentitySet{
		User: identity,
	}
	return []libregraph.Permission{p}
}

func (s *svc) ResourceInfoToDriveItem(r *provider.ResourceInfo, special string) libregraph.DriveItem {

	item := libregraph.DriveItem{
		Id:        libregraph.PtrString(spaces.EncodeResourceID(r.Id)),
		ETag:      libregraph.PtrString(r.Etag),
		Name:      libregraph.PtrString(r.Name),
		Size:      libregraph.PtrInt64(int64(r.Size)),
		WebDavUrl: libregraph.PtrString(fullURL(s.c.WebDavBase, r.Path)),
	}

	if len(strings.Split(r.Path, ".")) > 1 {
		mimetype := gomime.TypeByExtension(filepath.Ext(r.Path))
		if mimetype != "" {
			item.File = &libregraph.OpenGraphFile{
				MimeType: libregraph.PtrString(mimetype),
			}
		}
	}

	if special != "" {
		item.SpecialFolder = &libregraph.SpecialFolder{
			Name: libregraph.PtrString(special),
		}
	}

	return item
}
