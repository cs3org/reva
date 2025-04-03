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
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type SelectableProperty string

const (
	propId                           SelectableProperty = "id"
	propDisplayName                  SelectableProperty = "displayName"
	propDisplayNameDesc              SelectableProperty = "displayName desc"
	propMail                         SelectableProperty = "mail"
	propMailDesc                     SelectableProperty = "mail desc"
	propOnPremisesSamAccountName     SelectableProperty = "onPremisesSamAccountName"
	propOnPremisesSamAccountNameDesc SelectableProperty = "onPremisesSamAccountName desc"
)

func (s SelectableProperty) Valid() bool {
	valid := []SelectableProperty{
		propId, propDisplayName, propDisplayNameDesc, propMail, propMailDesc,
		propOnPremisesSamAccountName, propOnPremisesSamAccountNameDesc,
	}
	return slices.Contains(valid, s)
}

// https://owncloud.dev/apis/http/graph/users/#reading-users
func (s *svc) getMe(w http.ResponseWriter, r *http.Request) {
	user := appctx.ContextMustGetUser(r.Context())
	me := &libregraph.User{
		DisplayName:              &user.DisplayName,
		Mail:                     &user.Mail,
		OnPremisesSamAccountName: &user.Username,
		Id:                       &user.Id.OpaqueId,
	}
	_ = json.NewEncoder(w).Encode(me)
}

func (s *svc) listUsers(w http.ResponseWriter, r *http.Request) {
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
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get users: query error")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Query.Search == nil || req.Query.Search.RawValue == "" || len(req.Query.Search.RawValue) < 3 {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("must pass a search string of at least length 3 to list uses")
	}
	queryVal := strings.Trim(req.Query.Search.RawValue, "\"")

	log.Debug().Str("Query", queryVal).Str("orderBy", req.Query.OrderBy.RawValue).Any("select", getSelectionFromRequest(req.Query.Select)).Msg("Listing users in libregraph API")

	users, err := gw.FindUsers(ctx, &userv1beta1.FindUsersRequest{
		SkipFetchingUserGroups: true,
		Filter:                 queryVal,
	})

	if err != nil {
		handleError(err, w)
		return
	}
	if users.Status.Code != rpcv1beta1.Code_CODE_OK {
		handleRpcStatus(ctx, users.Status, w)
		return
	}

	lgUsers := mapToLibregraphUsers(users.GetUsers(), getSelectionFromRequest(req.Query.Select))

	if req.Query.OrderBy.RawValue != "" {
		lgUsers, err = sortUsers(ctx, lgUsers, req.Query.OrderBy.RawValue)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}

	_ = json.NewEncoder(w).Encode(lgUsers)

}

// From a Select query, return a list of `SelectableProperty`s
func getSelectionFromRequest(selQuery *godata.GoDataSelectQuery) []SelectableProperty {
	if selQuery == nil {
		return nil
	}
	selection := []SelectableProperty{}
	items := strings.Split(selQuery.RawValue, ",")
	for _, item := range items {
		prop := SelectableProperty(item)
		if prop.Valid() {
			selection = append(selection, prop)
		}
	}
	return selection
}

// Map Reva users to LibreGraph users. If `selection` is nil, we map everything,
// otherwise we only map the properties set in `selection`
func mapToLibregraphUsers(users []*userv1beta1.User, selection []SelectableProperty) []*libregraph.User {
	lgUsers := make([]*libregraph.User, 0, len(users))

	for _, u := range users {
		if u == nil {
			continue
		}
		lgUser := &libregraph.User{}
		if len(selection) == 0 {
			lgUser = &libregraph.User{
				Id:                       &u.Id.OpaqueId,
				Mail:                     &u.Mail,
				OnPremisesSamAccountName: &u.Username,
				DisplayName:              &u.DisplayName,
			}
		} else {
			for _, prop := range selection {
				appendPropToLgUser(u, lgUser, prop)
			}
		}
		lgUsers = append(lgUsers, lgUser)
	}

	return lgUsers
}

// Add a property `prop` from `u` to `lgUser`
func appendPropToLgUser(u *userv1beta1.User, lgUser *libregraph.User, prop SelectableProperty) {
	switch prop {
	case propId:
		lgUser.Id = &u.Id.OpaqueId
	case propDisplayName:
		lgUser.DisplayName = &u.DisplayName
	case propMail:
		lgUser.Mail = &u.Mail
	case propOnPremisesSamAccountName:
		lgUser.OnPremisesSamAccountName = &u.Username
	}
}

// Sort users by the given sortKey. Valid sortkeys are `SelectableProperty`s
func sortUsers(ctx context.Context, users []*libregraph.User, sortKey string) ([]*libregraph.User, error) {
	log := appctx.GetLogger(ctx)
	log.Trace().Any("users", users).Str("sortKey", sortKey).Msg("func=sortUsers")
	if !SelectableProperty(sortKey).Valid() {
		return nil, errors.New("Not a valid orderBy argument: " + sortKey)
	}

	switch SelectableProperty(sortKey) {
	case propDisplayName:
		slices.SortFunc(users, func(a, b *libregraph.User) int {
			if a == nil || b == nil {
				return 0
			}
			return cmp.Compare(*a.DisplayName, *b.DisplayName)
		})
	case propId:
		slices.SortFunc(users, func(a, b *libregraph.User) int {
			if a == nil || b == nil {
				return 0
			}
			return cmp.Compare(*a.Id, *b.Id)
		})
	case propMail:
		slices.SortFunc(users, func(a, b *libregraph.User) int {
			if a == nil || b == nil {
				return 0
			}
			return cmp.Compare(*a.Mail, *b.Mail)
		})
	case propOnPremisesSamAccountName:
		slices.SortFunc(users, func(a, b *libregraph.User) int {
			if a == nil || b == nil {
				return 0
			}
			return cmp.Compare(*a.OnPremisesSamAccountName, *b.OnPremisesSamAccountName)
		})
	}
	return users, nil
}
