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
	"strings"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/registry/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

type config struct {
	Rules map[string]string `mapstructure:"rules"`
}

func (c *config) init() {
	if len(c.Rules) == 0 {
		c.Rules = map[string]string{
			"text/plain": sharedconf.GetGatewaySVC(""),
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

type reg struct {
	rules map[string]string
}

func New(m map[string]interface{}) (app.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()
	return &reg{rules: c.Rules}, nil
}

func (b *reg) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	var providers = make([]*registrypb.ProviderInfo, 0, len(b.rules))
	for _, address := range b.rules {
		providers = append(providers, &registrypb.ProviderInfo{
			Address: address,
		})
	}
	return providers, nil
}

func (b *reg) FindProvider(ctx context.Context, mimeType string) (*registrypb.ProviderInfo, error) {
	// find the longest match
	var match string

	for prefix := range b.rules {
		if strings.HasPrefix(mimeType, prefix) && len(prefix) > len(match) {
			match = prefix
		}
	}

	if match == "" {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}

	p := &registrypb.ProviderInfo{
		Address: b.rules[match],
	}
	return p, nil
}
