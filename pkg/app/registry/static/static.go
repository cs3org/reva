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

	"github.com/cernbox/reva/pkg/app"
	"github.com/cernbox/reva/pkg/log"

	"github.com/mitchellh/mapstructure"
)

var logger = log.New("static")

type registry struct {
	rules map[string]string
}

func (b *registry) ListProviders(ctx context.Context) ([]*app.ProviderInfo, error) {
	var providers []*app.ProviderInfo
	for _, address := range b.rules {
		providers = append(providers, &app.ProviderInfo{
			Location: address,
		})
	}
	return providers, nil
}

func (b *registry) FindProvider(ctx context.Context, mimeType string) (*app.ProviderInfo, error) {
	// find longest match
	var match string

	for prefix := range b.rules {
		if strings.HasPrefix(mimeType, prefix) && len(prefix) > len(match) {
			match = prefix
		}
	}

	if match == "" {
		return nil, notFoundError("application provider not found for mime type " + mimeType)
	}

	p := &app.ProviderInfo{
		Location: b.rules[match],
	}
	return p, nil
}

type config struct {
	Rules map[string]string
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
func New(m map[string]interface{}) (app.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &registry{rules: c.Rules}, nil
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func (e notFoundError) IsNotFound()   {}
