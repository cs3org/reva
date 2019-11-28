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

package appregistry

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"

	appregistryv1beta1pb "github.com/cs3org/go-cs3apis/cs3/appregistry/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/registry/static"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
)

func init() {
	rgrpc.Register("appregistry", New)
}

type svc struct {
	registry app.Registry
}

func (s *svc) Close() error {
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

	svc := &svc{
		registry: registry,
	}

	appregistryv1beta1pb.RegisterAppRegistryServiceServer(ss, svc)
	return svc, nil
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

func (s *svc) GetAppProviders(ctx context.Context, req *appregistryv1beta1pb.GetAppProvidersRequest) (*appregistryv1beta1pb.GetAppProvidersResponse, error) {
	p, err := s.registry.FindProvider(ctx, req.ResourceInfo.MimeType)
	if err != nil {
		return &appregistryv1beta1pb.GetAppProvidersResponse{
			Status: status.NewInternal(ctx, err, "error looking for the app provider"),
		}, nil
	}

	provider := format(p)
	res := &appregistryv1beta1pb.GetAppProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: []*appregistryv1beta1pb.ProviderInfo{provider},
	}
	return res, nil
}

func (s *svc) ListAppProviders(ctx context.Context, req *appregistryv1beta1pb.ListAppProvidersRequest) (*appregistryv1beta1pb.ListAppProvidersResponse, error) {
	pvds, err := s.registry.ListProviders(ctx)
	if err != nil {
		return &appregistryv1beta1pb.ListAppProvidersResponse{
			Status: status.NewInternal(ctx, err, "error listing the app providers"),
		}, nil
	}
	providers := make([]*appregistryv1beta1pb.ProviderInfo, 0, len(pvds))
	for _, pvd := range pvds {
		providers = append(providers, format(pvd))
	}

	res := &appregistryv1beta1pb.ListAppProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: providers,
	}
	return res, nil
}

func format(p *app.ProviderInfo) *appregistryv1beta1pb.ProviderInfo {
	return &appregistryv1beta1pb.ProviderInfo{
		Address: p.Location,
	}
}
