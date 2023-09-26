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

package usershareprovider

import (
	"context"
	"regexp"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("usershareprovider", New)
	plugin.RegisterNamespace("grpc.services.usershareprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver                string                            `mapstructure:"driver"`
	Drivers               map[string]map[string]interface{} `mapstructure:"drivers"`
	AllowedPathsForShares []string                          `mapstructure:"allowed_paths_for_shares"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

type service struct {
	conf                  *config
	sm                    share.Manager
	allowedPathsForShares []*regexp.Regexp
}

func getShareManager(ctx context.Context, c *config) (share.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// TODO(labkode): add ctx to Close.
func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	collaboration.RegisterCollaborationAPIServer(ss, s)
}

// New creates a new user share provider svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	sm, err := getShareManager(ctx, &c)
	if err != nil {
		return nil, err
	}

	allowedPathsForShares := make([]*regexp.Regexp, 0, len(c.AllowedPathsForShares))
	for _, s := range c.AllowedPathsForShares {
		regex, err := regexp.Compile(s)
		if err != nil {
			return nil, err
		}
		allowedPathsForShares = append(allowedPathsForShares, regex)
	}

	service := &service{
		conf:                  &c,
		sm:                    sm,
		allowedPathsForShares: allowedPathsForShares,
	}

	return service, nil
}

func (s *service) isPathAllowed(path string) bool {
	if len(s.allowedPathsForShares) == 0 {
		return true
	}
	for _, reg := range s.allowedPathsForShares {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

func (s *service) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.CreateShareResponse, error) {
	u := ctxpkg.ContextMustGetUser(ctx)
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && req.Grant.Grantee.GetUserId().Idp == "" {
		// use logged in user Idp as default.
		g := &userpb.UserId{OpaqueId: req.Grant.Grantee.GetUserId().OpaqueId, Idp: u.Id.Idp, Type: userpb.UserType_USER_TYPE_PRIMARY}
		req.Grant.Grantee.Id = &provider.Grantee_UserId{UserId: g}
	}

	if !s.isPathAllowed(req.ResourceInfo.Path) {
		return &collaboration.CreateShareResponse{
			Status: status.NewInvalidArg(ctx, "share creation is not allowed for the specified path"),
		}, nil
	}

	share, err := s.sm.Share(ctx, req.ResourceInfo, req.Grant)
	if err != nil {
		return &collaboration.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error creating share"),
		}, nil
	}

	res := &collaboration.CreateShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemoveShare(ctx context.Context, req *collaboration.RemoveShareRequest) (*collaboration.RemoveShareResponse, error) {
	err := s.sm.Unshare(ctx, req.Ref)
	if err != nil {
		return &collaboration.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error removing share"),
		}, nil
	}

	return &collaboration.RemoveShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetShare(ctx context.Context, req *collaboration.GetShareRequest) (*collaboration.GetShareResponse, error) {
	share, err := s.sm.GetShare(ctx, req.Ref)
	if err != nil {
		return &collaboration.GetShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share"),
		}, nil
	}

	return &collaboration.GetShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}, nil
}

func (s *service) ListShares(ctx context.Context, req *collaboration.ListSharesRequest) (*collaboration.ListSharesResponse, error) {
	shares, err := s.sm.ListShares(ctx, req.Filters) // TODO(labkode): add filter to share manager
	if err != nil {
		return &collaboration.ListSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing shares"),
		}, nil
	}

	res := &collaboration.ListSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateShare(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.UpdateShareResponse, error) {
	share, err := s.sm.UpdateShare(ctx, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		return &collaboration.UpdateShareResponse{
			Status: status.NewInternal(ctx, err, "error updating share"),
		}, nil
	}

	res := &collaboration.UpdateShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest) (*collaboration.ListReceivedSharesResponse, error) {
	// For the UI add a filter to not display the denial shares
	foundExclude := false
	for _, f := range req.Filters {
		if f.Type == collaboration.Filter_TYPE_EXCLUDE_DENIALS {
			foundExclude = true
			break
		}
	}
	if !foundExclude {
		req.Filters = append(req.Filters, &collaboration.Filter{Type: collaboration.Filter_TYPE_EXCLUDE_DENIALS})
	}
	shares, err := s.sm.ListReceivedShares(ctx, req.Filters) // TODO(labkode): check what to update
	if err != nil {
		return &collaboration.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing received shares"),
		}, nil
	}

	res := &collaboration.ListReceivedSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) GetReceivedShare(ctx context.Context, req *collaboration.GetReceivedShareRequest) (*collaboration.GetReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)

	share, err := s.sm.GetReceivedShare(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("error getting received share")
		return &collaboration.GetReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res := &collaboration.GetReceivedShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) UpdateReceivedShare(ctx context.Context, req *collaboration.UpdateReceivedShareRequest) (*collaboration.UpdateReceivedShareResponse, error) {
	share, err := s.sm.UpdateReceivedShare(ctx, req.Share, req.UpdateMask) // TODO(labkode): check what to update
	if err != nil {
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	res := &collaboration.UpdateReceivedShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}
