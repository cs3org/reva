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
	user "github.com/cs3org/reva/pkg/cbox/user/rest"
	utils "github.com/cs3org/reva/pkg/cbox/utils"
	"github.com/cs3org/reva/pkg/group"
	"github.com/cs3org/reva/pkg/group/manager/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/cs3org/reva/pkg/utils/list"
	"github.com/gomodule/redigo/redis"
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

func (c *config) ApplyDefaults() {
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

// New returns a user manager implementation that makes calls to the GRAPPA API.
func New(ctx context.Context, m map[string]interface{}) (group.Manager, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	redisPool := initRedisPool(c.RedisAddress, c.RedisUsername, c.RedisPassword)
	apiTokenManager, err := utils.InitAPITokenManager(m)
	if err != nil {
		return nil, err
	}

	mgr := &manager{
		conf:            &c,
		redisPool:       redisPool,
		apiTokenManager: apiTokenManager,
	}
	go mgr.fetchAllGroups(context.Background())
	return mgr, nil
}

func (m *manager) fetchAllGroups(ctx context.Context) {
	_ = m.fetchAllGroupAccounts(ctx)
	ticker := time.NewTicker(time.Duration(m.conf.GroupFetchInterval) * time.Second)
	work := make(chan os.Signal, 1)
	signal.Notify(work, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)

	for {
		select {
		case <-work:
			return
		case <-ticker.C:
			_ = m.fetchAllGroupAccounts(ctx)
		}
	}
}

// Group contains the information about a group.
type Group struct {
	GroupIdentifier  string `json:"groupIdentifier"`
	DisplayName      string `json:"displayName"`
	GID              int    `json:"gid,omitempty"`
	IsComputingGroup bool   `json:"isComputingGroup"`
}

// GroupsResponse contains the expected response from grappa
// when getting the list of groups.
type GroupsResponse struct {
	Pagination struct {
		Links struct {
			Next *string `json:"next"`
		} `json:"links"`
	} `json:"pagination"`
	Data []*Group `json:"data"`
}

func (m *manager) fetchAllGroupAccounts(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1.0/Group?field=groupIdentifier&field=displayName&field=gid&field=isComputingGroup", m.conf.APIBaseURL)

	for {
		var r GroupsResponse
		if err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false, &r); err != nil {
			return err
		}

		for _, g := range r.Data {
			if g.IsComputingGroup {
				continue
			}
			if _, err := m.parseAndCacheGroup(ctx, g); err != nil {
				continue
			}
		}

		if r.Pagination.Links.Next == nil {
			break
		}
		url = fmt.Sprintf("%s%s", m.conf.APIBaseURL, *r.Pagination.Links.Next)
	}

	return nil
}

func (m *manager) parseAndCacheGroup(ctx context.Context, g *Group) (*grouppb.Group, error) {
	groupID := &grouppb.GroupId{
		Idp:      m.conf.IDProvider,
		OpaqueId: g.GroupIdentifier,
	}

	group := &grouppb.Group{
		Id:          groupID,
		GroupName:   g.GroupIdentifier,
		Mail:        g.GroupIdentifier + "@cern.ch",
		DisplayName: g.DisplayName,
		GidNumber:   int64(g.GID),
	}

	if err := m.cacheGroupDetails(group); err != nil {
		log.Error().Err(err).Msg("rest: error caching group details")
	}

	return group, nil
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

	url := fmt.Sprintf("%s/api/v1.0/Group/%s/memberidentities/precomputed?limit=10&field=upn&field=primaryAccountEmail&field=displayName&field=uid&field=gid&field=type&field=source", m.conf.APIBaseURL, gid.OpaqueId)

	var r user.IdentitiesResponse
	members := []*userpb.UserId{}
	for {
		if err := m.apiTokenManager.SendAPIGetRequest(ctx, url, false, &r); err != nil {
			return nil, err
		}

		users := list.Map(r.Data, func(i *user.Identity) *userpb.UserId {
			return &userpb.UserId{OpaqueId: i.Upn, Idp: m.conf.IDProvider, Type: i.UserType()}
		})
		members = append(members, users...)

		if r.Pagination.Links.Next == nil {
			break
		}
		url = fmt.Sprintf("%s%s", m.conf.APIBaseURL, *r.Pagination.Links.Next)
	}

	if err = m.cacheGroupMembers(gid, members); err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("rest: error caching group members")
	}

	return users, nil
}

func (m *manager) HasMember(ctx context.Context, gid *grouppb.GroupId, uid *userpb.UserId) (bool, error) {
	// TODO (gdelmont): this can be improved storing the users a group is composed of as a list in redis
	// and, instead of returning all the members, use the redis apis to check if the user is in the list.
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
