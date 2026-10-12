// Copyright 2018-2026 CERN
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

// Package appregistry serves the CS3 app registry on top of the service
// registry.
//
// App providers do not register themselves with the app registry: they are
// ordinary reva services, so the runtime already advertises them in the service
// registry with a reachable address and a liveness state, and they describe the
// app they serve in their node metadata. This registry only reads that, which
// means an app disappears when its providers die and comes back when they
// return, with no registration handshake to get lost across a restart.
//
// What it does own is the mime type catalogue: how a mime type is presented to
// the user, and which app opens it by default. That is deployment policy rather
// than something an app provider can know about itself, so it stays in the
// configuration.
package appregistry

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/service"
	orderedmap "github.com/wk8/go-ordered-map"
	"google.golang.org/protobuf/encoding/protojson"
)

const defaultPriority = 0

type mimeTypeConfig struct {
	MimeType      string `mapstructure:"mime_type"`
	Extension     string `mapstructure:"extension"`
	Name          string `mapstructure:"name"`
	Description   string `mapstructure:"description"`
	Icon          string `mapstructure:"icon"`
	DefaultApp    string `mapstructure:"default_app"`
	AllowCreation bool   `mapstructure:"allow_creation"`
}

type manager struct {
	// resolve returns the service registry to discover app providers in. It is
	// called per lookup rather than captured once, so that a registry installed
	// after this registry was built is still picked up.
	resolve  func() svcregistry.Registry
	selector service.Selector

	// mimetypes maps a mime type to its catalogue entry. Ordered so that
	// ListSupportedMimeTypes keeps the order the administrator configured.
	mu        sync.RWMutex
	mimetypes *orderedmap.OrderedMap // string -> *mimeTypeConfig
}

// newRegistry returns an app registry that discovers its providers in the
// service registry, presenting them through the configured mime type
// catalogue.
func newRegistry(c *config) *manager {
	mimetypes := orderedmap.New()
	for _, mime := range c.MimeTypes {
		if mime == nil {
			continue
		}
		mimetypes.Set(mime.MimeType, mime)
	}

	return &manager{
		resolve:   service.GlobalRegistry,
		selector:  &service.RoundRobinSelector{},
		mimetypes: mimetypes,
	}
}

// provider is an app provider discovered in the service registry.
type provider struct {
	info     *registrypb.ProviderInfo
	priority uint64
}

// providers returns the app providers currently advertised in the service
// registry, one per app: an app served by several nodes is reachable through
// any of them, so a live one is picked. They come back ordered by descending
// priority, and by name within a priority so that the order is stable.
func (m *manager) providers(ctx context.Context) ([]*provider, error) {
	discovered, err := m.discover(ctx)
	if err != nil {
		return nil, err
	}

	providers := make([]*provider, 0, len(discovered))
	for _, info := range discovered {
		providers = append(providers, &provider{info: info, priority: priorityOf(info)})
	}

	sort.Slice(providers, func(i, j int) bool {
		if providers[i].priority != providers[j].priority {
			return providers[i].priority > providers[j].priority
		}
		return providers[i].info.Name < providers[j].info.Name
	})
	return providers, nil
}

// discover returns the app providers advertised in the service registry, keyed
// by app. An app served by several nodes is reachable through any of them, so
// a live one is picked and its address handed out.
func (m *manager) discover(ctx context.Context) (map[string]*registrypb.ProviderInfo, error) {
	reg := m.resolve()
	if reg == nil {
		return nil, errtypes.NotFound("appregistry: no service registry configured")
	}

	svc, err := reg.GetService(service.NameAppProvider)
	if err != nil {
		// No app provider has registered (yet); that is not an error here, the
		// callers report it as "no provider for this mime type".
		return nil, nil
	}

	// Collect the nodes per app first, and only then pick the one to hand out.
	nodesByApp := map[string][]svcregistry.Node{}
	infoByApp := map[string]*registrypb.ProviderInfo{}
	for _, node := range svc.Nodes() {
		meta := node.Metadata()
		if meta[svcregistry.MetaTransport] != svcregistry.TransportGRPC {
			continue
		}
		encoded, ok := meta[svcregistry.MetaApp]
		if !ok || encoded == "" {
			continue
		}
		var info registrypb.ProviderInfo
		if err := protojson.Unmarshal([]byte(encoded), &info); err != nil {
			appctx.GetLogger(ctx).Error().Err(err).Str("node", node.ID()).
				Msg("appregistry: discarding an app provider advertising a malformed app description")
			continue
		}
		if info.Name == "" {
			continue
		}
		nodesByApp[info.Name] = append(nodesByApp[info.Name], node)
		infoByApp[info.Name] = &info
	}

	out := make(map[string]*registrypb.ProviderInfo, len(nodesByApp))
	for name, nodes := range nodesByApp {
		node, ok := m.selector.Pick(nodes)
		if !ok {
			// Every node serving this app is offline or draining.
			continue
		}
		info := infoByApp[name]
		info.Address = node.Address()
		out[name] = info
	}
	return out, nil
}

// priorityOf reads the priority an app provider advertises, defaulting to the
// lowest when it advertises none.
func priorityOf(p *registrypb.ProviderInfo) uint64 {
	if p.Opaque == nil || len(p.Opaque.Map) == 0 {
		return defaultPriority
	}
	entry, ok := p.Opaque.Map["priority"]
	if !ok {
		return defaultPriority
	}
	priority, err := strconv.ParseUint(string(entry.GetValue()), 10, 64)
	if err != nil {
		return defaultPriority
	}
	return priority
}

