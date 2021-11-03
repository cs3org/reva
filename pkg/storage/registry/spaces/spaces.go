// Copyright 2018-2021 CERN
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

package spaces

import (
	"context"
	"path/filepath"
	"regexp"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	rstatus "github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	pkgregistry "github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	pkgregistry.Register("spaces", New)
}

var bracketRegex = regexp.MustCompile(`\[(.*?)\]`)

type rule struct {
	Mapping           string            `mapstructure:"mapping"`
	Address           string            `mapstructure:"address"`
	Aliases           map[string]string `mapstructure:"aliases"`
	AllowedUserAgents []string          `mapstructure:"allowed_user_agents"`
}

type config struct {
	Rules        map[string]rule `mapstructure:"rules"`
	HomeProvider string          `mapstructure:"home_provider"`
}

func (c *config) init() {
	if c.HomeProvider == "" {
		c.HomeProvider = "/"
	}

	if len(c.Rules) == 0 {
		c.Rules = map[string]rule{
			"/": {
				Address: sharedconf.GetGatewaySVC(""),
			},
			"00000000-0000-0000-0000-000000000000": {
				Address: sharedconf.GetGatewaySVC(""),
			},
		}
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation of the storage.Registry interface that
// redirects requests to corresponding storage drivers.
func New(m map[string]interface{}) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	return &registry{c: c}, nil
}

type registry struct {
	c *config
}

func (r *registry) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	// after init we have a list of storage provider addresses
	// 1. lazily fetch all storage spaces by directly calling the provider
	providers := []*registrypb.ProviderInfo{}
	for _, rule := range r.c.Rules {
		c, err := pool.GetStorageProviderServiceClient(rule.Address)
		if err != nil {
			return nil, err
		}
		lSSRes, err := c.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{})
		if err != nil {
			return nil, err
		}
		if lSSRes.Status.Code != rpc.Code_CODE_OK {
			return nil, rstatus.NewErrorFromCode(lSSRes.Status.Code, "spaces registry")
		}
		for _, space := range lSSRes.StorageSpaces {
			providers = append(providers, &registrypb.ProviderInfo{
				ProviderPath: filepath.Join(space.SpaceType, space.Name),
				Address:      rule.Address,
			})
		}
	}
	return providers, nil
}

// returns the the root path of the first provider in the list.
func (r *registry) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

func (r *registry) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}
