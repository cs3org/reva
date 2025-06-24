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
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type UserSelectableProperty string

const (
	propUserId                           UserSelectableProperty = "id"
	propUserDisplayName                  UserSelectableProperty = "displayName"
	propUserDisplayNameDesc              UserSelectableProperty = "displayName desc"
	propUserMail                         UserSelectableProperty = "mail"
	propUserMailDesc                     UserSelectableProperty = "mail desc"
	propUserOnPremisesSamAccountName     UserSelectableProperty = "onPremisesSamAccountName"
	propUserOnPremisesSamAccountNameDesc UserSelectableProperty = "onPremisesSamAccountName desc"

	// Max number of users to return in a ListUsers query
	maxUserResponseLength int = 30
)

func (s UserSelectableProperty) Valid() bool {
	valid := []UserSelectableProperty{
		propUserId, propUserDisplayName, propUserDisplayNameDesc, propUserMail, propUserMailDesc,
		propUserOnPremisesSamAccountName, propUserOnPremisesSamAccountNameDesc,
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
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("must pass a search string of at least length 3 to list users")
	}
	queryVal := strings.Trim(req.Query.Search.RawValue, "\"")

	log.Debug().Str("Query", queryVal).Str("orderBy", req.Query.OrderBy.RawValue).Any("select", getUserSelectionFromRequest(req.Query.Select)).Msg("Listing users in libregraph API")

	users, err := gw.FindUsers(ctx, &userpb.FindUsersRequest{
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

	lgUsers := mapToLibregraphUsers(users.GetUsers(), getUserSelectionFromRequest(req.Query.Select), maxUserResponseLength)

	if req.Query.OrderBy.RawValue != "" {
		lgUsers, err = sortUsers(ctx, lgUsers, req.Query.OrderBy.RawValue)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}

	_ = json.NewEncoder(w).Encode(&ListResponse{
		Value: lgUsers,
	})

}

func (s *svc) getUserInfo(ctx context.Context, id *userpb.UserId) (*userpb.User, error) {
	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUser(ctx, &userpb.GetUserRequest{
		UserId: id,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	return res.User, nil
}

func (s *svc) getGroupInfo(ctx context.Context, id *grouppb.GroupId) (*grouppb.Group, error) {
	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
	if err != nil {
		return nil, err
	}
	res, err := gw.GetGroup(ctx, &grouppb.GetGroupRequest{
		GroupId: id,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	return res.Group, nil
}

// From a Select query, return a list of `SelectableProperty`s
func getUserSelectionFromRequest(selQuery *godata.GoDataSelectQuery) []UserSelectableProperty {
	if selQuery == nil {
		return nil
	}
	selection := []UserSelectableProperty{}
	items := strings.Split(selQuery.RawValue, ",")
	for _, item := range items {
		prop := UserSelectableProperty(item)
		if prop.Valid() {
			selection = append(selection, prop)
		}
	}
	return selection
}

// Map Reva users to LibreGraph users. If `selection` is nil, we map everything,
// otherwise we only map the properties set in `selection`
// If `max` > 0, we limit our response to `max` users
func mapToLibregraphUsers(users []*userpb.User, selection []UserSelectableProperty, max int) []libregraph.User {
	lgUsers := make([]libregraph.User, 0, len(users))

	for _, u := range users {
		if u == nil {
			continue
		}
		lgUser := libregraph.User{}
		if len(selection) == 0 {
			lgUser = libregraph.User{
				Id:                       &u.Id.OpaqueId,
				Mail:                     &u.Mail,
				OnPremisesSamAccountName: &u.Username,
				DisplayName:              &u.DisplayName,
			}
		} else {
			for _, prop := range selection {
				lgUser = appendPropToLgUser(u, lgUser, prop)
			}
		}
		lgUsers = append(lgUsers, lgUser)
		if max > 0 && len(lgUsers) > max {
			break
		}
	}

	return lgUsers
}

// Add a property `prop` from `u` to `lgUser`
func appendPropToLgUser(u *userpb.User, lgUser libregraph.User, prop UserSelectableProperty) libregraph.User {
	switch prop {
	case propUserId:
		lgUser.Id = &u.Id.OpaqueId
	case propUserDisplayName:
		lgUser.DisplayName = &u.DisplayName
	case propUserMail:
		lgUser.Mail = &u.Mail
	case propUserOnPremisesSamAccountName:
		lgUser.OnPremisesSamAccountName = &u.Username
	}
	return lgUser
}

// Sort users by the given sortKey. Valid sortkeys are `SelectableProperty`s
func sortUsers(ctx context.Context, users []libregraph.User, sortKey string) ([]libregraph.User, error) {
	log := appctx.GetLogger(ctx)
	log.Trace().Any("users", users).Str("sortKey", sortKey).Msg("func=sortUsers")
	if !UserSelectableProperty(sortKey).Valid() {
		return nil, errors.New("Not a valid orderBy argument: " + sortKey)
	}

	switch UserSelectableProperty(sortKey) {
	case propUserDisplayName:
		slices.SortFunc(users, func(a, b libregraph.User) int {
			return cmp.Compare(*a.DisplayName, *b.DisplayName)
		})
	case propUserId:
		slices.SortFunc(users, func(a, b libregraph.User) int {
			return cmp.Compare(*a.Id, *b.Id)
		})
	case propUserMail:
		slices.SortFunc(users, func(a, b libregraph.User) int {
			return cmp.Compare(*a.Mail, *b.Mail)
		})
	case propUserOnPremisesSamAccountName:
		slices.SortFunc(users, func(a, b libregraph.User) int {
			return cmp.Compare(*a.OnPremisesSamAccountName, *b.OnPremisesSamAccountName)
		})
	}
	return users, nil
}
