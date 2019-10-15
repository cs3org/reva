// Copyright 2018-2019 CERN
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

package userprovidersvc

import (
	"context"
	"fmt"
	"io"

	userproviderpb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("userprovidersvc", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func getDriver(c *config) (user.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, fmt.Errorf("driver %s not found for user manager", c.Driver)
}

// New returns a new UserProviderServiceServer.
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	userManager, err := getDriver(c)
	if err != nil {
		return nil, err
	}

	svc := &service{usermgr: userManager}
	userproviderpb.RegisterUserProviderServiceServer(ss, svc)

	return svc, nil
}

type service struct {
	usermgr user.Manager
}

func (s *service) Close() error {
	return nil
}

func (s *service) GetUser(ctx context.Context, req *userproviderpb.GetUserRequest) (*userproviderpb.GetUserResponse, error) {
	user, err := s.usermgr.GetUser(ctx, req.UserId)
	if err != nil {
		// TODO(labkode): check for not found.
		err = errors.Wrap(err, "userprovidersvc: error getting user")
		res := &userproviderpb.GetUserResponse{
			Status: status.NewInternal(ctx, err, "error authenticating user"),
		}
		return res, nil
	}

	res := &userproviderpb.GetUserResponse{
		Status: status.NewOK(ctx),
		User:   user,
	}
	return res, nil
}

func (s *service) FindUsers(ctx context.Context, req *userproviderpb.FindUsersRequest) (*userproviderpb.FindUsersResponse, error) {
	users, err := s.usermgr.FindUsers(ctx, req.Filter)
	if err != nil {
		err = errors.Wrap(err, "userprovidersvc: error finding users")
		res := &userproviderpb.FindUsersResponse{
			Status: status.NewInternal(ctx, err, "error finding users"),
		}
		return res, nil
	}

	res := &userproviderpb.FindUsersResponse{
		Status: status.NewOK(ctx),
		Users:  users,
	}
	return res, nil
}

func (s *service) GetUserGroups(ctx context.Context, req *userproviderpb.GetUserGroupsRequest) (*userproviderpb.GetUserGroupsResponse, error) {
	groups, err := s.usermgr.GetUserGroups(ctx, req.UserId)
	if err != nil {
		err = errors.Wrap(err, "userprovidersvc: error getting user groups")
		res := &userproviderpb.GetUserGroupsResponse{
			Status: status.NewInternal(ctx, err, "error getting user groups"),
		}
		return res, nil
	}

	res := &userproviderpb.GetUserGroupsResponse{
		Status: status.NewOK(ctx),
		Groups: groups,
	}
	return res, nil
}

func (s *service) IsInGroup(ctx context.Context, req *userproviderpb.IsInGroupRequest) (*userproviderpb.IsInGroupResponse, error) {
	ok, err := s.usermgr.IsInGroup(ctx, req.UserId, req.Group)
	if err != nil {
		err = errors.Wrap(err, "userprovidersvc: error checking if user belongs to group")
		res := &userproviderpb.IsInGroupResponse{
			Status: status.NewInternal(ctx, err, "error checking if user belongs to group"),
		}
		return res, nil
	}

	res := &userproviderpb.IsInGroupResponse{
		Status: status.NewOK(ctx),
		Ok:     ok,
	}

	return res, nil
}
