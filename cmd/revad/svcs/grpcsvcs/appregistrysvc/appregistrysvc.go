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

package appregistrysvc

import (
	"context"
	"fmt"
	"io"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/app/registry/static"
	"github.com/cernbox/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	grpcserver.Register("appregistrysvc", New)
}

type service struct {
	registry app.Registry
}

func (s *service) Close() error {
	return nil
}

type config struct {
	Driver string                 `mapstructure:"driver"`
	Static map[string]interface{} `mapstructure:"static"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	registry, err := getRegistry(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		registry: registry,
	}

	appregistryv0alphapb.RegisterAppRegistryServiceServer(ss, service)
	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getRegistry(c *config) (app.Registry, error) {
	switch c.Driver {
	case "static":
		return static.New(c.Static)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}
func (s *service) GetAppProvider(ctx context.Context, req *appregistryv0alphapb.GetAppProviderRequest) (*appregistryv0alphapb.GetAppProviderResponse, error) {
	log := appctx.GetLogger(ctx)
	mime := req.MimeType
	p, err := s.registry.FindProvider(ctx, mime)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc find provider request")
		res := &appregistryv0alphapb.GetAppProviderResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &appregistryv0alphapb.GetAppProviderResponse{
		Status:   &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Provider: provider,
	}
	return res, nil
}

func (s *service) ListAppProviders(ctx context.Context, req *appregistryv0alphapb.ListAppProvidersRequest) (*appregistryv0alphapb.ListAppProvidersResponse, error) {
	pvds, err := s.registry.ListProviders(ctx)
	if err != nil {
		res := &appregistryv0alphapb.ListAppProvidersResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}
	var providers []*appregistryv0alphapb.ProviderInfo
	for _, pvd := range pvds {
		providers = append(providers, format(pvd))
	}

	res := &appregistryv0alphapb.ListAppProvidersResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Providers: providers,
	}
	return res, nil
}

func format(p *app.ProviderInfo) *appregistryv0alphapb.ProviderInfo {
	return &appregistryv0alphapb.ProviderInfo{
		Address: p.Location,
	}
}
