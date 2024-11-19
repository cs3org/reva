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
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/utils/list"
	"github.com/go-chi/chi/v5"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

func (s *svc) listMySpaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	odataReq, err := godata.ParseRequest(r.Context(), r.URL.Path, r.URL.Query())
	if err != nil {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get drives: query error")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var spaces []*libregraph.Drive
	if isMountpointRequest(odataReq) {
		spaces, err = getDrivesForShares(ctx, gw)
		if err != nil {
			log.Error().Err(err).Msg("error getting share spaces")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		filters, err := generateCs3Filters(odataReq)
		if err != nil {
			log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get drives: error parsing filters")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		res, err := gw.ListStorageSpaces(ctx, &providerpb.ListStorageSpacesRequest{
			Filters: filters,
		})
		if err != nil {
			log.Error().Err(err).Msg("error listing storage spaces")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if res.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(res.Status.Code)).Str("message", res.Status.Message).Msg("error listing storage spaces")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		me := appctx.ContextMustGetUser(ctx)
		spaces = list.Map(res.StorageSpaces, func(space *providerpb.StorageSpace) *libregraph.Drive {
			return s.cs3StorageSpaceToDrive(me, space)
		})
	}

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": spaces,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling spaces as json")
		w.WriteHeader(http.StatusInternalServerError)
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

const shareJailID = "a0ca6a90-a365-4782-871e-d44447bbc668"

func getDrivesForShares(ctx context.Context, gw gateway.GatewayAPIClient) ([]*libregraph.Drive, error) {
	res, err := gw.ListExistingReceivedShares(ctx, &collaborationv1beta1.ListReceivedSharesRequest{})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	spacesRes := make([]*libregraph.Drive, 0, len(res.ShareInfos))
	for _, s := range res.ShareInfos {
		spacesRes = append(spacesRes, convertShareToSpace(s))
	}
	return spacesRes, nil
}

func libregraphShareID(shareID *collaborationv1beta1.ShareId) string {
	return fmt.Sprintf("%s$%s!%s", shareJailID, shareJailID, shareID.OpaqueId)
}

func convertShareToSpace(rsi *gateway.ReceivedShareResourceInfo) *libregraph.Drive {
	// the prefix of the remote_item.id and rootid
	return &libregraph.Drive{
		Id:         libregraph.PtrString(libregraphShareID(rsi.ReceivedShare.Share.Id)),
		DriveType:  libregraph.PtrString("mountpoint"),
		DriveAlias: libregraph.PtrString(rsi.ReceivedShare.Share.Id.OpaqueId), // this is not used, but must not be the same alias as the drive item
		Name:       filepath.Base(rsi.ResourceInfo.Path),
		Quota: &libregraph.Quota{
			Total:     libregraph.PtrInt64(24154390300000),
			Used:      libregraph.PtrInt64(3141592),
			Remaining: libregraph.PtrInt64(24154387158408),
		},
		Root: &libregraph.DriveItem{
			Id: libregraph.PtrString(fmt.Sprintf("%s$%s!%s", shareJailID, shareJailID, rsi.ReceivedShare.Share.Id.OpaqueId)),
			RemoteItem: &libregraph.RemoteItem{
				DriveAlias: libregraph.PtrString(strings.TrimSuffix(strings.TrimPrefix(rsi.ResourceInfo.Path, "/"), relativePathToSpaceID(rsi.ResourceInfo))), // the drive alias must not start with /
				ETag:       libregraph.PtrString(rsi.ResourceInfo.Etag),
				Folder:     &libregraph.Folder{},
				// The Id must correspond to the id in the OCS response, for the time being
				// It is in the form <something>!<something-else>
				Id:                   libregraph.PtrString(spaces.EncodeResourceID(rsi.ResourceInfo.Id)),
				LastModifiedDateTime: libregraph.PtrTime(time.Unix(int64(rsi.ResourceInfo.Mtime.Seconds), int64(rsi.ResourceInfo.Mtime.Nanos))),
				Name:                 libregraph.PtrString(filepath.Base(rsi.ResourceInfo.Path)),
				Path:                 libregraph.PtrString(relativePathToSpaceID(rsi.ResourceInfo)),
				// RootId must have the same token before ! as Id
				// the second part for the time being is not used
				RootId: libregraph.PtrString(fmt.Sprintf("%s!unused_root_id", spaces.EncodeSpaceID(rsi.ResourceInfo.Id.StorageId, rsi.ResourceInfo.Id.SpaceId))),
				Size:   libregraph.PtrInt64(int64(rsi.ResourceInfo.Size)),
			},
		},
	}
}

func relativePathToSpaceID(info *providerpb.ResourceInfo) string {
	return strings.TrimPrefix(info.Path, info.Id.SpaceId)
}

