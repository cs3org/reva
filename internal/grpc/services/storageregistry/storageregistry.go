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

package storageregistry

import (
	"context"
	"fmt"
	"io"

	storageregv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageregistry/v0alpha"
	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageregistry", New)
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
		return &storageregv0alphapb.ListStorageProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting list of storage providers"),
		}, nil
	}

	providers := make([]*storagetypespb.ProviderInfo, 0, len(pinfos))
	for _, info := range pinfos {
		fill(info)
		providers = append(providers, info)
	}

	res := &storageregv0alphapb.ListStorageProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: providers,
	}
	return res, nil
}

func (s *service) GetStorageProvider(ctx context.Context, req *storageregv0alphapb.GetStorageProviderRequest) (*storageregv0alphapb.GetStorageProviderResponse, error) {
	p, err := s.reg.FindProvider(ctx, req.Ref)
	if err != nil {
		return &storageregv0alphapb.GetStorageProviderResponse{
			Status: status.NewInternal(ctx, err, "error finding storage provider"),
		}, nil
	}

	fill(p)
	res := &storageregv0alphapb.GetStorageProviderResponse{
		Status:   status.NewOK(ctx),
		Provider: p,
	}
	return res, nil
}

func (s *service) GetHome(ctx context.Context, req *storageregv0alphapb.GetHomeRequest) (*storageregv0alphapb.GetHomeResponse, error) {
	log := appctx.GetLogger(ctx)
	p, err := s.reg.GetHome(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting home")
		res := &storageregv0alphapb.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error getting home"),
		}
		return res, nil
	}

	res := &storageregv0alphapb.GetHomeResponse{
		Status: status.NewOK(ctx),
		Path:   p,
	}
	return res, nil
}

// TODO(labkode): fix
func fill(p *storagetypespb.ProviderInfo) {}
