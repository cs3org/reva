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
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	"github.com/cs3org/reva/v2/pkg/sharedconf"
	"github.com/cs3org/reva/v2/pkg/storage"
	"github.com/cs3org/reva/v2/pkg/storage/registry/registry"
	"github.com/cs3org/reva/v2/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v2/pkg/utils/resourceid"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

var bracketRegex = regexp.MustCompile(`\[(.*?)\]`)

type alias struct {
	Address string `mapstructure:"address"`
	ID      string `mapstructure:"provider_id"`
}
type rule struct {
	Mapping           string           `mapstructure:"mapping"`
	Address           string           `mapstructure:"address"`
	ProviderID        string           `mapstructure:"provider_id"`
	ProviderPath      string           `mapstructure:"provider_path"`
	Aliases           map[string]alias `mapstructure:"aliases"`
	AllowedUserAgents []string         `mapstructure:"allowed_user_agents"`
	SpaceType         string           `mapstructure:"space_type"`
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
	return &reg{c: c}, nil
}

type reg struct {
	c *config
}

func getProviderAddr(ctx context.Context, r rule) (string, string) {
	addr := r.Address
	if addr == "" {
		if u, ok := ctxpkg.ContextGetUser(ctx); ok {
			layout := templates.WithUser(u, r.Mapping)
			for k, v := range r.Aliases {
				if match, _ := regexp.MatchString("^"+k, layout); match {
					return v.Address, v.ID
				}
			}
		}
	}
	return addr, r.ProviderID
}

func (b *reg) GetProvider(ctx context.Context, space *provider.StorageSpace) (*registrypb.ProviderInfo, error) {
	if space.SpaceType == "" {
		return nil, errors.New("need spacetype to figure out storage provider")
	}
	for _, rule := range b.c.Rules {
		if rule.SpaceType != space.SpaceType {
			continue
		}
		// TODO: Multiple providers per spacetype
		if addr, id := getProviderAddr(ctx, rule); addr != "" {
			return &registrypb.ProviderInfo{
				ProviderPath: b.c.HomeProvider,
				ProviderId:   id,
				Address:      addr,
			}, nil
		}
	}
	return nil, fmt.Errorf("no provider found for spacetype '%s'", space.SpaceType)
}

func (b *reg) ListProviders(ctx context.Context, filters map[string]string) ([]*registrypb.ProviderInfo, error) {
	if filters["storage_id"] == "" && filters["space_type"] == "" && filters["path"] == "" {
		return b.findAllProviders(ctx), nil
	}

	// find longest match
	var match *registrypb.ProviderInfo
	var shardedMatches []*registrypb.ProviderInfo
	// If the reference has a resource id set, use it to route
	typ := filters["space_type"]
	if s := filters["storage_id"]; s != "" {
		sid, spid := resourceid.StorageIDUnwrap(s)
		for prefix, rule := range b.c.Rules {
			addr, _ := getProviderAddr(ctx, rule)
			if typ != "" && typ == rule.SpaceType {
				return []*registrypb.ProviderInfo{{
					ProviderId:   rule.ProviderID,
					Address:      addr,
					ProviderPath: rule.ProviderPath,
				}}, nil
			}
			if spid != "" && spid == rule.ProviderID {
				return []*registrypb.ProviderInfo{{
					ProviderId:   spid,
					Address:      addr,
					ProviderPath: rule.ProviderPath,
				}}, nil

			}

			r, err := regexp.Compile("^" + prefix + "$")
			if err != nil {
				continue
			}

			// TODO(labkode): fill path info based on provider id, if path and storage id points to same id, take that.
			if m := r.FindString(sid); m != "" {
				return []*registrypb.ProviderInfo{{
					ProviderId:   prefix,
					Address:      addr,
					ProviderPath: rule.ProviderPath,
				}}, nil
			}
		}
		// TODO if the storage id is not set but node id is set we could poll all storage providers to check if the node is known there
		// for now, say the reference is invalid
		if filters["opaque_id"] != "" {
			return nil, errtypes.BadRequest(fmt.Sprintf("invalid filter %+v", filters))
		}
	}

	// Try to find by path  as most storage operations will be done using the path.
	// TODO this needs to be reevaluated once all clients query the storage registry for a list of storage providers
	fn := path.Clean(filters["path"])
	if fn != "" {
		for prefix, rule := range b.c.Rules {
			addr, id := getProviderAddr(ctx, rule)
			if typ != "" && typ == rule.SpaceType {
				return []*registrypb.ProviderInfo{{
					ProviderId:   rule.ProviderID,
					Address:      addr,
					ProviderPath: rule.ProviderPath,
				}}, nil
			}

			r, err := regexp.Compile("^" + prefix)
			if err != nil {
				continue
			}
			if m := r.FindString(fn); m != "" {
				if match != nil && len(match.ProviderPath) > len(m) {
					// Do not overwrite existing longer match
					continue
				}
				match = &registrypb.ProviderInfo{
					ProviderId:   id,
					ProviderPath: rule.ProviderPath,
					Address:      addr,
				}
				if match.ProviderPath == "" {
					match.ProviderPath = m
				}
			}
			// Check if the current rule forms a part of a reference spread across storage providers.
			if strings.HasPrefix(prefix, fn) {
				combs := generateRegexCombinations(prefix)
				for _, c := range combs {
					shardedMatches = append(shardedMatches, &registrypb.ProviderInfo{
						ProviderId:   id,
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

	return nil, errtypes.NotFound(fmt.Sprintf("storage provider not found for filters %+v", filters))

}

// findAllProviders returns a list of all storage providers
func (b *reg) findAllProviders(ctx context.Context) []*registrypb.ProviderInfo {
	unique := make(map[string]struct{})
	pis := make([]*registrypb.ProviderInfo, 0, len(b.c.Rules))
	for _, rule := range b.c.Rules {
		addr, _ := getProviderAddr(ctx, rule)
		if _, ok := unique[addr]; ok {
			continue
		}
		pis = append(pis, &registrypb.ProviderInfo{
			Address: addr,
		})
		unique[addr] = struct{}{}
	}
	return pis
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
