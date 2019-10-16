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

	authregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/authregistry/v0alpha"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/registry/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("authregistry", New)
}

type service struct {
	reg auth.Registry
}

func (s *service) Close() error {
	return nil
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

// New creates a new AuthRegistry
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

	authregistryv0alphapb.RegisterAuthRegistryServiceServer(ss, service)
	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getRegistry(c *config) (auth.Registry, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("authregistrysvc: driver not found: %s", c.Driver)
}

func (s *service) ListAuthProviders(ctx context.Context, req *authregistryv0alphapb.ListAuthProvidersRequest) (*authregistryv0alphapb.ListAuthProvidersResponse, error) {
	pinfos, err := s.reg.ListProviders(ctx)
	if err != nil {
		return &authregistryv0alphapb.ListAuthProvidersResponse{
			Status: status.NewInternal(ctx, err, "error getting list of auth providers"),
		}, nil
	}

	res := &authregistryv0alphapb.ListAuthProvidersResponse{
		Status:    status.NewOK(ctx),
		Providers: pinfos,
	}
	return res, nil
}

func (s *service) GetAuthProvider(ctx context.Context, req *authregistryv0alphapb.GetAuthProviderRequest) (*authregistryv0alphapb.GetAuthProviderResponse, error) {
	pinfo, err := s.reg.GetProvider(ctx, req.Type)
	if err != nil {
		return &authregistryv0alphapb.GetAuthProviderResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider for type: "+req.Type),
		}, nil
	}

	res := &authregistryv0alphapb.GetAuthProviderResponse{
		Status:   status.NewOK(ctx),
		Provider: pinfo,
	}
	return res, nil
}
