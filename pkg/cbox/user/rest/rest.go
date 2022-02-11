// Copyright 2018-2021 CERN
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

package rest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	utils "github.com/cs3org/reva/pkg/cbox/utils"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("rest", New)
}

var (
	emailRegex    = regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	usernameRegex = regexp.MustCompile(`^[ a-zA-Z0-9._-]+$`)
)

type manager struct {
	conf            *config
	redisPool       *redis.Pool
	apiTokenManager *utils.APITokenManager
}

type config struct {
	// The address at which the redis server is running
	RedisAddress string `mapstructure:"redis_address" docs:"localhost:6379"`
	// The username for connecting to the redis server
	RedisUsername string `mapstructure:"redis_username" docs:""`
	// The password for connecting to the redis server
	RedisPassword string `mapstructure:"redis_password" docs:""`
	// The time in minutes for which the groups to which a user belongs would be cached
	UserGroupsCacheExpiration int `mapstructure:"user_groups_cache_expiration" docs:"5"`
	// The OIDC Provider
	IDProvider string `mapstructure:"id_provider" docs:"http://cernbox.cern.ch"`
	// Base API Endpoint
	APIBaseURL string `mapstructure:"api_base_url" docs:"https://authorization-service-api-dev.web.cern.ch/api/v1.0"`
	// Client ID needed to authenticate
	ClientID string `mapstructure:"client_id" docs:"-"`
	// Client Secret
	ClientSecret string `mapstructure:"client_secret" docs:"-"`

	// Endpoint to generate token to access the API
	OIDCTokenEndpoint string `mapstructure:"oidc_token_endpoint" docs:"https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"`
	// The target application for which token needs to be generated
	TargetAPI string `mapstructure:"target_api" docs:"authorization-service-api"`
}

func (c *config) init() {
	if c.UserGroupsCacheExpiration == 0 {
		c.UserGroupsCacheExpiration = 5
	}
	if c.RedisAddress == "" {
		c.RedisAddress = ":6379"
	}
	if c.APIBaseURL == "" {
		c.APIBaseURL = "https://authorization-service-api-dev.web.cern.ch/api/v1.0"
	}
	if c.TargetAPI == "" {
		c.TargetAPI = "authorization-service-api"
	}
	if c.OIDCTokenEndpoint == "" {
		c.OIDCTokenEndpoint = "https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"
	}
	if c.IDProvider == "" {
		c.IDProvider = "http://cernbox.cern.ch"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns a user manager implementation that makes calls to the GRAPPA API.
func New(m map[string]interface{}) (user.Manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		return nil, err
	}
	return mgr, err
}

func (m *manager) Configure(ml map[string]interface{}) error {
	c, err := parseConfig(ml)
	if err != nil {
		return err
	}
	c.init()
	redisPool := initRedisPool(c.RedisAddress, c.RedisUsername, c.RedisPassword)
	apiTokenManager := utils.InitAPITokenManager(c.TargetAPI, c.OIDCTokenEndpoint, c.ClientID, c.ClientSecret)
	m.conf = c
	m.redisPool = redisPool
	m.apiTokenManager = apiTokenManager
	return nil
}

func (m *manager) getUser(ctx context.Context, url string) (map[string]interface{}, error) {
	responseData, err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false)
	if err != nil {
		return nil, err
	}

	var users []map[string]interface{}
	for _, usr := range responseData {
		userData, ok := usr.(map[string]interface{})
		if !ok {
			continue
		}

		t, _ := userData["type"].(string)
		userType := getUserType(t, userData["upn"].(string))
		if userType != userpb.UserType_USER_TYPE_APPLICATION && userType != userpb.UserType_USER_TYPE_FEDERATED {
			users = append(users, userData)
		}
	}

	if len(users) != 1 {
		return nil, errors.New("rest: user not found for URL: " + url)
	}

	return users[0], nil
}

func (m *manager) getUserByParam(ctx context.Context, param, val string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/Identity?filter=%s:%s&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type",
		m.conf.APIBaseURL, param, url.QueryEscape(val))
	return m.getUser(ctx, url)
}

func (m *manager) getLightweightUser(ctx context.Context, mail string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/Identity?filter=primaryAccountEmail:%s&filter=upn:contains:guest&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type",
		m.conf.APIBaseURL, url.QueryEscape(mail))
	return m.getUser(ctx, url)
}

