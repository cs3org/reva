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

package usershareprovidersvc

import (
	"context"
	"fmt"
	"io"

	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	grpcserver.Register("usershareprovidersvc", New)
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

	usershareproviderv0alphapb.RegisterUserShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreateShare(ctx context.Context, req *usershareproviderv0alphapb.CreateShareRequest) (*usershareproviderv0alphapb.CreateShareResponse, error) {
	log := appctx.GetLogger(ctx)

	// TODO(labkode): validate input
	// TODO(labkode): hack: use configured IDP or use hostname as default.
	if req.Grant.Grantee.Id.Idp == "" {
		req.Grant.Grantee.Id.Idp = "localhost"
	}
	share, err := s.sm.Share(ctx, req.ResourceInfo, req.Grant)
	if err != nil {
		log.Err(err).Msg("error creating share")
		return &usershareproviderv0alphapb.CreateShareResponse{
			Status: status.NewInternal(ctx, "error creating share"),
		}, nil
	}

	res := &usershareproviderv0alphapb.CreateShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemoveShare(ctx context.Context, req *usershareproviderv0alphapb.RemoveShareRequest) (*usershareproviderv0alphapb.RemoveShareResponse, error) {
	log := appctx.GetLogger(ctx)
	err := s.sm.Unshare(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("error removing share")
		return &usershareproviderv0alphapb.RemoveShareResponse{
			Status: status.NewInternal(ctx, "error removing share"),
		}, nil
	}

	return &usershareproviderv0alphapb.RemoveShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	log := appctx.GetLogger(ctx)
	share, err := s.sm.GetShare(ctx, req.Ref)
	if err != nil {
		log.Err(err).Msg("error getting share")
		return &usershareproviderv0alphapb.GetShareResponse{
			Status: status.NewInternal(ctx, "error getting share"),
		}, nil
	}

	return &usershareproviderv0alphapb.GetShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}, nil
}

func (s *service) ListShares(ctx context.Context, req *usershareproviderv0alphapb.ListSharesRequest) (*usershareproviderv0alphapb.ListSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	shares, err := s.sm.ListShares(ctx, req.Filters) // TODO(labkode): add filter to share manager
	if err != nil {
		log.Err(err).Msg("error listing shares")
		return &usershareproviderv0alphapb.ListSharesResponse{
			Status: status.NewInternal(ctx, "error listing shares"),
		}, nil
	}

	res := &usershareproviderv0alphapb.ListSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateShareRequest) (*usershareproviderv0alphapb.UpdateShareResponse, error) {
	log := appctx.GetLogger(ctx)

	_, err := s.sm.UpdateShare(ctx, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		log.Err(err).Msg("error updating share")
		return &usershareproviderv0alphapb.UpdateShareResponse{
			Status: status.NewInternal(ctx, "error updating share"),
		}, nil
	}

	res := &usershareproviderv0alphapb.UpdateShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *usershareproviderv0alphapb.ListReceivedSharesRequest) (*usershareproviderv0alphapb.ListReceivedSharesResponse, error) {
	log := appctx.GetLogger(ctx)

	shares, err := s.sm.ListReceivedShares(ctx) // TODO(labkode): check what to update
	if err != nil {
		log.Err(err).Msg("error listing received shares")
		return &usershareproviderv0alphapb.ListReceivedSharesResponse{
			Status: status.NewInternal(ctx, "error listing received shares"),
		}, nil
	}

	res := &usershareproviderv0alphapb.ListReceivedSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateReceivedShareRequest) (*usershareproviderv0alphapb.UpdateReceivedShareResponse, error) {
	log := appctx.GetLogger(ctx)

	_, err := s.sm.UpdateReceivedShare(ctx, req.Ref, req.Field) // TODO(labkode): check what to update
	if err != nil {
		log.Err(err).Msg("error updating received share")
		return &usershareproviderv0alphapb.UpdateReceivedShareResponse{
			Status: status.NewInternal(ctx, "error updating received share"),
		}, nil
	}

	res := &usershareproviderv0alphapb.UpdateReceivedShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
