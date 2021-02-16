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
	client    *http.Client
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
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(10*time.Second),
			rhttp.Insecure(true),
		),
	}, nil
}

func (m *manager) renewAPIToken(ctx context.Context, forceRenewal bool) error {
	// Received tokens have an expiration time of 20 minutes.
	// Take a couple of seconds as buffer time for the API call to complete
	if forceRenewal || m.oidcToken.tokenExpirationTime.Before(time.Now().Add(time.Second*time.Duration(2))) {
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

	httpReq, err := http.NewRequest("POST", m.conf.OIDCTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq.SetBasicAuth(m.conf.ClientID, m.conf.ClientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

	httpRes, err := m.client.Do(httpReq)
	if err != nil {
		return "", time.Time{}, err
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
		return "", time.Time{}, errors.New("rest: get token endpoint returned " + httpRes.Status)
	}

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

func (m *manager) sendAPIRequest(ctx context.Context, url string, forceRenewal bool) ([]interface{}, error) {
	err := m.renewAPIToken(ctx, forceRenewal)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// We don't need to take the lock when reading apiToken, because if we reach here,
	// the token is valid at least for a couple of seconds. Even if another request modifies
	// the token and expiration time while this request is in progress, the current token will still be valid.
	httpReq.Header.Set("Authorization", "Bearer "+m.oidcToken.apiToken)

	httpRes, err := m.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode == http.StatusUnauthorized {
		// The token is no longer valid, try renewing it
		return m.sendAPIRequest(ctx, url, true)
	}
	if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
		return nil, errors.New("rest: API request returned " + httpRes.Status)
	}

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
	url := fmt.Sprintf("%s/Identity?filter=%s:%s&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type",
		m.conf.APIBaseURL, param, val)
	responseData, err := m.sendAPIRequest(ctx, url, false)
	if err != nil {
		return nil, err
	}
	if len(responseData) != 1 {
		return nil, errors.New("rest: user not found")
	}

	userData, ok := responseData[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	if userData["type"].(string) == "Application" || strings.HasPrefix(userData["upn"].(string), "guest") {
		return nil, errors.New("rest: guest and application accounts not supported")
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
	upn, _ := userData["upn"].(string)
	mail, _ := userData["primaryAccountEmail"].(string)
	name, _ := userData["displayName"].(string)

	userID := &userpb.UserId{
		OpaqueId: upn,
		Idp:      m.conf.IDProvider,
	}
	u := &userpb.User{
		Id:          userID,
		Username:    upn,
		Mail:        mail,
		DisplayName: name,
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

func (m *manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	opaqueID, err := m.fetchCachedParam(claim, value)
	if err == nil {
		return m.GetUser(ctx, &userpb.UserId{OpaqueId: opaqueID})
	}

	switch claim {
	case "mail":
		claim = "primaryAccountEmail"
	case "uid":
		claim = "uid"
	case "username":
		claim = "upn"
	default:
		return nil, errors.New("rest: invalid field")
	}

	userData, err := m.getUserByParam(ctx, claim, value)
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

func (m *manager) findUsersByFilter(ctx context.Context, url string, users map[string]*userpb.User) error {

	userData, err := m.sendAPIRequest(ctx, url, false)
	if err != nil {
		return err
	}

	for _, usr := range userData {
		usrInfo, ok := usr.(map[string]interface{})
		if !ok || usrInfo["type"].(string) == "Application" || strings.HasPrefix(usrInfo["upn"].(string), "guest") {
			continue
		}

		upn, _ := usrInfo["upn"].(string)
		mail, _ := usrInfo["primaryAccountEmail"].(string)
		name, _ := usrInfo["displayName"].(string)

		uid := &userpb.UserId{
			OpaqueId: upn,
			Idp:      m.conf.IDProvider,
		}
		users[uid.OpaqueId] = &userpb.User{
			Id:          uid,
			Username:    upn,
			Mail:        mail,
			DisplayName: name,
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
		}
	}

	return nil
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

	users := make(map[string]*userpb.User)

	for _, f := range filters {
		url := fmt.Sprintf("%s/Identity/?filter=%s:contains:%s&field=id&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type",
			m.conf.APIBaseURL, f, query)
		err := m.findUsersByFilter(ctx, url, users)
		if err != nil {
			return nil, err
		}
	}

	userSlice := []*userpb.User{}
	for _, v := range users {
		userSlice = append(userSlice, v)
	}

	return userSlice, nil
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
	groupData, err := m.sendAPIRequest(ctx, url, false)
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
	if u.Opaque != nil && u.Opaque.Map != nil {
		if uidObj, ok := u.Opaque.Map["uid"]; ok {
			if uidObj.Decoder == "plain" {
				return string(uidObj.Value), nil
			}
		}
	}
	return "", errors.New("rest: could not retrieve UID from user")
}