func (m *manager) getInternalUserID(ctx context.Context, uid *userpb.UserId) (string, error) {

	internalID, err := m.fetchCachedInternalID(uid)
	if err != nil {
		var (
			userData map[string]interface{}
			err      error
		)
		if uid.Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT {
			// Lightweight accounts need to be fetched by email
			userData, err = m.getLightweightUser(ctx, strings.TrimPrefix(uid.OpaqueId, "guest:"))
		} else {
			userData, err = m.getUserByParam(ctx, "upn", uid.OpaqueId)
		}
		if err != nil {
			return "", err
		}
		id, ok := userData["id"].(string)
		if !ok {
			return "", errors.New("rest: error in type assertion")
		}

		if err = m.cacheInternalID(uid, id); err != nil {
			log := appctx.GetLogger(ctx)
			log.Error().Err(err).Msg("rest: error caching user details")
		}
		return id, nil
	}
	return internalID, nil
}

func (m *manager) parseAndCacheUser(ctx context.Context, userData map[string]interface{}) *userpb.User {
	upn, _ := userData["upn"].(string)
	mail, _ := userData["primaryAccountEmail"].(string)
	name, _ := userData["displayName"].(string)
	uidNumber, _ := userData["uid"].(float64)
	gidNumber, _ := userData["gid"].(float64)
	t, _ := userData["type"].(string)
	userType := getUserType(t, upn)

	userID := &userpb.UserId{
		OpaqueId: upn,
		Idp:      m.conf.IDProvider,
		Type:     userType,
	}
	u := &userpb.User{
		Id:          userID,
		Username:    upn,
		Mail:        mail,
		DisplayName: name,
		UidNumber:   int64(uidNumber),
		GidNumber:   int64(gidNumber),
	}

	if err := m.cacheUserDetails(u); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching user details")
	}
	if err := m.cacheInternalID(userID, userData["id"].(string)); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching user details")
	}
	return u

}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId, skipFetchingGroups bool) (*userpb.User, error) {
	u, err := m.fetchCachedUserDetails(uid)
	if err != nil {
		var (
			userData map[string]interface{}
			err      error
		)
		if uid.Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT {
			// Lightweight accounts need to be fetched by email
			userData, err = m.getLightweightUser(ctx, strings.TrimPrefix(uid.OpaqueId, "guest:"))
		} else {
			userData, err = m.getUserByParam(ctx, "upn", uid.OpaqueId)
		}
		if err != nil {
			return nil, err
		}
		u = m.parseAndCacheUser(ctx, userData)
	}

	if !skipFetchingGroups {
		userGroups, err := m.GetUserGroups(ctx, uid)
		if err != nil {
			return nil, err
		}
		u.Groups = userGroups
	}

	return u, nil
}

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string, skipFetchingGroups bool) (*userpb.User, error) {
	opaqueID, err := m.fetchCachedParam(claim, value)
	if err == nil {
		return m.GetUser(ctx, &userpb.UserId{OpaqueId: opaqueID}, skipFetchingGroups)
	}

	switch claim {
	case "mail":
		claim = "primaryAccountEmail"
	case "uid":
		claim = "uid"
	case "username":
		claim = "upn"
	default:
		return nil, errors.New("rest: invalid field: " + claim)
	}

	userData, err := m.getUserByParam(ctx, claim, value)
	if err != nil {
		// Lightweight accounts need to be fetched by email
		if strings.HasPrefix(value, "guest:") {
			if userData, err = m.getLightweightUser(ctx, strings.TrimPrefix(value, "guest:")); err != nil {
				return nil, err
			}
		}
	}
	u := m.parseAndCacheUser(ctx, userData)

	if !skipFetchingGroups {
		userGroups, err := m.GetUserGroups(ctx, u.Id)
		if err != nil {
			return nil, err
		}
		u.Groups = userGroups
	}

	return u, nil

}

func (m *manager) findUsersByFilter(ctx context.Context, url string, users map[string]*userpb.User, skipFetchingGroups bool) error {

	userData, err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false)
	if err != nil {
		return err
	}

	for _, usr := range userData {
		usrInfo, ok := usr.(map[string]interface{})
		if !ok {
			continue
		}

		upn, _ := usrInfo["upn"].(string)
		mail, _ := usrInfo["primaryAccountEmail"].(string)
		name, _ := usrInfo["displayName"].(string)
		uidNumber, _ := usrInfo["uid"].(float64)
		gidNumber, _ := usrInfo["gid"].(float64)
		t, _ := usrInfo["type"].(string)
		userType := getUserType(t, upn)

		if userType == userpb.UserType_USER_TYPE_APPLICATION || userType == userpb.UserType_USER_TYPE_FEDERATED {
			continue
		}

		uid := &userpb.UserId{
			OpaqueId: upn,
			Idp:      m.conf.IDProvider,
			Type:     userType,
		}
		var userGroups []string
		if !skipFetchingGroups {
			userGroups, err = m.GetUserGroups(ctx, uid)
			if err != nil {
				return err
			}
		}

		users[uid.OpaqueId] = &userpb.User{
			Id:          uid,
			Username:    upn,
			Mail:        mail,
			DisplayName: name,
			UidNumber:   int64(uidNumber),
			GidNumber:   int64(gidNumber),
			Groups:      userGroups,
		}
	}

	return nil
}

