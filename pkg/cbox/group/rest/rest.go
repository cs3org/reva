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

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/group"
	"github.com/cs3org/reva/pkg/group/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("rest", New)
}

var (
	emailRegex     = regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	groupNameRegex = regexp.MustCompile(`^[ a-zA-Z0-9._-]+$`)
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
	// The time in minutes for which the members of a group would be cached
	GroupMembersCacheExpiration int `mapstructure:"group_members_cache_expiration" docs:"5"`
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
	if c.GroupMembersCacheExpiration == 0 {
		c.GroupMembersCacheExpiration = 5
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
func New(m map[string]interface{}) (group.Manager, error) {
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

func (m *manager) getGroupByParam(ctx context.Context, param, val string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/Group?filter=%s:%s&field=groupIdentifier&field=displayName&field=gid",
		m.conf.APIBaseURL, param, val)
	responseData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	if len(responseData) == 0 {
		return nil, errors.New("rest: no user found")
	}

	userData, ok := responseData[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}
	return userData, nil
}

func (m *manager) getInternalGroupID(ctx context.Context, gid *grouppb.GroupId) (string, error) {

	internalID, err := m.fetchCachedInternalID(gid)
	if err != nil {
		groupData, err := m.getGroupByParam(ctx, "groupIdentifier", gid.OpaqueId)
		if err != nil {
			return "", err
		}
		id, ok := groupData["id"].(string)
		if !ok {
			return "", errors.New("rest: error in type assertion")
		}

		if err = m.cacheInternalID(gid, id); err != nil {
			log := appctx.GetLogger(ctx)
			log.Error().Err(err).Msg("rest: error caching group details")
		}
		return id, nil
	}
	return internalID, nil
}

func (m *manager) parseAndCacheGroup(ctx context.Context, groupData map[string]interface{}) *grouppb.Group {
	groupID := &grouppb.GroupId{
		OpaqueId: groupData["groupIdentifier"].(string),
		Idp:      m.conf.IDProvider,
	}
	gid, ok := groupData["gid"].(int64)
	if !ok {
		gid = 0
	}
	g := &grouppb.Group{
		Id:          groupID,
		GroupName:   groupData["groupIdentifier"].(string),
		Mail:        groupData["groupIdentifier"].(string) + "@cern.ch",
		DisplayName: groupData["displayName"].(string),
		GidNumber:   gid,
	}

	if err := m.cacheGroupDetails(g); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching group details")
	}
	if err := m.cacheInternalID(groupID, groupData["id"].(string)); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching group details")
	}
	return g

}

func (m *manager) GetGroup(ctx context.Context, gid *grouppb.GroupId) (*grouppb.Group, error) {

	g, err := m.fetchCachedGroupDetails(gid)
	if err != nil {
		groupData, err := m.getGroupByParam(ctx, "groupIdentifier", gid.OpaqueId)
		if err != nil {
			return nil, err
		}
		g = m.parseAndCacheGroup(ctx, groupData)
	}

	groupMembers, err := m.GetMembers(ctx, gid)
	if err != nil {
		return nil, err
	}
	g.Members = groupMembers

	return g, nil
}

func (m *manager) GetGroupByClaim(ctx context.Context, claim, value string) (*grouppb.Group, error) {
	opaqueID, err := m.fetchCachedParam(claim, value)
	if err == nil {
		return m.GetGroup(ctx, &grouppb.GroupId{OpaqueId: opaqueID})
	}

	switch claim {
	case "mail":
		claim = "groupIdentifier"
		value = strings.TrimSuffix(value, "@cern.ch")
	case "gid_number":
		claim = "gid"
	case "group_name":
		claim = "groupIdentifier"
	default:
		return nil, errors.New("rest: invalid field")
	}

	groupData, err := m.getGroupByParam(ctx, claim, value)
	if err != nil {
		return nil, err
	}
	g := m.parseAndCacheGroup(ctx, groupData)

	groupMembers, err := m.GetMembers(ctx, g.Id)
	if err != nil {
		return nil, err
	}
	g.Members = groupMembers

	return g, nil

}

func (m *manager) findGroupsByFilter(ctx context.Context, url string, groups map[string]*grouppb.Group) error {

	groupData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return err
	}

	for _, grp := range groupData {
		grpInfo, ok := grp.(map[string]interface{})
		if !ok {
			return errors.New("rest: error in type assertion")
		}

		groupID := &grouppb.GroupId{
			OpaqueId: grpInfo["groupIdentifier"].(string),
			Idp:      m.conf.IDProvider,
		}
		gid, ok := grpInfo["gid"].(int64)
		if !ok {
			gid = 0
		}
		groups[groupID.OpaqueId] = &grouppb.Group{
			Id:          groupID,
			GroupName:   grpInfo["groupIdentifier"].(string),
			Mail:        grpInfo["groupIdentifier"].(string) + "@cern.ch",
			DisplayName: grpInfo["displayName"].(string),
			GidNumber:   gid,
		}
	}

	return nil
}

func (m *manager) FindGroups(ctx context.Context, query string) ([]*grouppb.Group, error) {

	var filters []string
	switch {
	case groupNameRegex.MatchString(query):
		filters = []string{"groupidentifier", "displayName"}
	case emailRegex.MatchString(query):
		filters = []string{"groupidentifier"}
		query = strings.TrimSuffix(query, "@cern.ch")
	default:
		return nil, errors.New("rest: illegal characters present in query")
	}

	groups := make(map[string]*grouppb.Group)

	for _, f := range filters {
		url := fmt.Sprintf("%s/Group/?filter=%s:contains:%s&field=groupIdentifier&field=displayName&field=gid",
			m.conf.APIBaseURL, f, query)
		err := m.findGroupsByFilter(ctx, url, groups)
		if err != nil {
			return nil, err
		}
	}

	groupSlice := []*grouppb.Group{}
	for _, v := range groups {
		groupSlice = append(groupSlice, v)
	}

	return groupSlice, nil
}

func (m *manager) GetMembers(ctx context.Context, gid *grouppb.GroupId) ([]*userpb.UserId, error) {

	users, err := m.fetchCachedGroupMembers(gid)
	if err == nil {
		return users, nil
	}

	internalID, err := m.getInternalGroupID(ctx, gid)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/Group/%s/memberidentities/precomputed", m.conf.APIBaseURL, internalID)
	userData, err := m.sendAPIRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	users = []*userpb.UserId{}

	for _, u := range userData {
		userInfo, ok := u.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}
		users = append(users, &userpb.UserId{OpaqueId: userInfo["upn"].(string), Idp: m.conf.IDProvider})
	}

	if err = m.cacheGroupMembers(gid, users); err != nil {
		log := appctx.GetLogger(ctx)
		log.Error().Err(err).Msg("rest: error caching group members")
	}

	return users, nil
}

func (m *manager) HasMember(ctx context.Context, gid *grouppb.GroupId, uid *userpb.UserId) (bool, error) {
	groupMemers, err := m.GetMembers(ctx, gid)
	if err != nil {
		return false, err
	}

	for _, u := range groupMemers {
		if uid.OpaqueId == u.OpaqueId {
			return true, nil
		}
	}
	return false, nil
}
