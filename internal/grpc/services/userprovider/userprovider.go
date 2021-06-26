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
	"sort"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("userprovider", New)
}

type pluginConfig struct {
	Driver   string `mapstructure:"driver"`
	Location string `mapstructure:"location"`
	Users    string `mapstructure:"users"`
}

func (p *pluginConfig) load() (interface{}, error) {
	sym, err := plugin.Load(p.Location)
	if err != nil {
		return nil, err
	}
	return sym, nil
}

func parseConfig(m map[string]interface{}) (*pluginConfig, error) {
	plgConfig := &pluginConfig{}
	if err := mapstructure.Decode(m, plgConfig); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return plgConfig, nil
}

func getDriver(p *pluginConfig) (user.UserManager, error) {
	sym, err := p.load()
	if err != nil {
		err = errors.Wrap(err, "error loading plugin")
		return nil, err
	}

	manager, ok := sym.(user.UserManager)
	if !ok {
		return nil, fmt.Errorf("could not assert")
	}

	// New method initializes the manager struct in the plugin
	err = manager.New(p.Users)

	if err != nil {
		err = errors.Wrap(err, "could not call rpc method `New`")
		return nil, err
	}
	return manager, nil
}

// type config struct {
// 	Driver  string                            `mapstructure:"driver"`
// 	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
// }

// func (c *config) init() {
// 	if c.Driver == "" {
// 		c.Driver = "json"
// 	}
// }

// func parseConfig(m map[string]interface{}) (*config, error) {
// 	c := &config{}
// 	if err := mapstructure.Decode(m, c); err != nil {
// 		err = errors.Wrap(err, "error decoding conf")
// 		return nil, err
// 	}
// 	c.init()
// 	return c, nil
// }

// func getDriver(c *config) (user.Manager, error) {
// 	if f, ok := registry.NewFuncs[c.Driver]; ok {
// 		return f(c.Drivers[c.Driver])
// 	}

// 	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for user manager", c.Driver))
// }

// New returns a new UserProviderServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	userManager, err := getDriver(c)
	if err != nil {
		return nil, err
	}

	// Testing the plugin call
	user, _ := userManager.GetUser(context.Background(), &userpb.UserId{OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51", Idp: "cernbox.cern.ch"})
	fmt.Println(user.DisplayName)

	svc := &service{usermgr: userManager}

	return svc, nil
}

type service struct {
	usermgr user.UserManager
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
