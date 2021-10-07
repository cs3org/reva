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
	"sync"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/registry/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("static", New)
}

type mimeTypeConfig struct {
	Extension     string `mapstructure:"extension"`
	Name          string `mapstructure:"name"`
	Description   string `mapstructure:"description"`
	Icon          string `mapstructure:"icon"`
	DefaultApp    string `mapstructure:"default_app"`
	AllowCreation bool   `mapstructure:"allow_creation"`
}

type mimeTypeIndex struct {
	mimeConf mimeTypeConfig
	apps     []string
}

type config struct {
	Providers map[string]*registrypb.ProviderInfo `mapstructure:"providers"`
	MimeTypes map[string]mimeTypeConfig           `mapstructure:"mime_types"`
}

func (c *config) init() {
	if len(c.Providers) == 0 {
		c.Providers = map[string]*registrypb.ProviderInfo{}
	}
}

type manager struct {
	config       *config
	providers    map[string]*registrypb.ProviderInfo
	mimetypesIdx map[string]*mimeTypeIndex // map the mime type to the addresses of the corresponding providers
	sync.RWMutex
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an implementation of the app.Registry interface.
func New(m map[string]interface{}) (app.Registry, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	conf.init()

	mimetypes := make(map[string]*mimeTypeIndex)

	for addr, p := range conf.Providers {
		if p != nil {
			for _, m := range p.MimeTypes {
				_, ok := mimetypes[m]
				if ok {
					mimetypes[m].apps = append(mimetypes[m].apps, addr)
				} else {
					mimetypes[m] = &mimeTypeIndex{apps: []string{addr}}
					if mimeConf, ok := conf.MimeTypes[m]; ok {
						mimetypes[m].mimeConf = mimeConf
					}
				}
				// set this as default app for mime types if configured via name
				if mimeConf, ok := conf.MimeTypes[m]; ok && mimeConf.DefaultApp == p.Name {
					mimetypes[m].mimeConf.DefaultApp = addr
				}
			}
		}
	}

	return &manager{
		config:       conf,
		providers:    conf.Providers,
		mimetypesIdx: mimetypes,
	}, nil
}

func (regManager *manager) FindProviders(ctx context.Context, mimeType string) ([]*registrypb.ProviderInfo, error) {
	regManager.RLock()
	defer regManager.RUnlock()

	// find longest match
	var match string
	var apps []string

	for prefix, idx := range regManager.mimetypesIdx {
		if strings.HasPrefix(mimeType, prefix) && len(prefix) > len(match) {
			match = prefix
			apps = idx.apps
		}
	}

	if match == "" {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}

	providers := make([]*registrypb.ProviderInfo, 0, len(apps))
	for _, p := range apps {
		providers = append(providers, regManager.providers[p])
	}

	return providers, nil
}

func (regManager *manager) AddProvider(ctx context.Context, p *registrypb.ProviderInfo) error {
	regManager.Lock()
	defer regManager.Unlock()

	regManager.providers[p.Address] = p

	for _, m := range p.MimeTypes {
		if idx, ok := regManager.mimetypesIdx[m]; ok {
			idx.apps = append(idx.apps, p.Address)
		} else {
			regManager.mimetypesIdx[m] = &mimeTypeIndex{apps: []string{p.Address}}
			if mimetypeConfig, ok := regManager.config.MimeTypes[m]; ok {
				regManager.mimetypesIdx[m].mimeConf = mimetypeConfig
			}
		}

		// set this as default app for mime types if configured via name
		if mimetypeConfig, ok := regManager.config.MimeTypes[m]; ok && mimetypeConfig.DefaultApp == p.Name {
			regManager.mimetypesIdx[m].mimeConf.DefaultApp = p.Address
		}
	}

	return nil
}

func (regManager *manager) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	regManager.RLock()
	defer regManager.RUnlock()

	providers := make([]*registrypb.ProviderInfo, 0, len(regManager.providers))
	for _, p := range regManager.providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (regManager *manager) ListSupportedMimeTypes(ctx context.Context) ([]*registrypb.MimeTypeInfo, error) {
	regManager.RLock()
	defer regManager.RUnlock()

	res := []*registrypb.MimeTypeInfo{}

	for m, mime := range regManager.mimetypesIdx {
		info := &registrypb.MimeTypeInfo{
			MimeType:      m,
			Ext:           mime.mimeConf.Extension,
			Name:          mime.mimeConf.Name,
			Description:   mime.mimeConf.Description,
			Icon:          mime.mimeConf.Icon,
			AllowCreation: mime.mimeConf.AllowCreation,
		}
		for _, p := range mime.apps {
			if provider, ok := regManager.providers[p]; ok {
				t := *provider
				t.MimeTypes = nil
				info.AppProviders = append(info.AppProviders, &t)
			}
		}
		res = append(res, info)
	}

	return res, nil
}

func (regManager *manager) SetDefaultProviderForMimeType(ctx context.Context, mimeType string, p *registrypb.ProviderInfo) error {
	regManager.Lock()
	defer regManager.Unlock()

	idx, ok := regManager.mimetypesIdx[mimeType]
	if ok {
		idx.mimeConf.DefaultApp = p.Address

		// Add to list of apps if not present
		var present bool
		for _, pr := range idx.apps {
			if pr == p.Address {
				present = true
				break
			}
		}
		if !present {
			idx.apps = append(idx.apps, p.Address)
		}
	}

	return errtypes.NotFound("mime type not found " + mimeType)
}

func (regManager *manager) GetDefaultProviderForMimeType(ctx context.Context, mimeType string) (*registrypb.ProviderInfo, error) {
	regManager.RLock()
	defer regManager.RUnlock()

	m, ok := regManager.mimetypesIdx[mimeType]
	if ok {
		if p, ok := regManager.providers[m.mimeConf.DefaultApp]; ok {
			return p, nil
		}
	}

	return nil, errtypes.NotFound("default application provider not set for mime type " + mimeType)
}
