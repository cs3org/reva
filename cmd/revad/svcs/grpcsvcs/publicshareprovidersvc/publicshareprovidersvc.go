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
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/pool"
	"github.com/cs3org/reva/cmd/revad/svcs/grpcsvcs/status"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/pkg/user"
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

	publicshareproviderv0alphapb.RegisterPublicShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreatePublicShare(ctx context.Context, req *publicshareproviderv0alphapb.CreatePublicShareRequest) (*publicshareproviderv0alphapb.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msg("create public share")

	// TODO(refs) this is just for testing purposes until the update of the cs3apis: https://github.com/cs3org/cs3apis/pull/34
	endpoint := "localhost:9999"
	// TODO(refs) wait until [] is merged and get rid of this hardcoded url
	_, err := pool.GetStorageProviderServiceClient(endpoint)
	if err != nil {
		log.Debug().Err(errors.New("error getting a connection to a storage provider")).Msgf("address: %v", endpoint)
	}

	// TODO(refs) until https://github.com/cs3org/cs3apis/pull/34 is merged, we need a stat call to get the ResourceInfo
	// build stat request. We have the ID from the CreatePublicShareRequest
	statReq := storageproviderv0alphapb.StatRequest{
		Ref: &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Id{
				Id: req.GetResourceId(),
			},
		},
	}

	// get user from context
	u, _ := user.ContextGetUser(ctx) // TODO(refs) handle error

	// do stat
	spConn, err := pool.GetStorageProviderServiceClient(endpoint)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error connecting to storage provider")
	}

	statRes, err := spConn.Stat(ctx, &statReq)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error on stat call")
	}

	share, err := s.sm.CreatePublicShare(ctx, u, statRes.GetInfo(), req.Grant)
	if err != nil {
		log.Debug().Err(err).Str("createShare", "shares").Msg("error connecting to storage provider")
	}

	// TODO(refs) where does the 1970 come from?
	res := &publicshareproviderv0alphapb.CreatePublicShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
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
	user, _ := user.ContextGetUser(ctx)

	shares, err := s.sm.ListPublicShares(ctx, user, &storageproviderv0alphapb.ResourceInfo{})
	if err != nil {
		log.Err(err).Msg("error listing shares")
		return &publicshareproviderv0alphapb.ListPublicSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing public shares"),
		}, nil
	}

	res := &publicshareproviderv0alphapb.ListPublicSharesResponse{
		Status: status.NewOK(ctx),
		Share:  shares,
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
