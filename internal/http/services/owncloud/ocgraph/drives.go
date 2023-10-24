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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/spaces/

package ocgraph

import (
	"encoding/json"
	"net/http"
	"net/url"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	conversions "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/utils/list"
	libregraph "github.com/owncloud/libre-graph-api-go"
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

	res, err := gw.ListStorageSpaces(ctx, &providerpb.ListStorageSpacesRequest{
		Filters: nil, // TODO: add filters
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

	spaces := list.Map(res.StorageSpaces, s.cs3StorageSpaceToDrive)

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": spaces,
	}); err != nil {
		log.Error().Int("code", int(res.Status.Code)).Str("message", res.Status.Message).Msg("error listing storage spaces")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *svc) cs3StorageSpaceToDrive(space *providerpb.StorageSpace) *libregraph.Drive {
	drive := &libregraph.Drive{
		Id:        libregraph.PtrString(space.Id.OpaqueId),
		Name:      space.Name,
		DriveType: libregraph.PtrString(space.SpaceType),
		Root: &libregraph.DriveItem{
			Id:          libregraph.PtrString(space.Id.OpaqueId),
			Permissions: cs3PermissionsToLibreGraph(space.RootInfo.PermissionSet),
		},
	}

	drive.Root.WebDavUrl = libregraph.PtrString(fullUrl(s.webDavBaseURL, space.RootInfo.Path))
	drive.WebUrl = libregraph.PtrString(fullUrl(s.webBaseURL, space.RootInfo.Path))

	if space.Owner != nil && space.Owner.Id != nil {
		drive.Owner = &libregraph.IdentitySet{
			User: &libregraph.Identity{
				Id: &space.Owner.Id.OpaqueId,
			},
		}
	}
	return drive
}

func fullUrl(base *url.URL, path string) string {
	full, _ := url.JoinPath(base.Path, path)
	return full
}

func cs3PermissionsToLibreGraph(perms *providerpb.ResourcePermissions) []libregraph.Permission {
	role := conversions.RoleFromResourcePermissions(perms)
	var p libregraph.Permission
	p.SetRoles([]string{role.Name})
	return []libregraph.Permission{p}
}
