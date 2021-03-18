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

package static

import (
	"context"
	"path"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("static", New)
}

type config struct {
	Rules        map[string]string `mapstructure:"rules"`
	HomeProvider string            `mapstructure:"home_provider"`
}

func (c *config) init() {
	if c.HomeProvider == "" {
		c.HomeProvider = "/"
	}

	if len(c.Rules) == 0 {
		c.Rules = map[string]string{
			"/":                                    sharedconf.GetGatewaySVC(""),
			"00000000-0000-0000-0000-000000000000": sharedconf.GetGatewaySVC(""),
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

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	return &reg{c: c}, nil
}

type reg struct {
	c *config
}

func (b *reg) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	providers := []*registrypb.ProviderInfo{}
	for k, v := range b.c.Rules {
		providers = append(providers, &registrypb.ProviderInfo{
			ProviderPath: k,
			Address:      v,
		})
	}
	return providers, nil
}

// returns the the root path of the first provider in the list.
// TODO(labkode): this is not production ready.
func (b *reg) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	address, ok := b.c.Rules[b.c.HomeProvider]
	if ok {
		return &registrypb.ProviderInfo{
			ProviderPath: b.c.HomeProvider,
			Address:      address,
		}, nil
	}
	return nil, errors.New("static: home not found")
}

func (b *reg) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	// find longest match
	var match *registrypb.ProviderInfo
	var shardedMatches []*registrypb.ProviderInfo

	// we try to find first by path as most storage operations will be done on path.
	fn := path.Clean(ref.GetPath())
	if fn != "" {
		for prefix, addr := range b.c.Rules {
			if strings.HasPrefix(fn, prefix) && len(prefix) > len(match.ProviderPath) {
				match = &registrypb.ProviderInfo{
					ProviderPath: prefix,
					Address:      addr,
				}
			}
			// Check if the current rule forms a part of a reference spread across storage providers.
			if fn != "/" && strings.HasPrefix(prefix, fn) {
				shardedMatches = append(shardedMatches, &registrypb.ProviderInfo{
					ProviderPath: prefix,
					Address:      addr,
				})
			}
		}
	}

	if match.ProviderPath != "" {
		return []*registrypb.ProviderInfo{match}, nil
	} else if len(shardedMatches) > 0 {
		// If we don't find a perfect match but at least one provider is encapsulated
		// by the reference, return all such providers.
		return shardedMatches, nil
	}

	// we try with id
	id := ref.GetId()
	if id == nil {
		return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
	}
	if address, ok := b.c.Rules[id.StorageId]; ok {
		// TODO(labkode): fill path info based on provider id, if path and storage id points to same id, take that.
		return []*registrypb.ProviderInfo{&registrypb.ProviderInfo{
			ProviderId: id.StorageId,
			Address:    address,
		}}, nil
	}
	return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
}
