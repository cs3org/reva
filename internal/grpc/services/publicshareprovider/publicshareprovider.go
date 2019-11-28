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

package publicshareprovider

import (
	"context"
	"fmt"
	"io"

	publicshareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v1beta1"
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
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

type service struct {
	conf *config
	sm   publicshare.Manager
}

func getShareManager(c *config) (publicshare.Manager, error) {
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

	publicshareproviderv1beta1pb.RegisterPublicShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreatePublicShare(ctx context.Context, req *publicshareproviderv1beta1pb.CreatePublicShareRequest) (*publicshareproviderv1beta1pb.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("create public share")

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	share, err := s.sm.CreatePublicShare(ctx, u, req.ResourceInfo, req.Grant)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error connecting to storage provider")
	}

	res := &publicshareproviderv1beta1pb.CreatePublicShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemovePublicShare(ctx context.Context, req *publicshareproviderv1beta1pb.RemovePublicShareRequest) (*publicshareproviderv1beta1pb.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv1beta1pb.RemovePublicShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetPublicShareByToken(ctx context.Context, req *publicshareproviderv1beta1pb.GetPublicShareByTokenRequest) (*publicshareproviderv1beta1pb.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("remove public share")

	return &publicshareproviderv1beta1pb.GetPublicShareByTokenResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetPublicShare(ctx context.Context, req *publicshareproviderv1beta1pb.GetPublicShareRequest) (*publicshareproviderv1beta1pb.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("get public share")

	return &publicshareproviderv1beta1pb.GetPublicShareResponse{
		Status: status.NewOK(ctx),
		// Share:  share,
	}, nil
}

func (s *service) ListPublicShares(ctx context.Context, req *publicshareproviderv1beta1pb.ListPublicSharesRequest) (*publicshareproviderv1beta1pb.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("list public share")
	user, _ := user.ContextGetUser(ctx)

	shares, err := s.sm.ListPublicShares(ctx, user, &storageproviderv1beta1pb.ResourceInfo{})
	if err != nil {
		log.Err(err).Msg("error listing shares")
		return &publicshareproviderv1beta1pb.ListPublicSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing public shares"),
		}, nil
	}

	res := &publicshareproviderv1beta1pb.ListPublicSharesResponse{
		Status: status.NewOK(ctx),
		Share:  shares,
	}
	return res, nil
}

func (s *service) UpdatePublicShare(ctx context.Context, req *publicshareproviderv1beta1pb.UpdatePublicShareRequest) (*publicshareproviderv1beta1pb.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("list public share")

	res := &publicshareproviderv1beta1pb.UpdatePublicShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}
