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

package ocmproviderauthorizer

import (
	"context"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/provider"
	"github.com/cs3org/reva/v3/pkg/ocm/provider/authorizer/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmproviderauthorizer", New)
	plugin.RegisterNamespace("grpc.services.ocmproviderauthorizer.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf *config
	pa   provider.Authorizer
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	ocmprovider.RegisterProviderAPIServer(ss, s)
}

func getProviderAuthorizer(ctx context.Context, c *config) (provider.Authorizer, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// New creates a new OCM provider authorizer svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	pa, err := getProviderAuthorizer(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: &c,
		pa:   pa,
	}
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{
		"/cs3.ocm.provider.v1beta1.ProviderAPI/IsProviderAllowed",
		"/cs3.ocm.provider.v1beta1.ProviderAPI/ListAllProviders",
	}
}

func (s *service) GetInfoByDomain(ctx context.Context, req *ocmprovider.GetInfoByDomainRequest) (*ocmprovider.GetInfoByDomainResponse, error) {
	domainInfo, err := s.pa.GetInfoByDomain(ctx, req.Domain)
	if err != nil {
		return &ocmprovider.GetInfoByDomainResponse{
			Status: status.NewInternal(ctx, err, "error getting provider info"),
		}, nil
	}

	return &ocmprovider.GetInfoByDomainResponse{
		Status:       status.NewOK(ctx),
		ProviderInfo: domainInfo,
	}, nil
}

func (s *service) IsProviderAllowed(ctx context.Context, req *ocmprovider.IsProviderAllowedRequest) (*ocmprovider.IsProviderAllowedResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Msgf("is provider '%s' allowed?", req.Provider.Domain)
	err := s.pa.IsProviderAllowed(ctx, req.Provider)
	if err != nil {
		return &ocmprovider.IsProviderAllowedResponse{
			Status: status.NewInternal(ctx, err, "error verifying mesh provider"),
		}, nil
	}

	return &ocmprovider.IsProviderAllowedResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) ListAllProviders(ctx context.Context, req *ocmprovider.ListAllProvidersRequest) (*ocmprovider.ListAllProvidersResponse, error) {
	providers, err := s.pa.ListAllProviders(ctx)
	if err != nil {
		return &ocmprovider.ListAllProvidersResponse{
			Status: status.NewInternal(ctx, err, "error retrieving mesh providers"),
		}, nil
	}

	return &ocmprovider.ListAllProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: providers,
	}, nil
}
