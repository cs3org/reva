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
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"github.com/pkg/errors"
)

type UserSelectableProperty string

const (
	languageKey   = "lang"
	preferencesNS = "core"
)

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
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	user := appctx.ContextMustGetUser(r.Context())
	me := &libregraph.User{
		DisplayName:              user.DisplayName,
		Mail:                     &user.Mail,
		OnPremisesSamAccountName: user.Username,
		Id:                       &user.Id.OpaqueId,
	}

	gw, err := s.getClient()
	if err == nil {
		lang, err := gw.GetKey(r.Context(), &preferences.GetKeyRequest{
			Key: &preferences.PreferenceKey{
				Key:       languageKey,
				Namespace: preferencesNS,
			},
		})
		if err == nil && lang.Status != nil && lang.Status.Code == rpc.Code_CODE_OK {
			me.PreferredLanguage = libregraph.PtrString(lang.Val)
		} else {
			if lang != nil {
				log.Debug().Err(err).Any("Status", lang.Status).Msg("Failed to fetch language for user")
			} else {
				log.Debug().Err(err).Msg("Failed to fetch language for user")
			}
		}
	}
	_ = json.NewEncoder(w).Encode(me)
}

func (s *svc) patchMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	update := &libregraph.UserUpdate{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(update); err != nil {
		log.Error().Err(err).Interface("Body", r.Body).Msg("failed unmarshalling request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if update.PreferredLanguage == nil || *update.PreferredLanguage == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Must set preferredLanguage"))

		return
	}

	user := appctx.ContextMustGetUser(r.Context())
	me := &libregraph.User{
		DisplayName:              user.DisplayName,
		Mail:                     &user.Mail,
		OnPremisesSamAccountName: user.Username,
		Id:                       &user.Id.OpaqueId,
		PreferredLanguage:        update.PreferredLanguage,
	}

	gw, err := s.getClient()
	if err != nil {
		handleError(ctx, err, w)
		return
	}

	res, err := gw.SetKey(ctx, &preferences.SetKeyRequest{
		Key: &preferences.PreferenceKey{
			Key:       languageKey,
			Namespace: preferencesNS,
		},
		Val: *update.PreferredLanguage,
	})

	if err != nil {
		handleError(ctx, err, w)
		return
	}
	if res != nil && res.Status != nil && res.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, res.Status, "Failed to set preference key in gateway", w)
		return
	}

	_ = json.NewEncoder(w).Encode(me)
}

func (s *svc) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gw, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		handleError(ctx, err, w)
		return
	}

	req, err := godata.ParseRequest(ctx, r.URL.Path, r.URL.Query())
	if err != nil {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("could not get users: query error")
		handleBadRequest(ctx, err, w)
		return
	}

	if req.Query.Search == nil || req.Query.Search.RawValue == "" || len(req.Query.Search.RawValue) < 3 {
		log.Debug().Err(err).Interface("query", r.URL.Query()).Msg("must pass a search string of at least length 3 to list users")
	}
	queryVal := strings.Trim(req.Query.Search.RawValue, "\"")
	log.Debug().Str("Query", queryVal).Str("orderBy", req.Query.OrderBy.RawValue).Any("select", getUserSelectionFromRequest(req.Query.Select)).Msg("Listing users in libregraph API")

	filters, err := generateUserFilters(req)
	if err != nil {
		handleBadRequest(ctx, err, w)
		return
	}
	// If no filter on type is specified, we default to
	// searching only primary accounts
	if len(filters) == 0 {
		filters = append(filters, &userpb.Filter{
			Type: userpb.Filter_TYPE_USERTYPE,
			Term: &userpb.Filter_Usertype{
				Usertype: userpb.UserType_USER_TYPE_PRIMARY,
			},
		})
	}
	filters = append(filters, &userpb.Filter{
		Type: userpb.Filter_TYPE_QUERY,
		Term: &userpb.Filter_Query{
			Query: queryVal,
		},
	})
	request := &userpb.FindUsersRequest{
		SkipFetchingUserGroups: true,
		Filters:                filters,
	}

	users, err := gw.FindUsers(ctx, request)
	if err != nil {
		handleError(ctx, err, w)
		return
	}
	if users.Status.Code != rpc.Code_CODE_OK {
		handleRpcStatus(ctx, users.Status, "ocgraph FindUsers request failed", w)
		return
	}

	lgUsers := mapToLibregraphUsers(users.GetUsers(), getUserSelectionFromRequest(req.Query.Select), maxUserResponseLength)

	if req.Query.OrderBy.RawValue != "" {
		lgUsers, err = sortUsers(ctx, lgUsers, req.Query.OrderBy.RawValue)
		if err != nil {
			handleBadRequest(ctx, err, w)
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
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	return res.User, nil
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
		var id string = u.Id.OpaqueId

		// For federated users, form their OCM address by appending @IdP to the OpaqueId
		if u.Id.Type == userpb.UserType_USER_TYPE_FEDERATED {
			id = id + "@" + u.Id.Idp
		}
		if len(selection) == 0 {
			lgUser = libregraph.User{
				Id:                       &id,
				Mail:                     &u.Mail,
				OnPremisesSamAccountName: u.Username,
				DisplayName:              u.DisplayName,
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
		lgUser.DisplayName = u.DisplayName
	case propUserMail:
		lgUser.Mail = &u.Mail
	case propUserOnPremisesSamAccountName:
		lgUser.OnPremisesSamAccountName = u.Username
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
			return cmp.Compare(a.DisplayName, b.DisplayName)
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
			return cmp.Compare(a.OnPremisesSamAccountName, b.OnPremisesSamAccountName)
		})
	}
	return users, nil
}

func generateUserFilters(request *godata.GoDataRequest) ([]*userpb.Filter, error) {
	var filters []*userpb.Filter
	if request.Query.Filter != nil {
		if request.Query.Filter.Tree.Token.Value == "eq" {
			switch strings.ToLower(request.Query.Filter.Tree.Children[0].Token.Value) {
			case "usertype":
				ut := strings.Trim(request.Query.Filter.Tree.Children[1].Token.Value, "'")
				userType := UserType(ut)
				if userType == nil {
					return nil, errors.Errorf("unknown usertype: %s", ut)
				}

				filter := &userpb.Filter{
					Type: userpb.Filter_TYPE_USERTYPE,
					Term: &userpb.Filter_Usertype{
						Usertype: *userType,
					},
				}
				filters = append(filters, filter)

			}
		} else {
			err := errors.Errorf("unsupported filter operand: %s", request.Query.Filter.Tree.Token.Value)
			return nil, err
		}
	}
	return filters, nil
}

func UserType(userType string) *userpb.UserType {
	var ut userpb.UserType
	switch strings.ToLower(userType) {
	case "invalid":
		ut = userpb.UserType_USER_TYPE_INVALID
	case "primary":
		ut = userpb.UserType_USER_TYPE_PRIMARY
	case "secondary":
		ut = userpb.UserType_USER_TYPE_SECONDARY
	case "service":
		ut = userpb.UserType_USER_TYPE_SERVICE
	case "application":
		ut = userpb.UserType_USER_TYPE_APPLICATION
	case "guest":
		ut = userpb.UserType_USER_TYPE_GUEST
	case "federated":
		ut = userpb.UserType_USER_TYPE_FEDERATED
	case "lightweight":
		ut = userpb.UserType_USER_TYPE_LIGHTWEIGHT
	case "space_owner":
		ut = userpb.UserType_USER_TYPE_SPACE_OWNER

	default:
		return nil

	}
	return &ut
}