func generateCs3Filters(request *godata.GoDataRequest) ([]*providerpb.ListStorageSpacesRequest_Filter, error) {
	var filters spaces.ListStorageSpaceFilter
	if request.Query.Filter != nil {
		if request.Query.Filter.Tree.Token.Value == "eq" {
			switch request.Query.Filter.Tree.Children[0].Token.Value {
			case "driveType":
				spaceType := spaces.SpaceType(strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'"))
				filters = filters.BySpaceType(spaceType)
			case "id":
				id := strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'")
				filters = filters.ByID(&providerpb.StorageSpaceId{OpaqueId: id})
			}
		} else {
			err := errors.Errorf("unsupported filter operand: %s", request.Query.Filter.Tree.Token.Value)
			return nil, err
		}
	}
	return filters.List(), nil
}

func (s *svc) cs3StorageSpaceToDrive(user *userpb.User, space *providerpb.StorageSpace) *libregraph.Drive {
	drive := &libregraph.Drive{
		DriveAlias: libregraph.PtrString(space.RootInfo.Path[1:]),
		Id:         libregraph.PtrString(space.Id.OpaqueId),
		Name:       space.Name,
		DriveType:  libregraph.PtrString(space.SpaceType),
	}

	drive.Root = &libregraph.DriveItem{}

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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	spaceID, _ := router.ShiftPath(r.URL.Path)
	if isShareJail(spaceID) {
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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if shareRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(shareRes.Status.Code)).Str("message", shareRes.Status.Message).Msg("error getting received share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		stat, err := gw.Stat(ctx, &providerpb.StatRequest{
			Ref: &providerpb.Reference{
				ResourceId: shareRes.Share.Share.ResourceId,
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("error statting received share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if stat.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Interface("stat.Status", stat.Status).Msg("error statting received share")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		space := convertShareToSpace(&gateway.ReceivedShareResourceInfo{
			ResourceInfo:  stat.Info,
			ReceivedShare: shareRes.Share,
		})
		_ = json.NewEncoder(w).Encode(space)
		return
	} else {
		listRes, err := gw.ListStorageSpaces(ctx, &providerpb.ListStorageSpacesRequest{
			Filters: []*providerpb.ListStorageSpacesRequest_Filter{
				{
					Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
					Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
						Id: &providerpb.StorageSpaceId{
							OpaqueId: spaceID,
						},
					},
				},
			},
		})
		if err != nil {
			log.Error().Err(err).Msg("error getting space by id")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if listRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Int("code", int(listRes.Status.Code)).Str("message", listRes.Status.Message).Msg("error getting space by id")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		spaces := listRes.StorageSpaces
		if len(spaces) == 1 {
			user := appctx.ContextMustGetUser(ctx)
			space := s.cs3StorageSpaceToDrive(user, spaces[0])
			_ = json.NewEncoder(w).Encode(space)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func isShareJail(spaceID string) bool {
	return false // TODO
}

func shareID(spaceID string) string {
	return "" // TODO
}

func fullURL(base, path string) string {
	full, _ := url.JoinPath(base, path)
	return full
}

func cs3PermissionsToLibreGraph(user *userpb.User, perms *providerpb.ResourcePermissions) []libregraph.Permission {
	var p libregraph.Permission
	// we need to map the permissions to the roles
	switch {
	// having RemoveGrant qualifies you as a manager
	case perms.RemoveGrant:
		p.SetRoles([]string{"manager"})
	// InitiateFileUpload means you are an editor
	case perms.InitiateFileUpload:
		p.SetRoles([]string{"editor"})
	// Stat permission at least makes you a viewer
	case perms.Stat:
		p.SetRoles([]string{"viewer"})
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

func (s *svc) getDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	resourceID := chi.URLParam(r, "resource-id")
	resourceID, _ = url.QueryUnescape(resourceID)
	storageID, _, itemID, ok := spaces.DecodeResourceID(resourceID)
	if !ok {
		log.Error().Str("resource-id", resourceID).Msg("resource id cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.getPermissionsByCs3Reference(ctx, w, log, &providerpb.Reference{
		ResourceId: &providerpb.ResourceId{
			StorageId: storageID,
			OpaqueId:  itemID,
		},
	})
}

func (s *svc) getRootDrivePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	spaceID := chi.URLParam(r, "space-id")
	spaceID, _ = url.QueryUnescape(spaceID)
	_, path, ok := spaces.DecodeSpaceID(spaceID)
	if !ok {
		log.Error().Str("space-id", spaceID).Msg("space id cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.getPermissionsByCs3Reference(ctx, w, log, &providerpb.Reference{Path: path})
}

func (s *svc) getPermissionsByCs3Reference(ctx context.Context, w http.ResponseWriter, log *zerolog.Logger, ref *providerpb.Reference) {
	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statRes, err := gw.Stat(ctx, &providerpb.StatRequest{
		Ref: ref,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting space by id")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		log.Error().Interface("ref", ref).Int("code", int(statRes.Status.Code)).Str("message", statRes.Status.Message).Msg("error statting resource")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	actions := CS3ResourcePermissionsToLibregraphActions(statRes.Info.PermissionSet)
	roles := GetApplicableRoleDefinitionsForActions(actions)

	if err := json.NewEncoder(w).Encode(map[string]any{
		"@libre.graph.permissions.actions.allowedValues": actions,
		"@libre.graph.permissions.roles.allowedValues":   roles,
	}); err != nil {
		log.Error().Err(err).Msg("error marshalling spaces as json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
