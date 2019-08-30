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

package publicshareprovidersvc

import (
	"context"
	"fmt"
	"io"

	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
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
	grpcserver.Register("publicshareprovidersvc", New)
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

	publicshareproviderv0alphapb.RegisterPublicShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreatePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.CreatePublicShareRequest) (*publicshareproviderv0alphapb.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("create public share")

	res := &publicshareproviderv0alphapb.CreatePublicShareResponse{
		Status: status.NewOK(ctx),
		// Share:  share,
	}
	return res, nil
}

func (s *service) RemovePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.RemovePublicShareRequest) (*publicshareproviderv0alphapb.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv0alphapb.RemovePublicShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetPublicShareByToken(ctx context.Context, req *publicshareproviderv0alphapb.GetPublicShareByTokenRequest) (*publicshareproviderv0alphapb.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv0alphapb.GetPublicShareByTokenResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetPublicShare(ctx context.Context, req *publicshareproviderv0alphapb.GetPublicShareRequest) (*publicshareproviderv0alphapb.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share")

	return &publicshareproviderv0alphapb.GetPublicShareResponse{
		Status: status.NewOK(ctx),
		// Share:  share,
	}, nil
}

func (s *service) ListPublicShares(ctx context.Context, req *publicshareproviderv0alphapb.ListPublicSharesRequest) (*publicshareproviderv0alphapb.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("list public share")

	res := &publicshareproviderv0alphapb.ListPublicSharesResponse{
		Status: status.NewOK(ctx),
		// Shares: shares,
	}
	return res, nil
}

func (s *service) UpdatePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.UpdatePublicShareRequest) (*publicshareproviderv0alphapb.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("list public share")

	res := &publicshareproviderv0alphapb.UpdatePublicShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
