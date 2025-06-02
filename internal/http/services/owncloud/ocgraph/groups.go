// Copyright 2018-2025 CERN
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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/CiscoM31/godata"
	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type GroupSelectableProperty string

const (
	propGroupId          GroupSelectableProperty = "id"
	propGroupDisplayName GroupSelectableProperty = "displayName"
	propGroupMail        GroupSelectableProperty = "mail"
	propGroupDescription GroupSelectableProperty = "description"
	propGroupMembers     GroupSelectableProperty = "members"
)

func (s GroupSelectableProperty) Valid() bool {
	valid := []GroupSelectableProperty{
		propGroupId, propGroupDisplayName, propGroupMail, propGroupDescription, propGroupMembers,
	}
	return slices.Contains(valid, s)
}

func (s *svc) listGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req, err := godata.ParseRequest(ctx, r.URL.Path, r.URL.Query())
	if err != nil {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get groups: query error")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Query.Search == nil || req.Query.Search.RawValue == "" || len(req.Query.Search.RawValue) < 3 {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("must pass a search string of at least length 3 to list groups")
	}
	queryVal := strings.Trim(req.Query.Search.RawValue, "\"")

	log.Debug().Str("Query", queryVal).Str("orderBy", req.Query.OrderBy.RawValue).Any("select", getGroupSelectionFromRequest(req.Query.Select)).Msg("Listing groups in libregraph API")

	groups, err := gw.FindGroups(ctx, &groupv1beta1.FindGroupsRequest{
		SkipFetchingMembers: true,
		Filter:              queryVal,
	})

	if err != nil {
		handleError(err, w)
		return
	}
	if groups.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, groups.Status, w)
		return
	}

	lgGroups := mapToLibregraphGroups(groups.GetGroups(), getGroupSelectionFromRequest(req.Query.Select))

	if req.Query.OrderBy.RawValue != "" {
		lgGroups, err = sortGroups(ctx, lgGroups, req.Query.OrderBy.RawValue)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}

	_ = json.NewEncoder(w).Encode(&ListResponse{
		Value: lgGroups,
	})

}

// From a Select query, return a list of `SelectableProperty`s
func getGroupSelectionFromRequest(selQuery *godata.GoDataSelectQuery) []GroupSelectableProperty {
	if selQuery == nil {
		return nil
	}
	selection := []GroupSelectableProperty{}
	items := strings.Split(selQuery.RawValue, ",")
	for _, item := range items {
		prop := GroupSelectableProperty(item)
		if prop.Valid() {
			selection = append(selection, prop)
		}
	}
	return selection
}

// Map Reva users to LibreGraph users. If `selection` is nil, we map everything,
// otherwise we only map the properties set in `selection`
func mapToLibregraphGroups(groups []*groupv1beta1.Group, selection []GroupSelectableProperty) []libregraph.Group {
	lgGroups := make([]libregraph.Group, 0, len(groups))

	for _, g := range groups {
		if g == nil {
			continue
		}
		lgGroup := libregraph.Group{}
		if len(selection) == 0 {
			lgGroup = libregraph.Group{
				Id:          &g.Id.OpaqueId,
				DisplayName: &g.DisplayName,
				Members:     mapToLibregraphUsersById(g.Members),
			}
		} else {
			for _, prop := range selection {
				lgGroup = appendPropToLgGroup(g, lgGroup, prop)
			}
		}
		lgGroups = append(lgGroups, lgGroup)
	}

	return lgGroups
}

// Add a property `prop` from `u` to `lgUser`
func appendPropToLgGroup(u *groupv1beta1.Group, lgGroup libregraph.Group, prop GroupSelectableProperty) libregraph.Group {
	switch prop {
	case propGroupId:
		lgGroup.Id = &u.Id.OpaqueId
	case propGroupDisplayName:
		lgGroup.DisplayName = &u.DisplayName
	case propGroupMembers:
		lgGroup.Members = mapToLibregraphUsersById(u.Members)
	}
	return lgGroup
}

// Sort groups by displayName
func sortGroups(ctx context.Context, groups []libregraph.Group, sortKey string) ([]libregraph.Group, error) {
	log := appctx.GetLogger(ctx)
	log.Trace().Any("groups", groups).Str("sortKey", "displayName").Msg("func=sortUsers")
	if sortKey != "displayName" {
		return nil, errors.New("Invalid sortKey: supported values are: displayName")
	}

	slices.SortFunc(groups, func(a, b libregraph.Group) int {
		return cmp.Compare(*a.DisplayName, *b.DisplayName)
	})

	return groups, nil
}

func mapToLibregraphUsersById(ids []*userv1beta1.UserId) []libregraph.User {
	lgUsers := make([]libregraph.User, 0, len(ids))

	for _, id := range ids {
		lgUser := libregraph.User{
			Id: &id.OpaqueId,
		}
		lgUsers = append(lgUsers, lgUser)
	}

	return lgUsers
}
