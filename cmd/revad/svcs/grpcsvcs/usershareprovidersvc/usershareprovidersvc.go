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
	"fmt"
	"io"

	"github.com/cs3org/reva/cmd/revad/grpcserver"

	"context"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
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
	conf    *config
	storage storage.FS
}

func (s *service) Close() error {
	return s.storage.Shutdown()
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

	fs, err := getFS(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		storage: fs,
	}

	usershareproviderv0alphapb.RegisterUserShareProviderServiceServer(ss, service)
	return service, nil
}

func (s *service) CreateShare(ctx context.Context, req *usershareproviderv0alphapb.CreateShareRequest) (*usershareproviderv0alphapb.CreateShareResponse, error) {
	res := &usershareproviderv0alphapb.CreateShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) RemoveShare(ctx context.Context, req *usershareproviderv0alphapb.RemoveShareRequest) (*usershareproviderv0alphapb.RemoveShareResponse, error) {
	res := &usershareproviderv0alphapb.RemoveShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) GetShare(ctx context.Context, req *usershareproviderv0alphapb.GetShareRequest) (*usershareproviderv0alphapb.GetShareResponse, error) {
	res := &usershareproviderv0alphapb.GetShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) ListShares(ctx context.Context, req *usershareproviderv0alphapb.ListSharesRequest) (*usershareproviderv0alphapb.ListSharesResponse, error) {
	log := appctx.GetLogger(ctx)

	shares := []*usershareproviderv0alphapb.Share{}

	for _, filter := range req.Filters {
		if filter.Type == usershareproviderv0alphapb.ListSharesRequest_Filter_LIST_SHARES_REQUEST_FILTER_TYPE_RESOURCE_ID {
			path := filter.GetResourceId().OpaqueId
			log.Debug().Str("path", path).Msg("list shares")
			grants, err := s.storage.ListGrants(ctx, path)
			if err != nil {
				return nil, err
			}
			for _, grant := range grants {
				shares = append(shares, grantToShare(grant))
			}
		}
	}
	res := &usershareproviderv0alphapb.ListSharesResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_OK,
		},
		Shares: shares,
	}
	return res, nil
}

func grantToShare(grant *storage.Grant) *usershareproviderv0alphapb.Share {
	share := &usershareproviderv0alphapb.Share{
		// TODO map grant
	}
	return share
}

func (s *service) UpdateShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateShareRequest) (*usershareproviderv0alphapb.UpdateShareResponse, error) {
	res := &usershareproviderv0alphapb.UpdateShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) ListReceivedShares(ctx context.Context, req *usershareproviderv0alphapb.ListReceivedSharesRequest) (*usershareproviderv0alphapb.ListReceivedSharesResponse, error) {
	res := &usershareproviderv0alphapb.ListReceivedSharesResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func (s *service) UpdateReceivedShare(ctx context.Context, req *usershareproviderv0alphapb.UpdateReceivedShareRequest) (*usershareproviderv0alphapb.UpdateReceivedShareResponse, error) {
	res := &usershareproviderv0alphapb.UpdateReceivedShareResponse{
		Status: &rpcpb.Status{
			Code: rpcpb.Code_CODE_UNIMPLEMENTED,
		},
	}
	return res, nil
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}
