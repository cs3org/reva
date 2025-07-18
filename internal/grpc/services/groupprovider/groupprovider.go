// Copyright 2018-2024 CERN
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

package groupprovider

import (
	"context"
	"fmt"
	"sort"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/group"
	"github.com/cs3org/reva/v3/pkg/group/manager/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("groupprovider", New)
	plugin.RegisterNamespace("grpc.services.groupprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func getDriver(ctx context.Context, c *config) (group.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}

	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for group manager", c.Driver))
}

// New returns a new GroupProviderServiceServer.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	groupManager, err := getDriver(ctx, &c)
	if err != nil {
		return nil, err
	}

	svc := &service{groupmgr: groupManager}

	return svc, nil
}

type service struct {
	groupmgr group.Manager
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	grouppb.RegisterGroupAPIServer(ss, s)
}

func (s *service) GetGroup(ctx context.Context, req *grouppb.GetGroupRequest) (*grouppb.GetGroupResponse, error) {
	group, err := s.groupmgr.GetGroup(ctx, req.GroupId, req.SkipFetchingMembers)
	if err != nil {
		res := &grouppb.GetGroupResponse{}
		if _, ok := err.(errtypes.NotFound); ok {
			res.Status = status.NewNotFound(ctx, "group not found")
		} else {
			err = errors.Wrap(err, "groupprovidersvc: error getting group")
			res.Status = status.NewInternal(ctx, err, "error getting group")
		}
		return res, nil
	}

	return &grouppb.GetGroupResponse{
		Status: status.NewOK(ctx),
		Group:  group,
	}, nil
}

func (s *service) GetGroupByClaim(ctx context.Context, req *grouppb.GetGroupByClaimRequest) (*grouppb.GetGroupByClaimResponse, error) {
	group, err := s.groupmgr.GetGroupByClaim(ctx, req.Claim, req.Value, req.SkipFetchingMembers)
	if err != nil {
		res := &grouppb.GetGroupByClaimResponse{}
		if _, ok := err.(errtypes.NotFound); ok {
			res.Status = status.NewNotFound(ctx, fmt.Sprintf("group not found %s %s", req.Claim, req.Value))
		} else {
			err = errors.Wrap(err, "groupprovidersvc: error getting group by claim")
			res.Status = status.NewInternal(ctx, err, "error getting group by claim")
		}
		return res, nil
	}

	return &grouppb.GetGroupByClaimResponse{
		Status: status.NewOK(ctx),
		Group:  group,
	}, nil
}

func (s *service) FindGroups(ctx context.Context, req *grouppb.FindGroupsRequest) (*grouppb.FindGroupsResponse, error) {
	groups, err := s.groupmgr.FindGroups(ctx, req.Filter, req.SkipFetchingMembers)
	if err != nil {
		err = errors.Wrap(err, "groupprovidersvc: error finding groups")
		return &grouppb.FindGroupsResponse{
			Status: status.NewInternal(ctx, err, "error finding groups"),
		}, nil
	}

	// sort group by groupname
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GroupName <= groups[j].GroupName
	})

	return &grouppb.FindGroupsResponse{
		Status: status.NewOK(ctx),
		Groups: groups,
	}, nil
}

func (s *service) GetMembers(ctx context.Context, req *grouppb.GetMembersRequest) (*grouppb.GetMembersResponse, error) {
	members, err := s.groupmgr.GetMembers(ctx, req.GroupId)
	if err != nil {
		err = errors.Wrap(err, "groupprovidersvc: error getting group members")
		return &grouppb.GetMembersResponse{
			Status: status.NewInternal(ctx, err, "error getting group members"),
		}, nil
	}

	return &grouppb.GetMembersResponse{
		Status:  status.NewOK(ctx),
		Members: members,
	}, nil
}

func (s *service) HasMember(ctx context.Context, req *grouppb.HasMemberRequest) (*grouppb.HasMemberResponse, error) {
	ok, err := s.groupmgr.HasMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		err = errors.Wrap(err, "groupprovidersvc: error checking for group member")
		return &grouppb.HasMemberResponse{
			Status: status.NewInternal(ctx, err, "error checking for group member"),
		}, nil
	}

	return &grouppb.HasMemberResponse{
		Status: status.NewOK(ctx),
		Ok:     ok,
	}, nil
}
