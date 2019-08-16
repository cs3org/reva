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

package storageregsvc

import (
	"context"
	"fmt"
	"io"

	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	storageregv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
)

func init() {
	grpcserver.Register("storageregistrysvc", New)
}

type service struct {
	reg storage.Registry
}

func (s *service) Close() error {
	return nil
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

// New creates a new StorageBrokerService
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	reg, err := getRegistry(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		reg: reg,
	}

	storageregv0alphapb.RegisterStorageRegistryServiceServer(ss, service)
	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getRegistry(c *config) (storage.Registry, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) ListStorageProviders(ctx context.Context, req *storageregv0alphapb.ListStorageProvidersRequest) (*storageregv0alphapb.ListStorageProvidersResponse, error) {
	pinfos, err := s.reg.ListProviders(ctx)
	if err != nil {
		res := &storageregv0alphapb.ListStorageProvidersResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	providers := make([]*storagetypespb.ProviderInfo, 0, len(pinfos))
	for _, info := range pinfos {
		fill(info)
		providers = append(providers, info)
	}

	res := &storageregv0alphapb.ListStorageProvidersResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Providers: providers,
	}
	return res, nil
}

func (s *service) GetStorageProvider(ctx context.Context, req *storageregv0alphapb.GetStorageProviderRequest) (*storageregv0alphapb.GetStorageProviderResponse, error) {
	log := appctx.GetLogger(ctx)
	p, err := s.reg.FindProvider(ctx, req.Ref)
	if err != nil {
		log.Error().Err(err).Msg("error finding storage provider")
		res := &storageregv0alphapb.GetStorageProviderResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	fill(p)
	res := &storageregv0alphapb.GetStorageProviderResponse{
		Status:   &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Provider: p,
	}
	return res, nil
}

// TODO(labkode): fix
func fill(p *storagetypespb.ProviderInfo) {}
