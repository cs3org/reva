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
	"regexp"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/registry/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("static", New)
}

var bracketRegex = regexp.MustCompile(`\[(.*?)\]`)

type rule struct {
	Mapping string            `mapstructure:"mapping"`
	Address string            `mapstructure:"address"`
	Aliases map[string]string `mapstructure:"aliases"`
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
			"/": rule{
				Address: sharedconf.GetGatewaySVC(""),
			},
			"00000000-0000-0000-0000-000000000000": rule{
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
	return &reg{c: c}, nil
}

type reg struct {
	c *config
}

func getProviderAddr(ctx context.Context, r rule) string {
	addr := r.Address
	if addr == "" {
		if u, ok := user.ContextGetUser(ctx); ok {
			layout := templates.WithUser(u, r.Mapping)
			for k, v := range r.Aliases {
				if match, _ := regexp.MatchString("^"+k, layout); match {
					addr = v
				}
			}
		}
	}
	return addr
}

func (b *reg) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	providers := []*registrypb.ProviderInfo{}
	for k, v := range b.c.Rules {
		if addr := getProviderAddr(ctx, v); addr != "" {
			combs := generateRegexCombinations(k)
			for _, c := range combs {
				providers = append(providers, &registrypb.ProviderInfo{
					ProviderPath: c,
					Address:      addr,
				})
			}
		}
	}
	return providers, nil
}

// returns the the root path of the first provider in the list.
func (b *reg) GetHome(ctx context.Context) (*registrypb.ProviderInfo, error) {
	// Assume that HomeProvider is not a regexp
	if r, ok := b.c.Rules[b.c.HomeProvider]; ok {
		if addr := getProviderAddr(ctx, r); addr != "" {
			return &registrypb.ProviderInfo{
				ProviderPath: b.c.HomeProvider,
				Address:      addr,
			}, nil
		}
	}
	return nil, errors.New("static: home not found")
}

func (b *reg) FindProviders(ctx context.Context, ref *provider.Reference) ([]*registrypb.ProviderInfo, error) {
	// find longest match
	var match *registrypb.ProviderInfo
	var shardedMatches []*registrypb.ProviderInfo

	// Try to find by path first as most storage operations will be done using the path.
	fn := path.Clean(ref.GetPath())
	if fn != "" {
		for prefix, rule := range b.c.Rules {
			addr := getProviderAddr(ctx, rule)
			r, err := regexp.Compile("^" + prefix)
			if err != nil {
				continue
			}
			if m := r.FindString(fn); m != "" {
				match = &registrypb.ProviderInfo{
					ProviderPath: m,
					Address:      addr,
				}
			}
			// Check if the current rule forms a part of a reference spread across storage providers.
			if strings.HasPrefix(prefix, fn) {
				combs := generateRegexCombinations(prefix)
				for _, c := range combs {
					shardedMatches = append(shardedMatches, &registrypb.ProviderInfo{
						ProviderPath: c,
						Address:      addr,
					})
				}
			}
		}
	}

	if match != nil && match.ProviderPath != "" {
		return []*registrypb.ProviderInfo{match}, nil
	} else if len(shardedMatches) > 0 {
		// If we don't find a perfect match but at least one provider is encapsulated
		// by the reference, return all such providers.
		return shardedMatches, nil
	}

	// Try with id
	id := ref.GetId()
	if id == nil {
		return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
	}

	for prefix, rule := range b.c.Rules {
		addr := getProviderAddr(ctx, rule)
		r, err := regexp.Compile("^" + prefix + "$")
		if err != nil {
			continue
		}
		// TODO(labkode): fill path info based on provider id, if path and storage id points to same id, take that.
		if m := r.FindString(id.StorageId); m != "" {
			return []*registrypb.ProviderInfo{&registrypb.ProviderInfo{
				ProviderId: id.StorageId,
				Address:    addr,
			}}, nil
		}
	}

	return nil, errtypes.NotFound("storage provider not found for ref " + ref.String())
}

func generateRegexCombinations(rex string) []string {
	m := bracketRegex.FindString(rex)
	r := strings.Trim(strings.Trim(m, "["), "]")
	if r == "" {
		return []string{rex}
	}
	var combinations []string
	for i := 0; i < len(r); i++ {
		if i < len(r)-2 && r[i+1] == '-' {
			for j := r[i]; j <= r[i+2]; j++ {
				p := strings.Replace(rex, m, string(j), 1)
				combinations = append(combinations, generateRegexCombinations(p)...)
			}
			i += 2
		} else {
			p := strings.Replace(rex, m, string(r[i]), 1)
			combinations = append(combinations, generateRegexCombinations(p)...)
		}
	}
	return combinations
}