// FindProviders returns the providers that can open the given mime type, the
// preferred one first.
func (m *manager) FindProviders(ctx context.Context, mimeType string) ([]*registrypb.ProviderInfo, error) {
	providers, err := m.providers(ctx)
	if err != nil {
		return nil, err
	}

	match := m.longestMatch(mimeType, providers)
	if match == "" {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}

	res := make([]*registrypb.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		if handles(p.info, match) {
			res = append(res, p.info)
		}
	}
	if len(res) == 0 {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}
	return res, nil
}

// longestMatch returns the longest known mime type the given one starts with,
// so that a catalogue entry can stand for a whole family of mime types. Both
// the configured mime types and those the providers advertise are candidates.
func (m *manager) longestMatch(mimeType string, providers []*provider) string {
	var match string
	consider := func(candidate string) {
		if strings.HasPrefix(mimeType, candidate) && len(candidate) > len(match) {
			match = candidate
		}
	}

	m.mu.RLock()
	for pair := m.mimetypes.Oldest(); pair != nil; pair = pair.Next() {
		consider(pair.Key.(string))
	}
	m.mu.RUnlock()

	for _, p := range providers {
		for _, mime := range p.info.MimeTypes {
			consider(mime)
		}
	}
	return match
}

func handles(p *registrypb.ProviderInfo, mimeType string) bool {
	return slices.Contains(p.MimeTypes, mimeType)
}

// ListProviders returns every app provider currently advertised.
func (m *manager) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	providers, err := m.providers(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]*registrypb.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		res = append(res, p.info)
	}
	return res, nil
}

// ListSupportedMimeTypes returns the mime type catalogue, each entry carrying
// the apps that can currently open it. Configured mime types come first, in
// configuration order, followed by any further mime type a provider handles.
func (m *manager) ListSupportedMimeTypes(ctx context.Context) ([]*registrypb.MimeTypeInfo, error) {
	providers, err := m.providers(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]*registrypb.MimeTypeInfo, 0, m.mimetypes.Len())
	configured := make(map[string]bool, m.mimetypes.Len())
	for pair := m.mimetypes.Oldest(); pair != nil; pair = pair.Next() {
		mime := pair.Value.(*mimeTypeConfig)
		configured[mime.MimeType] = true
		res = append(res, &registrypb.MimeTypeInfo{
			MimeType:           mime.MimeType,
			Ext:                mime.Extension,
			Name:               mime.Name,
			Description:        mime.Description,
			Icon:               mime.Icon,
			AppProviders:       appsFor(providers, mime.MimeType),
			AllowCreation:      mime.AllowCreation,
			DefaultApplication: mime.DefaultApp,
		})
	}

	// A provider may handle a mime type the catalogue says nothing about. It
	// has no presentation metadata, but the app is still usable, so report it.
	for _, extra := range uncatalogued(providers, configured) {
		res = append(res, &registrypb.MimeTypeInfo{
			MimeType:     extra,
			AppProviders: appsFor(providers, extra),
		})
	}
	return res, nil
}

func appsFor(providers []*provider, mimeType string) []*registrypb.ProviderInfo {
	apps := make([]*registrypb.ProviderInfo, 0, len(providers))
	for _, p := range providers {
		if handles(p.info, mimeType) {
			apps = append(apps, p.info)
		}
	}
	return apps
}

// uncatalogued returns the mime types handled by some provider that the
// catalogue does not describe, in a stable order.
func uncatalogued(providers []*provider, configured map[string]bool) []string {
	seen := map[string]bool{}
	var extra []string
	for _, p := range providers {
		for _, mime := range p.info.MimeTypes {
			if configured[mime] || seen[mime] {
				continue
			}
			seen[mime] = true
			extra = append(extra, mime)
		}
	}
	sort.Strings(extra)
	return extra
}

// GetDefaultProviderForMimeType returns the app configured to open the given
// mime type by default, provided it is currently running.
func (m *manager) GetDefaultProviderForMimeType(ctx context.Context, mimeType string) (*registrypb.ProviderInfo, error) {
	m.mu.RLock()
	entry, ok := m.mimetypes.Get(mimeType)
	m.mu.RUnlock()
	if !ok {
		return nil, errtypes.NotFound("default application provider not set for mime type " + mimeType)
	}

	defaultApp := entry.(*mimeTypeConfig).DefaultApp
	if defaultApp == "" {
		return nil, errtypes.NotFound("default application provider not set for mime type " + mimeType)
	}

	providers, err := m.providers(ctx)
	if err != nil {
		return nil, err
	}
	// The default is normally an app name, but an address is accepted too, as
	// that is what an older configuration may be carrying.
	for _, p := range providers {
		if p.info.Name == defaultApp || p.info.Address == defaultApp {
			return p.info, nil
		}
	}
	return nil, errtypes.NotFound("default application provider " + defaultApp + " for mime type " + mimeType + " is not available")
}

// SetDefaultProviderForMimeType records the app to open a mime type with by
// default. The catalogue is configuration, so the change only lives as long as
// this process does.
func (m *manager) SetDefaultProviderForMimeType(ctx context.Context, mimeType string, p *registrypb.ProviderInfo) error {
	// An app is identified by its name; an address is accepted as well, since
	// that is how a default used to be expressed.
	defaultApp := p.GetName()
	if defaultApp == "" {
		defaultApp = p.GetAddress()
	}
	if defaultApp == "" {
		return errtypes.BadRequest("appregistry: cannot set a default app provider without a name")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.mimetypes.Get(mimeType); ok {
		entry.(*mimeTypeConfig).DefaultApp = defaultApp
		return nil
	}
	m.mimetypes.Set(mimeType, &mimeTypeConfig{MimeType: mimeType, DefaultApp: defaultApp})
	return nil
}
