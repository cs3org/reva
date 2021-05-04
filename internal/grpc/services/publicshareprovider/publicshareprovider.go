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

package publicshareprovider

import (
	"context"

	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("publicshareprovider", New)
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
	sm   publicshare.Manager
}

func getShareManager(c *config) (publicshare.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// TODO(labkode): add ctx to Close.
func (s *service) Close() error {
	return nil
}
func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.sharing.link.v1beta1.LinkAPI/GetPublicShareByToken"}
}

func (s *service) Register(ss *grpc.Server) {
	link.RegisterLinkAPIServer(ss, s)
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

func (s *service) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest) (*link.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "create").Msg("create public share")

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	share, err := s.sm.CreatePublicShare(ctx, u, req.ResourceInfo, req.Grant)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error connecting to storage provider")
	}

	res := &link.CreatePublicShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest) (*link.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "remove").Msg("remove public share")

	user := user.ContextMustGetUser(ctx)
	err := s.sm.RevokePublicShare(ctx, user, req.Ref)
	if err != nil {
		return &link.RemovePublicShareResponse{
			Status: status.NewInternal(ctx, err, "error deleting public share"),
		}, err
	}
	return &link.RemovePublicShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetPublicShareByToken(ctx context.Context, req *link.GetPublicShareByTokenRequest) (*link.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Msg("getting public share by token")

	// there are 2 passes here, and the second request has no password
	found, err := s.sm.GetPublicShareByToken(ctx, req.GetToken(), req.GetAuthentication(), req.GetSign())
	switch v := err.(type) {
	case nil:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewOK(ctx),
			Share:  found,
		}, nil
	case errtypes.InvalidCredentials:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewPermissionDenied(ctx, v, "wrong password"),
		}, nil
	case errtypes.NotFound:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewNotFound(ctx, "unknown token"),
		}, nil
	default:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewInternal(ctx, v, "unexpected error"),
		}, nil
	}
}

func (s *service) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest) (*link.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "get").Msg("get public share")

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	found, err := s.sm.GetPublicShare(ctx, u, req.Ref, req.GetSign())
	if err != nil {
		return nil, err
	}

	return &link.GetPublicShareResponse{
		Status: status.NewOK(ctx),
		Share:  found,
	}, nil
}

func (s *service) ListPublicShares(ctx context.Context, req *link.ListPublicSharesRequest) (*link.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "list").Msg("list public share")
	user, _ := user.ContextGetUser(ctx)

	shares, err := s.sm.ListPublicShares(ctx, user, req.Filters, &provider.ResourceInfo{}, req.GetSign())
	if err != nil {
		log.Err(err).Msg("error listing shares")
		return &link.ListPublicSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing public shares"),
		}, nil
	}

	res := &link.ListPublicSharesResponse{
		Status: status.NewOK(ctx),
		Share:  shares,
	}
	return res, nil
}

func (s *service) UpdatePublicShare(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "update").Msg("update public share")

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	updateR, err := s.sm.UpdatePublicShare(ctx, u, req, nil)
	if err != nil {
		log.Err(err).Msgf("error updating public shares: %v", err)
	}

	res := &link.UpdatePublicShareResponse{
		Status: status.NewOK(ctx),
		Share:  updateR,
	}
	return res, nil
}
