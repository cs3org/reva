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

package authregistry

import (
	"context"

	registrypb "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/registry/registry"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("authregistry", New)
	plugin.RegisterNamespace("grpc.services.authregistry.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type service struct {
	reg auth.Registry
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{
		"/cs3.auth.registry.v1beta1.RegistryAPI/GetAuthProviders",
		"/cs3.auth.registry.v1beta1.RegistryAPI/ListAuthProviders",
	}
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

// New creates a new AuthRegistry.
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

func getRegistry(ctx context.Context, c *config) (auth.Registry, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("authregistrysvc: driver not found: " + c.Driver)
}

func (s *service) ListAuthProviders(ctx context.Context, req *registrypb.ListAuthProvidersRequest) (*registrypb.ListAuthProvidersResponse, error) {
	pinfos, err := s.reg.ListProviders(ctx)
	if err != nil {
		return &registrypb.ListAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting list of auth providers"),
		}, nil
	}

	res := &registrypb.ListAuthProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: pinfos,
	}
	return res, nil
}

func (s *service) GetAuthProviders(ctx context.Context, req *registrypb.GetAuthProvidersRequest) (*registrypb.GetAuthProvidersResponse, error) {
	pinfo, err := s.reg.GetProvider(ctx, req.Type)
	if err != nil {
		return &registrypb.GetAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider for type: "+req.Type),
		}, nil
	}

	res := &registrypb.GetAuthProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: []*registrypb.ProviderInfo{pinfo},
	}
	return res, nil
}
