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

package usershareprovider

import (
	"context"
	"fmt"
	"io"

	usershareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
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

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new user share provider svc
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	sm, err := getShareManager(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		sm:   sm,
	}

	usershareproviderv1beta1pb.RegisterUserShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreateShare(ctx context.Context, req *usershareproviderv1beta1pb.CreateShareRequest) (*usershareproviderv1beta1pb.CreateShareResponse, error) {
	// TODO(labkode): validate input
	// TODO(labkode): hack: use configured IDP or use hostname as default.
	if req.Grant.Grantee.Id.Idp == "" {
		req.Grant.Grantee.Id.Idp = "localhost"
	}
	share, err := s.sm.Share(ctx, req.ResourceInfo, req.Grant)
	if err != nil {
		return &usershareproviderv1beta1pb.CreateShareResponse{
			Status: status.NewInternal(ctx, err, "error creating share"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.CreateShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemoveShare(ctx context.Context, req *usershareproviderv1beta1pb.RemoveShareRequest) (*usershareproviderv1beta1pb.RemoveShareResponse, error) {
	err := s.sm.Unshare(ctx, req.Ref)
	if err != nil {
		return &usershareproviderv1beta1pb.RemoveShareResponse{
			Status: status.NewInternal(ctx, err, "error removing share"),
		}, nil
	}

	return &usershareproviderv1beta1pb.RemoveShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetShare(ctx context.Context, req *usershareproviderv1beta1pb.GetShareRequest) (*usershareproviderv1beta1pb.GetShareResponse, error) {
	share, err := s.sm.GetShare(ctx, req.Ref)
	if err != nil {
		return &usershareproviderv1beta1pb.GetShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share"),
		}, nil
	}

	return &usershareproviderv1beta1pb.GetShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}, nil
}

func (s *service) ListShares(ctx context.Context, req *usershareproviderv1beta1pb.ListSharesRequest) (*usershareproviderv1beta1pb.ListSharesResponse, error) {
	shares, err := s.sm.ListShares(ctx, req.Filters) // TODO(labkode): add filter to share manager
	if err != nil {
		return &usershareproviderv1beta1pb.ListSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing shares"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.ListSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateShare(ctx context.Context, req *usershareproviderv1beta1pb.UpdateShareRequest) (*usershareproviderv1beta1pb.UpdateShareResponse, error) {
	_, err := s.sm.UpdateShare(ctx, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		return &usershareproviderv1beta1pb.UpdateShareResponse{
			Status: status.NewInternal(ctx, err, "error updating share"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.UpdateShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *usershareproviderv1beta1pb.ListReceivedSharesRequest) (*usershareproviderv1beta1pb.ListReceivedSharesResponse, error) {
	shares, err := s.sm.ListReceivedShares(ctx) // TODO(labkode): check what to update
	if err != nil {
		return &usershareproviderv1beta1pb.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing received shares"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.ListReceivedSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) GetReceivedShare(ctx context.Context, req *usershareproviderv1beta1pb.GetReceivedShareRequest) (*usershareproviderv1beta1pb.GetReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)

	_, err := s.sm.GetReceivedShare(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("error getting received share")
		return &usershareproviderv1beta1pb.GetReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.GetReceivedShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) UpdateReceivedShare(ctx context.Context, req *usershareproviderv1beta1pb.UpdateReceivedShareRequest) (*usershareproviderv1beta1pb.UpdateReceivedShareResponse, error) {
	_, err := s.sm.UpdateReceivedShare(ctx, req.Ref, req.Field) // TODO(labkode): check what to update
	if err != nil {
		return &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	res := &usershareproviderv1beta1pb.UpdateReceivedShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
