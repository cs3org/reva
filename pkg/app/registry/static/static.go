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
	Providers map[string]*registrypb.ProviderInfo `mapstructure:"providers"`
}

func (c *config) init() {
	if len(c.Providers) == 0 {
		c.Providers = map[string]*registrypb.ProviderInfo{
			sharedconf.GetGatewaySVC(""): &registrypb.ProviderInfo{
				Address:   sharedconf.GetGatewaySVC(""),
				MimeTypes: []string{"text/plain"},
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

type mimeTypeIndex struct {
	defaultApp string
	apps       []string
}

type reg struct {
	providers map[string]*registrypb.ProviderInfo
	mimetypes map[string]*mimeTypeIndex // map the mime type to the addresses of the corresponding providers
}

// New returns an implementation of the app.Registry interface.
func New(m map[string]interface{}) (app.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	newReg := reg{
		providers: c.Providers,
		mimetypes: make(map[string]*mimeTypeIndex),
	}

	for addr, p := range c.Providers {
		if p != nil {
			for _, m := range p.MimeTypes {
				_, ok := newReg.mimetypes[m]
				if ok {
					newReg.mimetypes[m].apps = append(newReg.mimetypes[m].apps, addr)
				} else {
					newReg.mimetypes[m] = &mimeTypeIndex{apps: []string{addr}}
				}
			}
		}
	}
	return &newReg, nil
}

func (b *reg) FindProviders(ctx context.Context, mimeType string) ([]*registrypb.ProviderInfo, error) {
	// find longest match
	var match string

	for prefix := range b.mimetypes {
		if strings.HasPrefix(mimeType, prefix) && len(prefix) > len(match) {
			match = prefix
		}
	}

	if match == "" {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}

	var providers = make([]*registrypb.ProviderInfo, 0, len(b.mimetypes[match].apps))
	for _, p := range b.mimetypes[match].apps {
		providers = append(providers, b.providers[p])
	}
	return providers, nil
}

func (b *reg) AddProvider(ctx context.Context, p *registrypb.ProviderInfo) error {
	b.providers[p.Address] = p

	for _, m := range p.MimeTypes {
		_, ok := b.mimetypes[m]
		if ok {
			b.mimetypes[m].apps = append(b.mimetypes[m].apps, p.Address)
		} else {
			b.mimetypes[m] = &mimeTypeIndex{apps: []string{p.Address}}
		}
	}
	return nil
}

func (b *reg) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	var providers = make([]*registrypb.ProviderInfo, 0, len(b.providers))
	for _, p := range b.providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (b *reg) SetDefaultProviderForMimeType(ctx context.Context, mimeType string, p *registrypb.ProviderInfo) error {
	_, ok := b.mimetypes[mimeType]
	if ok {
		b.mimetypes[mimeType].defaultApp = p.Address
		// Add to list of apps if not present
		var present bool
		for _, pr := range b.mimetypes[mimeType].apps {
			if pr == p.Address {
				present = true
				break
			}
		}
		if !present {
			b.mimetypes[mimeType].apps = append(b.mimetypes[mimeType].apps, p.Address)
		}
	} else {
		b.mimetypes[mimeType] = &mimeTypeIndex{apps: []string{p.Address}, defaultApp: p.Address}
	}
	return nil
}

func (b *reg) GetDefaultProviderForMimeType(ctx context.Context, mimeType string) (*registrypb.ProviderInfo, error) {
	_, ok := b.mimetypes[mimeType]
	if ok {
		p, ok := b.providers[b.mimetypes[mimeType].defaultApp]
		if ok {
			return p, nil
		}
	}
	return nil, errtypes.NotFound("default application provider not set for mime type " + mimeType)
}
