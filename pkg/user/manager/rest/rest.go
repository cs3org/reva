// Copyright 2018-2020 CERN
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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
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
	conf      *config
	redisPool *redis.Pool
	oidcToken OIDCToken
}

// OIDCToken stores the OIDC token used to authenticate requests to the REST API service
type OIDCToken struct {
	sync.Mutex          // concurrent access to apiToken and tokenExpirationTime
	apiToken            string
	tokenExpirationTime time.Time
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
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	redisPool := initRedisPool(c.RedisAddress, c.RedisUsername, c.RedisPassword)
	return &manager{
		conf:      c,
		redisPool: redisPool,
	}, nil
}

func (m *manager) renewAPIToken(ctx context.Context) error {
	// Received tokens have an expiration time of 20 minutes.
	// Take a couple of seconds as buffer time for the API call to complete
	if m.oidcToken.tokenExpirationTime.Before(time.Now().Add(time.Second * time.Duration(2))) {
		token, expiration, err := m.getAPIToken(ctx)
		if err != nil {
			return err
		}

		m.oidcToken.Lock()
		defer m.oidcToken.Unlock()

		m.oidcToken.apiToken = token
		m.oidcToken.tokenExpirationTime = expiration
	}
	return nil
}

func (m *manager) getAPIToken(ctx context.Context) (string, time.Time, error) {

	params := url.Values{
		"grant_type": {"client_credentials"},
		"audience":   {m.conf.TargetAPI},
	}

	httpClient := rhttp.GetHTTPClient(rhttp.Context(ctx), rhttp.Timeout(10*time.Second), rhttp.Insecure(true))
	httpReq, err := http.NewRequest("POST", m.conf.OIDCTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq.SetBasicAuth(m.conf.ClientID, m.conf.ClientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return "", time.Time{}, err
	}
	defer httpRes.Body.Close()

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", time.Time{}, err
	}

	expirationSecs := result["expires_in"].(float64)
	expirationTime := time.Now().Add(time.Second * time.Duration(expirationSecs))
	return result["access_token"].(string), expirationTime, nil
}

func (m *manager) sendAPIRequest(ctx context.Context, url string) ([]interface{}, error) {
	err := m.renewAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	httpClient := rhttp.GetHTTPClient(rhttp.Context(ctx), rhttp.Timeout(10*time.Second), rhttp.Insecure(true))
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// We don't need to take the lock when reading apiToken, because if we reach here,
	// the token is valid at least for a couple of seconds. Even if another request modifies
	// the token and expiration time while this request is in progress, the current token will still be valid.
	httpReq.Header.Set("Authorization", "Bearer "+m.oidcToken.apiToken)

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	responseData, ok := result["data"].([]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	return responseData, nil
}

func (m *manager) getUserByParam(ctx context.Context, param, val string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/Identity?filter=%s:%s&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid",
		m.conf.APIBaseURL, param, val)
	responseData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	userData, ok := responseData[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}
	return userData, nil
}

func (m *manager) getInternalUserID(ctx context.Context, uid *userpb.UserId) (string, error) {

	internalID, err := m.fetchCachedInternalID(uid)
	if err != nil {
		userData, err := m.getUserByParam(ctx, "upn", uid.OpaqueId)
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
	userID := &userpb.UserId{
		OpaqueId: userData["upn"].(string),
		Idp:      m.conf.IDProvider,
	}
	u := &userpb.User{
		Id:          userID,
		Username:    userData["upn"].(string),
		Mail:        userData["primaryAccountEmail"].(string),
		DisplayName: userData["displayName"].(string),
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"uid": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(fmt.Sprintf("%0.f", userData["uid"])),
				},
				"gid": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(fmt.Sprintf("%0.f", userData["gid"])),
				},
			},
		},
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

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {

	u, err := m.fetchCachedUserDetails(uid)
	if err != nil {
		userData, err := m.getUserByParam(ctx, "upn", uid.OpaqueId)
		if err != nil {
			return nil, err
		}
		u = m.parseAndCacheUser(ctx, userData)
	}

	userGroups, err := m.GetUserGroups(ctx, uid)
	if err != nil {
		return nil, err
	}
	u.Groups = userGroups

	return u, nil
}

func (m *manager) GetUserByUID(ctx context.Context, uid string) (*userpb.User, error) {
	opaqueID, err := m.fetchCachedUID(uid)
	if err == nil {
		return m.GetUser(ctx, &userpb.UserId{OpaqueId: opaqueID})
	}

	userData, err := m.getUserByParam(ctx, "uid", uid)
	if err != nil {
		return nil, err
	}
	u := m.parseAndCacheUser(ctx, userData)

	userGroups, err := m.GetUserGroups(ctx, u.Id)
	if err != nil {
		return nil, err
	}
	u.Groups = userGroups

	return u, nil

}

func (m *manager) findUsersByFilter(ctx context.Context, url string) ([]*userpb.User, error) {

	userData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	users := []*userpb.User{}

	for _, usr := range userData {
		usrInfo, ok := usr.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}

		uid := &userpb.UserId{
			OpaqueId: usrInfo["upn"].(string),
			Idp:      m.conf.IDProvider,
		}
		users = append(users, &userpb.User{
			Id:          uid,
			Username:    usrInfo["upn"].(string),
			Mail:        usrInfo["primaryAccountEmail"].(string),
			DisplayName: usrInfo["displayName"].(string),
			Opaque: &types.Opaque{
				Map: map[string]*types.OpaqueEntry{
					"uid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte(fmt.Sprintf("%0.f", usrInfo["uid"])),
					},
					"gid": &types.OpaqueEntry{
						Decoder: "plain",
						Value:   []byte(fmt.Sprintf("%0.f", usrInfo["gid"])),
					},
				},
			},
		})
	}

	return users, nil
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {

	var filters []string
	switch {
	case usernameRegex.MatchString(query):
		filters = []string{"upn", "displayName", "primaryAccountEmail"}
	case emailRegex.MatchString(query):
		filters = []string{"primaryAccountEmail"}
	default:
		return nil, errors.New("rest: illegal characters present in query")
	}

	users := []*userpb.User{}

	for _, f := range filters {
		url := fmt.Sprintf("%s/Identity/?filter=%s:contains:%s&field=id&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid",
			m.conf.APIBaseURL, f, query)
		filteredUsers, err := m.findUsersByFilter(ctx, url)
		if err != nil {
			return nil, err
		}
		users = append(users, filteredUsers...)
	}
	return users, nil
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
	url := fmt.Sprintf("%s/Identity/%s/groups", m.conf.APIBaseURL, internalID)
	groupData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	groups = []string{}

	for _, g := range groupData {
		groupInfo, ok := g.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}
		groups = append(groups, groupInfo["displayName"].(string))
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
	if u.Opaque != nil && u.Opaque.Map != nil {
		if uidObj, ok := u.Opaque.Map["uid"]; ok {
			if uidObj.Decoder == "plain" {
				return string(uidObj.Value), nil
			}
		}
	}
	return "", errors.New("rest: could not retrieve UID from user")
}
