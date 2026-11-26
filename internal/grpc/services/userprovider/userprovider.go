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

package userprovider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/cs3org/reva/v3/pkg/user"
	"github.com/cs3org/reva/v3/pkg/user/manager/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func init() {
	rgrpc.Register("userprovider", New)
	plugin.RegisterNamespace("grpc.services.userprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                    `mapstructure:"driver"`
	Drivers map[string]map[string]any `mapstructure:"drivers"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func getDriver(ctx context.Context, c *config) (user.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		mgr, err := f(ctx, c.Drivers[c.Driver])
		return mgr, err
	}
	return nil, errtypes.NotFound(fmt.Sprintf("driver %s not found for user manager", c.Driver))
}

// New returns a new UserProviderServiceServer.
func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	userManager, err := getDriver(ctx, &c)
	if err != nil {
		return nil, err
	}
	svc := &service{
		usermgr: userManager,
	}

	return svc, nil
}

type service struct {
	usermgr user.Manager
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.identity.user.v1beta1.UserAPI/GetUser", "/cs3.identity.user.v1beta1.UserAPI/GetUserByClaim", "/cs3.identity.user.v1beta1.UserAPI/GetUserGroups"}
}

func (s *service) Register(ss *grpc.Server) {
	userpb.RegisterUserAPIServer(ss, s)
}

func (s *service) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	user, err := s.usermgr.GetUser(ctx, req.UserId, req.SkipFetchingUserGroups)
	if err != nil {
		res := &userpb.GetUserResponse{}
		if _, ok := err.(errtypes.NotFound); ok {
			res.Status = status.NewNotFound(ctx, "user not found")
		} else {
			res.Status = status.NewStatusFromErrType(ctx, "error getting user", err)
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
	user, err := s.usermgr.GetUserByClaim(ctx, req.Claim, req.Value, req.SkipFetchingUserGroups)
	if err != nil {
		res := &userpb.GetUserByClaimResponse{}
		if req.Claim == "userid" {
			res.Status = useridLookupStatus(ctx, err)
			return res, nil
		}
		switch err.(type) {
		case errtypes.NotFound:
			res.Status = status.NewNotFound(ctx, fmt.Sprintf("user not found %s %s", req.Claim, req.Value))
		case errtypes.Conflict:
			res.Status = status.NewConflict(ctx, err, fmt.Sprintf("conflict getting user %s by claim %s", req.Value, req.Claim))
		default:
			res.Status = status.NewStatusFromErrType(ctx, fmt.Sprintf("error getting user %s by claim %s", req.Value, req.Claim), err)
		}
		return res, nil
	}

	res := &userpb.GetUserByClaimResponse{
		Status: status.NewOK(ctx),
		User:   user,
	}
	return res, nil
}

func useridLookupStatus(ctx context.Context, err error) *rpc.Status {
	// Match NewStatusFromErrType without formatting or logging the source error.
	code := rpc.Code_CODE_INTERNAL
	switch err.(type) {
	case errtypes.IsNotFound:
		code = rpc.Code_CODE_NOT_FOUND
	case errtypes.IsInvalidCredentials, errtypes.IsUserRequired:
		code = rpc.Code_CODE_UNAUTHENTICATED
	case errtypes.IsPermissionDenied:
		code = rpc.Code_CODE_PERMISSION_DENIED
	case errtypes.IsNotSupported:
		code = rpc.Code_CODE_UNIMPLEMENTED
	case errtypes.IsBadRequest:
		code = rpc.Code_CODE_INVALID_ARGUMENT
	case errtypes.IsAlreadyExists:
		code = rpc.Code_CODE_ALREADY_EXISTS
	case errtypes.IsConflict:
		code = rpc.Code_CODE_ABORTED
	case errtypes.IsInsufficientStorage:
		code = rpc.Code_CODE_INSUFFICIENT_STORAGE
	default:
		for cause := err; cause != nil && code == rpc.Code_CODE_INTERNAL; cause = errors.Unwrap(cause) {
			var grpcErr interface{ GRPCStatus() *grpcstatus.Status }
			if !errors.As(cause, &grpcErr) {
				break
			}
			switch grpcErr.GRPCStatus().Code() {
			case codes.NotFound:
				code = rpc.Code_CODE_NOT_FOUND
			case codes.Unauthenticated:
				code = rpc.Code_CODE_UNAUTHENTICATED
			case codes.PermissionDenied:
				code = rpc.Code_CODE_PERMISSION_DENIED
			}
		}
	}
	const reason = "userid lookup failed"
	log := appctx.GetLogger(ctx)
	log.Warn().Str("code", code.String()).Msg(reason)
	return &rpc.Status{Code: code, Message: reason, Trace: trace.Get(ctx)}
}

func (s *service) FindUsers(ctx context.Context, req *userpb.FindUsersRequest) (*userpb.FindUsersResponse, error) {
	log := appctx.GetLogger(ctx)
	idx := slices.IndexFunc(req.Filters, func(f *userpb.Filter) bool { return f != nil && f.Type == userpb.Filter_TYPE_QUERY })
	if idx < 0 {
		return nil, errtypes.BadRequest("Must pass a filter with Filter_TYPE_QUERY to FindUsers")
	}
	query := req.Filters[idx].GetQuery()

	users, err := s.usermgr.FindUsers(ctx, query, req.Filters, req.SkipFetchingUserGroups)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("Failed to find users")
		res := &userpb.FindUsersResponse{
			Status: status.NewStatusFromErrType(ctx, "error finding users", err),
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
	// Public-link guests and OCM (federated) users are not local accounts, so no user backend knows them.
	if t := req.UserId.GetType(); t == userpb.UserType_USER_TYPE_GUEST || t == userpb.UserType_USER_TYPE_FEDERATED {
		return &userpb.GetUserGroupsResponse{Status: status.NewOK(ctx)}, nil
	}

	groups, err := s.usermgr.GetUserGroups(ctx, req.UserId)
	if err != nil {
		res := &userpb.GetUserGroupsResponse{
			Status: status.NewStatusFromErrType(ctx, "error getting user groups", err),
		}
		return res, nil
	}

	res := &userpb.GetUserGroupsResponse{
		Status: status.NewOK(ctx),
		Groups: groups,
	}
	return res, nil
}