func (m *manager) FindUsers(ctx context.Context, query string, skipFetchingGroups bool) ([]*userpb.User, error) {

	// Look at namespaces filters. If the query starts with:
	// "a" => look into primary/secondary/service accounts
	// "l" => look into lightweight accounts
	// none => look into primary

	parts := strings.SplitN(query, ":", 2)

	var namespace string
	if len(parts) == 2 {
		// the query contains a namespace filter
		namespace, query = parts[0], parts[1]
	}

	var filters []string
	switch {
	case usernameRegex.MatchString(query):
		filters = []string{"upn", "displayName", "primaryAccountEmail"}
	case emailRegex.MatchString(query):
		filters = []string{"primaryAccountEmail"}
	default:
		return nil, errors.New("rest: illegal characters present in query: " + query)
	}

	users := make(map[string]*userpb.User)

	for _, f := range filters {
		url := fmt.Sprintf("%s/Identity/?filter=%s:contains:%s&field=id&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type",
			m.conf.APIBaseURL, f, url.QueryEscape(query))
		err := m.findUsersByFilter(ctx, url, users, skipFetchingGroups)
		if err != nil {
			return nil, err
		}
	}

	userSlice := []*userpb.User{}

	var accountsFilters []userpb.UserType
	switch namespace {
	case "":
		accountsFilters = []userpb.UserType{userpb.UserType_USER_TYPE_PRIMARY}
	case "a":
		accountsFilters = []userpb.UserType{userpb.UserType_USER_TYPE_PRIMARY, userpb.UserType_USER_TYPE_SECONDARY, userpb.UserType_USER_TYPE_SERVICE}
	case "l":
		accountsFilters = []userpb.UserType{userpb.UserType_USER_TYPE_LIGHTWEIGHT}
	}

	for _, u := range users {
		if isUserAnyType(u, accountsFilters) {
			userSlice = append(userSlice, u)
		}
	}

	return userSlice, nil
}

// isUserAnyType returns true if the user's type is one of types list
func isUserAnyType(user *userpb.User, types []userpb.UserType) bool {
	for _, t := range types {
		if user.GetId().Type == t {
			return true
		}
	}
	return false
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {

	groups, err := m.fetchCachedUserGroups(uid)
	if err == nil {
		return groups, nil
	}

	internalID, err := m.getInternalUserID(ctx, uid)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/Identity/%s/groups?recursive=true", m.conf.APIBaseURL, internalID)
	groupData, err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false)
	if err != nil {
		return nil, err
	}

	groups = []string{}

	for _, g := range groupData {
		groupInfo, ok := g.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}
		name, ok := groupInfo["displayName"].(string)
		if ok {
			groups = append(groups, name)
		}
	}

	if err = m.cacheUserGroups(uid, groups); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching user groups")
	}

	return groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, uid *userpb.UserId, group string) (bool, error) {
	userGroups, err := m.GetUserGroups(ctx, uid)
	if err != nil {
		return false, err
	}

	for _, g := range userGroups {
		if group == g {
			return true, nil
		}
	}
	return false, nil
}

func extractUID(u *userpb.User) (string, error) {
	if u.UidNumber == 0 {
		return "", errors.New("rest: could not retrieve UID from user")
	}
	return strconv.FormatInt(u.UidNumber, 10), nil
}

func getUserType(userType, upn string) userpb.UserType {
	var t userpb.UserType
	switch userType {
	case "Application":
		t = userpb.UserType_USER_TYPE_APPLICATION
	case "Service":
		t = userpb.UserType_USER_TYPE_SERVICE
	case "Secondary":
		t = userpb.UserType_USER_TYPE_SECONDARY
	case "Person":
		switch {
		case strings.HasPrefix(upn, "guest"):
			t = userpb.UserType_USER_TYPE_LIGHTWEIGHT
		case strings.Contains(upn, "@"):
			t = userpb.UserType_USER_TYPE_FEDERATED
		default:
			t = userpb.UserType_USER_TYPE_PRIMARY
		}
	default:
		t = userpb.UserType_USER_TYPE_INVALID
	}
	return t

}
