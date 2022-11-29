// Copyright 2018-2022 CERN
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
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	utils "github.com/cs3org/reva/pkg/cbox/utils"
	"github.com/cs3org/reva/pkg/group"
	"github.com/cs3org/reva/pkg/group/manager/registry"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
)

func init() {
	registry.Register("rest", New)
}

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
	// The time in minutes for which the members of a group would be cached
	GroupMembersCacheExpiration int `mapstructure:"group_members_cache_expiration" docs:"5"`
	// The OIDC Provider
	IDProvider string `mapstructure:"id_provider" docs:"http://cernbox.cern.ch"`
	// Base API Endpoint
	APIBaseURL string `mapstructure:"api_base_url" docs:"https://authorization-service-api-dev.web.cern.ch"`
	// Client ID needed to authenticate
	ClientID string `mapstructure:"client_id" docs:"-"`
	// Client Secret
	ClientSecret string `mapstructure:"client_secret" docs:"-"`

	// Endpoint to generate token to access the API
	OIDCTokenEndpoint string `mapstructure:"oidc_token_endpoint" docs:"https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"`
	// The target application for which token needs to be generated
	TargetAPI string `mapstructure:"target_api" docs:"authorization-service-api"`
	// The time in seconds between bulk fetch of groups
	GroupFetchInterval int `mapstructure:"group_fetch_interval" docs:"3600"`
}

func (c *config) init() {
	if c.GroupMembersCacheExpiration == 0 {
		c.GroupMembersCacheExpiration = 5
	}
	if c.RedisAddress == "" {
		c.RedisAddress = ":6379"
	}
	if c.APIBaseURL == "" {
		c.APIBaseURL = "https://authorization-service-api-dev.web.cern.ch"
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
	if c.GroupFetchInterval == 0 {
		c.GroupFetchInterval = 3600
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
	apiTokenManager, err := utils.InitAPITokenManager(m)
	if err != nil {
		return nil, err
	}

	mgr := &manager{
		conf:            c,
		redisPool:       redisPool,
		apiTokenManager: apiTokenManager,
	}
	go mgr.fetchAllGroups()
	return mgr, nil
}

func (m *manager) fetchAllGroups() {
	_ = m.fetchAllGroupAccounts()
	ticker := time.NewTicker(time.Duration(m.conf.GroupFetchInterval) * time.Second)
	work := make(chan os.Signal, 1)
	signal.Notify(work, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)

	for {
		select {
		case <-work:
			return
		case <-ticker.C:
			_ = m.fetchAllGroupAccounts()
		}
	}
}

type groupData struct {
	GroupIdentifier string `json:"groupIdentifier"`
	DisplayName     string `json:"displayName"`
	Gid             *int64 `json:"gid"`
	ID              string `json:"id"`
}

func (m *manager) fetchAllGroupAccounts() error {
	ctx := context.Background()
	url := fmt.Sprintf("%s/api/v1.0/Group?field=groupIdentifier&field=displayName&field=gid", m.conf.APIBaseURL)

	var result struct {
		Pagination struct {
			Links struct {
				Next *string `json:"next"`
			} `json:"links"`
			Token string `json:"token"`
		} `json:"pagination"`
		Data []*groupData `json:"data"`
	}

	for {
		if err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false, &result); err != nil {
			return err
		}

		for _, groupData := range result.Data {
			if _, err := m.parseAndCacheGroup(ctx, groupData); err != nil {
				continue
			}
		}

		if result.Pagination.Links.Next == nil {
			break
		}
		url = fmt.Sprintf("%s%s", m.conf.APIBaseURL, *result.Pagination.Links.Next)
	}

	return nil
}

func (m *manager) parseAndCacheGroup(ctx context.Context, groupData *groupData) (*grouppb.Group, error) {
	var gid int64
	if groupData.Gid != nil {
		gid = *groupData.Gid
	}
	groupID := &grouppb.GroupId{
		OpaqueId: groupData.GroupIdentifier,
		Idp:      m.conf.IDProvider,
	}

	g := &grouppb.Group{
		Id:          groupID,
		GroupName:   groupData.GroupIdentifier,
		Mail:        groupData.GroupIdentifier + "@cern.ch",
		DisplayName: groupData.DisplayName,
		GidNumber:   gid,
	}

	if err := m.cacheGroupDetails(g); err != nil {
		log.Error().Err(err).Msg("rest: error caching group details")
	}

	if err := m.cacheInternalID(groupID, groupData.ID); err != nil {
		log.Error().Err(err).Msg("rest: error caching group details")
	}

	return g, nil
}

func (m *manager) GetGroup(ctx context.Context, gid *grouppb.GroupId, skipFetchingMembers bool) (*grouppb.Group, error) {
	g, err := m.fetchCachedGroupDetails(gid)
	if err != nil {
		return nil, err
	}

	if !skipFetchingMembers {
		groupMembers, err := m.GetMembers(ctx, gid)
		if err != nil {
			return nil, err
		}
		g.Members = groupMembers
	}

	return g, nil
}

func (m *manager) GetGroupByClaim(ctx context.Context, claim, value string, skipFetchingMembers bool) (*grouppb.Group, error) {
	if claim == "group_name" {
		return m.GetGroup(ctx, &grouppb.GroupId{OpaqueId: value}, skipFetchingMembers)
	}

	g, err := m.fetchCachedGroupByParam(claim, value)
	if err != nil {
		return nil, err
	}

	if !skipFetchingMembers {
		groupMembers, err := m.GetMembers(ctx, g.Id)
		if err != nil {
			return nil, err
		}
		g.Members = groupMembers
	}

	return g, nil
}

func (m *manager) FindGroups(ctx context.Context, query string, skipFetchingMembers bool) ([]*grouppb.Group, error) {
	// Look at namespaces filters. If the query starts with:
	// "a" or none => get egroups
	// other filters => get empty list

	parts := strings.SplitN(query, ":", 2)

	if len(parts) == 2 {
		if parts[0] == "a" {
			query = parts[1]
		} else {
			return []*grouppb.Group{}, nil
		}
	}

	return m.findCachedGroups(query)
}

func (m *manager) GetMembers(ctx context.Context, gid *grouppb.GroupId) ([]*userpb.UserId, error) {
	users, err := m.fetchCachedGroupMembers(gid)
	if err == nil {
		return users, nil
	}

	internalID, err := m.fetchCachedInternalID(gid)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/v1.0/Group/%s/memberidentities/precomputed", m.conf.APIBaseURL, internalID)

	var result struct {
		Data []struct {
			Upn *string `json:"upn"`
		} `json:"data"`
	}
	if err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false, &result); err != nil {
		return nil, err
	}

	users = make([]*userpb.UserId, 0, len(result.Data))

	for _, u := range result.Data {
		if u.Upn != nil {
			users = append(users, &userpb.UserId{
				// TODO: the type is missing
				OpaqueId: *u.Upn,
				Idp:      m.conf.IDProvider,
			})
		}
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
