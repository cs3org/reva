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

package gatewaysvc

import (
	"context"

	userproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/userprovider/v0alpha"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) GetUser(ctx context.Context, req *userproviderv0alphapb.GetUserRequest) (*userproviderv0alphapb.GetUserResponse, error) {
	c, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		return &userproviderv0alphapb.GetUserResponse{
			Status: status.NewInternal(ctx, err, "error getting auth client"),
		}, nil
	}

	res, err := c.GetUser(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetUser")
	}

	return res, nil
}

func (s *svc) FindUsers(ctx context.Context, req *userproviderv0alphapb.FindUsersRequest) (*userproviderv0alphapb.FindUsersResponse, error) {
	c, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		return &userproviderv0alphapb.FindUsersResponse{
			Status: status.NewInternal(ctx, err, "error getting auth client"),
		}, nil
	}

	res, err := c.FindUsers(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetUser")
	}

	return res, nil
}

func (s *svc) GetUserGroups(ctx context.Context, req *userproviderv0alphapb.GetUserGroupsRequest) (*userproviderv0alphapb.GetUserGroupsResponse, error) {
	c, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		return &userproviderv0alphapb.GetUserGroupsResponse{
			Status: status.NewInternal(ctx, err, "error getting auth client"),
		}, nil
	}

	res, err := c.GetUserGroups(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetUser")
	}

	return res, nil
}

func (s *svc) IsInGroup(ctx context.Context, req *userproviderv0alphapb.IsInGroupRequest) (*userproviderv0alphapb.IsInGroupResponse, error) {
	c, err := pool.GetUserProviderServiceClient(s.c.UserProviderEndpoint)
	if err != nil {
		return &userproviderv0alphapb.IsInGroupResponse{
			Status: status.NewInternal(ctx, err, "error getting auth client"),
		}, nil
	}

	res, err := c.IsInGroup(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gatewaysvc: error calling GetUser")
	}

	return res, nil
}
