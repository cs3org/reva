// Copyright 2018-2023 CERN
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

package spacesregistry

import (
	"context"
	"encoding/base32"
	"errors"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/spaces/manager/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("spacesregistry", New)
	plugin.RegisterNamespace("grpc.services.spacesregistry.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                    `mapstructure:"driver"`
	Drivers map[string]map[string]any `mapstructure:"drivers"`
}

func (c *config) ApplyDefaults() {

}

type service struct {
	c      *config
	spaces spaces.Manager
}

func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	s, err := getSpacesDriver(ctx, c.Driver, c.Drivers)
	if err != nil {
		return nil, err
	}
	svc := service{
		c:      &c,
		spaces: s,
	}
	return &svc, nil
}

func getSpacesDriver(ctx context.Context, driver string, cfg map[string]map[string]any) (spaces.Manager, error) {
	if f, ok := registry.NewFuncs[driver]; ok {
		return f(ctx, cfg[driver])
	}
	return nil, errtypes.NotFound("driver not found: " + driver)
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	spaces, err := s.spaces.ListSpaces(ctx, user, req.Filters)
	if err != nil {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewInternal(ctx, err, "error listing storage spaces"),
		}, nil
	}

	for _, s := range spaces {
		s.Id = &provider.StorageSpaceId{
			OpaqueId: base32.StdEncoding.EncodeToString([]byte(s.RootInfo.Path)),
		}
	}
	return &provider.ListStorageSpacesResponse{
		Status:        status.NewOK(ctx),
		StorageSpaces: spaces,
	}, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterSpacesAPIServer(ss, s)
}

func (s *service) UnprotectedEndpoints() []string { return nil }

func (s *service) Close() error { return nil }
