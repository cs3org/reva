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

package usershareprovider

import (
	"context"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("usershareprovider", New)
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

type service struct {
	conf *config
	sm   share.Manager
}

func getShareManager(c *config) (share.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
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

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new user share provider svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	sm, err := getShareManager(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		sm:   sm,
	}

	return service, nil
}

func (s *service) CreateShare(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.CreateShareResponse, error) {
	u := user.ContextMustGetUser(ctx)
	if req.Grant.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && req.Grant.Grantee.GetUserId().Idp == "" {
		// use logged in user Idp as default.
		g := &userpb.UserId{OpaqueId: req.Grant.Grantee.GetUserId().OpaqueId, Idp: u.Id.Idp}
		req.Grant.Grantee.Id = &provider.Grantee_UserId{UserId: g}
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
	_, err := s.sm.UpdateShare(ctx, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		return &collaboration.UpdateShareResponse{
			Status: status.NewInternal(ctx, err, "error updating share"),
		}, nil
	}

	res := &collaboration.UpdateShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *collaboration.ListReceivedSharesRequest) (*collaboration.ListReceivedSharesResponse, error) {
	shares, err := s.sm.ListReceivedShares(ctx) // TODO(labkode): check what to update
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
	_, err := s.sm.UpdateReceivedShare(ctx, req.Ref, req.Field) // TODO(labkode): check what to update
	if err != nil {
		return &collaboration.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	res := &collaboration.UpdateReceivedShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
