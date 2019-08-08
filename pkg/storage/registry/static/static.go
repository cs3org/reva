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

package static

import (
	"context"
	"strings"

	"github.com/cs3org/reva/pkg/storage/registry/registry"

	storagetypespb "github.com/cs3org/go-cs3apis/cs3/storagetypes"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

type reg struct {
	rules map[string]string
}

func (b *reg) ListProviders(ctx context.Context) ([]*storagetypespb.ProviderInfo, error) {
	providers := []*storagetypespb.ProviderInfo{}
	for k, v := range b.rules {
		providers = append(providers, &storagetypespb.ProviderInfo{
			Address:      v,
			ProviderPath: k,
		})
	}
	return providers, nil
}

func (b *reg) FindProvider(ctx context.Context, fn string) (*storagetypespb.ProviderInfo, error) {
	// find longest match
	var match string
	for prefix := range b.rules {
		if strings.HasPrefix(fn, prefix) && len(prefix) > len(match) {
			match = prefix
		}
	}

	if match == "" {
		return nil, errtypes.NotFound("storage provider not found for path " + fn)
	}

	p := &storagetypespb.ProviderInfo{
		ProviderPath: match,
		Address:      b.rules[match],
	}
	return p, nil
}

type config struct {
	Rules map[string]string `mapstructure:"rules"`
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
	return &reg{rules: c.Rules}, nil
}
