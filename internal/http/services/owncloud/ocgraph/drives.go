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
	"strings"

	"github.com/CiscoM31/godata"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/utils/list"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
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

	spaces := list.Map(res.StorageSpaces, func(space *providerpb.StorageSpace) *libregraph.Drive {
		return s.cs3StorageSpaceToDrive(me, space)
	})

	if err := json.NewEncoder(w).Encode(map[string]any{
		"value": spaces,
	}); err != nil {
		log.Error().Int("code", int(res.Status.Code)).Str("message", res.Status.Message).Msg("error listing storage spaces")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
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
		Id:         libregraph.PtrString(space.RootInfo.Path),
		Name:       space.Name,
		DriveType:  libregraph.PtrString(space.SpaceType),
		Root: &libregraph.DriveItem{
			Id:          libregraph.PtrString(space.RootInfo.Path),
			Permissions: cs3PermissionsToLibreGraph(user, space.RootInfo.PermissionSet),
		},
	}

	drive.Root.WebDavUrl = libregraph.PtrString(fullUrl(s.c.WebDavBase, space.RootInfo.Path))
	drive.WebUrl = libregraph.PtrString(fullUrl(s.c.WebBase, space.RootInfo.Path))

	if space.Owner != nil && space.Owner.Id != nil {
		drive.Owner = &libregraph.IdentitySet{
			User: &libregraph.Identity{
				Id: &space.Owner.Id.OpaqueId,
			},
		}
	}
	return drive
}

func fullUrl(base, path string) string {
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
	p.GrantedToIdentities = []libregraph.IdentitySet{
		{
			User: &libregraph.Identity{
				DisplayName: user.DisplayName,
				Id:          &user.Id.OpaqueId,
			},
		},
	}
	return []libregraph.Permission{p}
}
