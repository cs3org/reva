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

package userprovider

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("userprovider", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (c *config) load() (interface{}, error) {
	sym, err := plugin.Load(c.Driver, "userprovider")
	if err != nil {
		return nil, err
	}
	return sym, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	c.init()
	return c, nil
}

// getDriverPlugin fetches the runtime driver from the plugins package
func getDriverPlugin(c *config) (user.Manager, error) {
	sym, err := c.load()
	if err != nil {
		return nil, err
	}

	// assert the loaded plugin into required interface
	manager, ok := sym.(user.Manager)
	if !ok {
		return nil, fmt.Errorf("could not assert the loaded plugin")
	}

	pluginConfig := filepath.Base(c.Driver)
	err = manager.Configure(c.Drivers[pluginConfig])
	if err != nil {
		return nil, err
	}
	return manager, nil
}

func getDriver(c *config, plugin bool) (user.Manager, error) {
	// if plugin flag is set, we fetch the driver from the plugin package via hashicorp go-plugin system
	if plugin {
		mgr, err := getDriverPlugin(c)
		if err != nil {
			return nil, err
		}
		return mgr, nil
	}
	// fetch drivers from the in-memory registry
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for user manager", c.Driver))
}

// New returns a new UserProviderServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	plugin := sharedconf.GetPluginFlag()

	userManager, err := getDriver(c, plugin)
	if err != nil {
		return nil, err
	}
	svc := &service{usermgr: userManager}

	return svc, nil
}

type service struct {
	usermgr user.Manager
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.identity.user.v1beta1.UserAPI/GetUser", "/cs3.identity.user.v1beta1.UserAPI/GetUserByClaim"}
}

func (s *service) Register(ss *grpc.Server) {
	userpb.RegisterUserAPIServer(ss, s)
}

func (s *service) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	user, err := s.usermgr.GetUser(ctx, req.UserId)
	if err != nil {
		// TODO(labkode): check for not found.
		err = errors.Wrap(err, "userprovidersvc: error getting user")
		res := &userpb.GetUserResponse{
			Status: status.NewInternal(ctx, err, "error getting user"),
		}
		return res, nil
	}

	res := &userpb.GetUserResponse{
		Status: status.NewOK(ctx),
		User:   user,
	}
	return res, nil
}

func (s *service) GetUserByClaim(ctx context.Context, req *userpb.GetUserByClaimRequest) (*userpb.GetUserByClaimResponse, error) {
	user, err := s.usermgr.GetUserByClaim(ctx, req.Claim, req.Value)
	if err != nil {
		// TODO(labkode): check for not found.
		err = errors.Wrap(err, "userprovidersvc: error getting user by claim")
		res := &userpb.GetUserByClaimResponse{
			Status: status.NewInternal(ctx, err, "error getting user by claim"),
		}
		return res, nil
	}

	res := &userpb.GetUserByClaimResponse{
		Status: status.NewOK(ctx),
		User:   user,
	}
	return res, nil
}

func (s *service) FindUsers(ctx context.Context, req *userpb.FindUsersRequest) (*userpb.FindUsersResponse, error) {
	users, err := s.usermgr.FindUsers(ctx, req.Filter)
	if err != nil {
		err = errors.Wrap(err, "userprovidersvc: error finding users")
		res := &userpb.FindUsersResponse{
			Status: status.NewInternal(ctx, err, "error finding users"),
		}
		return res, nil
	}

	// sort users by username
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username <= users[j].Username
	})

	res := &userpb.FindUsersResponse{
		Status: status.NewOK(ctx),
		Users:  users,
	}
	return res, nil
}

func (s *service) GetUserGroups(ctx context.Context, req *userpb.GetUserGroupsRequest) (*userpb.GetUserGroupsResponse, error) {
	groups, err := s.usermgr.GetUserGroups(ctx, req.UserId)
	if err != nil {
		err = errors.Wrap(err, "userprovidersvc: error getting user groups")
		res := &userpb.GetUserGroupsResponse{
			Status: status.NewInternal(ctx, err, "error getting user groups"),
		}
		return res, nil
	}

	res := &userpb.GetUserGroupsResponse{
		Status: status.NewOK(ctx),
		Groups: groups,
	}
	return res, nil
}
