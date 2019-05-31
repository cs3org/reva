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

	"github.com/cs3org/reva/pkg/share/registry"

	"github.com/cs3org/reva/pkg/share"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

type shareRegistry struct {
	types map[string]string
}

func (r *shareRegistry) ListProviders(ctx context.Context) ([]*share.ProviderInfo, error) {
	providers := []*share.ProviderInfo{}
	for t, e := range r.types {
		providers = append(providers, &share.ProviderInfo{
			Type:     t,
			Endpoint: e,
		})
	}
	return providers, nil
}

func (r *shareRegistry) FindProvider(ctx context.Context, shareType string) (*share.ProviderInfo, error) {
	match := r.types[shareType]

	if match == "" {
		return nil, notFoundError("share provider not found for type " + shareType)
	}

	p := &share.ProviderInfo{
		Type:     shareType,
		Endpoint: match,
	}
	return p, nil
}

type config struct {
	Types map[string]string `mapstructure:"types"`
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
func New(m map[string]interface{}) (share.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &shareRegistry{types: c.Types}, nil
}

type notSupportedError string

func (e notSupportedError) Error() string   { return string(e) }
func (e notSupportedError) IsNotSupported() {}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func (e notFoundError) IsNotFound()   {}
