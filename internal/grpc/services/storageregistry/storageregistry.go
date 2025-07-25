// Copyright 2018-2024 CERN
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

	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/registry/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("storageregistry", New)
	plugin.RegisterNamespace("grpc.services.storageregistry.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type service struct {
	reg storage.Registry
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	registrypb.RegisterRegistryAPIServer(ss, s)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "static"
	}
}

// New creates a new StorageBrokerService.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	reg, err := getRegistry(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		reg: reg,
	}

	return service, nil
}

func getRegistry(ctx context.Context, c *config) (storage.Registry, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func (s *service) ListStorageProviders(ctx context.Context, req *registrypb.ListStorageProvidersRequest) (*registrypb.ListStorageProvidersResponse, error) {
	pinfos, err := s.reg.ListProviders(ctx)
	if err != nil {
		return &registrypb.ListStorageProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting list of storage providers"),
		}, nil
	}

	res := &registrypb.ListStorageProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: pinfos,
	}
	return res, nil
}

func (s *service) GetStorageProviders(ctx context.Context, req *registrypb.GetStorageProvidersRequest) (*registrypb.GetStorageProvidersResponse, error) {
	p, err := s.reg.FindProviders(ctx, req.Ref)
	if err != nil {
		switch err.(type) {
		case errtypes.IsNotFound:
			return &registrypb.GetStorageProvidersResponse{
				Status: status.NewNotFound(ctx, err.Error()),
			}, nil
		default:
			return &registrypb.GetStorageProvidersResponse{
				Status: status.NewInternal(ctx, err, "error finding storage provider"),
			}, nil
		}
	}

	res := &registrypb.GetStorageProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: p,
	}
	return res, nil
}

func (s *service) GetHome(ctx context.Context, req *registrypb.GetHomeRequest) (*registrypb.GetHomeResponse, error) {
	log := appctx.GetLogger(ctx)
	p, err := s.reg.GetHome(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting home")
		res := &registrypb.GetHomeResponse{
			Status: status.NewInternal(ctx, err, "error getting home"),
		}
		return res, nil
	}

	res := &registrypb.GetHomeResponse{
		Status:   status.NewOK(ctx),
		Provider: p,
	}
	return res, nil
}
