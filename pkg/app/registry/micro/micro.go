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

package micro

import (
	"container/heap"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	oreg "github.com/owncloud/ocis/v2/ocis-pkg/registry"
	"github.com/rs/zerolog/log"
	orderedmap "github.com/wk8/go-ordered-map"
	mreg "go-micro.dev/v4/registry"

	"github.com/cs3org/reva/v2/pkg/app"
	"github.com/cs3org/reva/v2/pkg/app/registry/registry"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
)

func init() {
	registry.Register("micro", New)
}

const defaultPriority = "0"

type mimeTypeConfig struct {
	MimeType      string `mapstructure:"mime_type"`
	Extension     string `mapstructure:"extension"`
	Name          string `mapstructure:"name"`
	Description   string `mapstructure:"description"`
	Icon          string `mapstructure:"icon"`
	DefaultApp    string `mapstructure:"default_app"`
	AllowCreation bool   `mapstructure:"allow_creation"`
	apps          providerHeap
}

type config struct {
	Namespace string            `mapstructure:"namespace"`
	MimeTypes []*mimeTypeConfig `mapstructure:"mime_types"`
}

func (c *config) init() {
	if c.Namespace == "" {
		c.Namespace = "cs3"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

type manager struct {
	namespace string
	mimetypes *orderedmap.OrderedMap // map[string]*mimeTypeConfig  ->  map the mime type to the addresses of the corresponding providers
	sync.RWMutex
	providers map[string]interface{}
}

// New returns an implementation of the app.Registry interface.
func New(m map[string]interface{}) (app.Registry, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	mimetypes := orderedmap.New()

	for _, mime := range c.MimeTypes {
		mimetypes.Set(mime.MimeType, mime)
	}

	newManager := manager{
		mimetypes: mimetypes,
		namespace: "bazFoo",
	}

	return &newManager, nil
}

func getPriority(p *registrypb.ProviderInfo) string {
	if p.Opaque != nil && len(p.Opaque.Map) != 0 {
		if priority, ok := p.Opaque.Map["priority"]; ok {
			return string(priority.GetValue())
		}
	}
	return defaultPriority
}

// use the UTF-8 record seperator
func splitMimeTypes(s string) []string {
	return strings.Split(s, "␞")
}

func joinMimeTypes(mimetypes []string) string {
	return strings.Join(mimetypes, "␞")
}

func (m *manager) providerFromMetadata(metadata map[string]string) registrypb.ProviderInfo {
	p := registrypb.ProviderInfo{
		MimeTypes: splitMimeTypes(metadata[m.namespace+".app-provider.mime_type"]),
		//		Address:     node.Address,
		Name:        metadata[m.namespace+".app-provider.name"],
		Description: metadata[m.namespace+".app-provider.description"],
		Icon:        metadata[m.namespace+".app-provider.icon"],
		DesktopOnly: metadata[m.namespace+".app-provider.desktop_only"] == "true",
		Capability:  registrypb.ProviderInfo_Capability(registrypb.ProviderInfo_Capability_value[metadata[m.namespace+".app-provider.capability"]]),
	}
	if metadata[m.namespace+".app-provider.priority"] != "" {
		p.Opaque = &typesv1beta1.Opaque{Map: map[string]*typesv1beta1.OpaqueEntry{
			"priority": {
				Decoder: "plain",
				Value:   []byte(metadata[m.namespace+".app-provider.priority"]),
			},
		}}
	}
	return p
}

func (m *manager) FindProviders(ctx context.Context, mimeType string) ([]*registrypb.ProviderInfo, error) {
	reg := oreg.GetRegistry()
	services, err := reg.GetService(m.namespace+".api.app-provider", mreg.GetContext(ctx))
	if err != nil {
		return nil, err
	}

	var providers = make([]*registrypb.ProviderInfo, 0)
	if len(services) == 0 {
		return nil, errtypes.NotFound("no application provider service found for mime type " + mimeType)
	}
	if len(services) > 1 {
		// TODO we could iterate over all ?
		return nil, errtypes.InternalError("more than one application provider service registered for mimetype " + mimeType)
	}

	// find longest match
	var match string
	for _, node := range services[0].Nodes {
		for _, providerMimeType := range splitMimeTypes(node.Metadata[m.namespace+".app-provider.mime_type"]) {
			if strings.HasPrefix(mimeType, providerMimeType) && len(providerMimeType) > len(match) {
				match = providerMimeType
			}
		}
	}

	if match == "" {
		return nil, errtypes.NotFound("application provider not found for mime type " + mimeType)
	}

	for _, node := range services[0].Nodes {
		for _, providerMimeType := range splitMimeTypes(node.Metadata[m.namespace+".app-provider.mime_type"]) {
			if match == providerMimeType {
				p := m.providerFromMetadata(node.Metadata)
				p.Address = node.Address
				providers = append(providers, &p)
			}
		}
	}

	// TODO sort by priority?

	return providers, nil
}

func (m *manager) AddProvider(ctx context.Context, p *registrypb.ProviderInfo) error {
	log := appctx.GetLogger(ctx)

	log.Debug().Interface("provider", p).Msg("AddProvider")

	reg := oreg.GetRegistry()

	serviceID := m.namespace + ".api.app-provider"

	node := &mreg.Node{
		Id:       serviceID + "-" + uuid.New().String(),
		Address:  p.Address,
		Metadata: make(map[string]string),
	}

	node.Metadata["registry"] = reg.String()
	node.Metadata["server"] = "grpc"
	node.Metadata["transport"] = "grpc"
	node.Metadata["protocol"] = "grpc"

	node.Metadata[m.namespace+".app-provider.mime_type"] = joinMimeTypes(p.MimeTypes)
	node.Metadata[m.namespace+".app-provider.name"] = p.Name
	node.Metadata[m.namespace+".app-provider.description"] = p.Description
	node.Metadata[m.namespace+".app-provider.icon"] = p.Icon
	//node.Metadata[m.namespace+".app-provider.default_app"] =
	node.Metadata[m.namespace+".app-provider.allow_creation"] = registrypb.ProviderInfo_Capability_name[int32(p.Capability)]
	node.Metadata[m.namespace+".app-provider.priority"] = getPriority(p)
	if p.DesktopOnly {
		node.Metadata[m.namespace+".app-provider.desktop_only"] = "true"
	}

	service := &mreg.Service{
		Name: serviceID,
		//Version:   version,
		Nodes:     []*mreg.Node{node},
		Endpoints: make([]*mreg.Endpoint, 0),
	}

	log.Info().Msgf("registering external service %v@%v", node.Id, node.Address)

	rOpts := []mreg.RegisterOption{mreg.RegisterTTL(time.Minute)} // TODO: this should be configurable
	if err := reg.Register(service, rOpts...); err != nil {
		log.Fatal().Err(err).Msgf("Registration error for external service %v", serviceID)
	}

	t := time.NewTicker(time.Second * 30)

	go func() {
		for {
			select {
			case <-t.C:
				log.Debug().Interface("service", service).Msg("refreshing external service-registration")
				err := reg.Register(service, rOpts...)
				if err != nil {
					log.Error().Err(err).Msgf("registration error for external service %v", serviceID)
				}
			case <-ctx.Done():
				log.Debug().Interface("service", service).Msg("unregistering")
				t.Stop()
				err := reg.Deregister(service)
				if err != nil {
					log.Err(err).Msgf("Error unregistering external service %v", serviceID)
				}
				// FIXME how do we end this when a provider reregisters?
				// Proposal: use manager.providers and register function there aswell
			}
		}
	}()

	return nil
}

func (m *manager) ListProviders(ctx context.Context) ([]*registrypb.ProviderInfo, error) {
	reg := oreg.GetRegistry()
	// FIXME: for some reason it can not get the service
	// seems like an issue with registering with grpc
	services, err := reg.GetService(m.namespace+".api.app-provider", mreg.GetContext(ctx))
	if err != nil {
		return nil, err
	}

	if len(services) == 0 {
		return nil, errtypes.NotFound("no application provider service registered")
	}
	if len(services) > 1 {
		return nil, errtypes.InternalError("more than one application provider services registered")
	}

	providers := make([]*registrypb.ProviderInfo, 0, len(services[0].Nodes))
	for _, node := range services[0].Nodes {
		p := m.providerFromMetadata(node.Metadata)
		p.Address = node.Address
		providers = append(providers, &p)
	}
	return providers, nil
}

func (m *manager) ListSupportedMimeTypes(ctx context.Context) ([]*registrypb.MimeTypeInfo, error) {
	m.RLock()
	defer m.RUnlock()

	res := make([]*registrypb.MimeTypeInfo, 0, m.mimetypes.Len())

	for pair := m.mimetypes.Oldest(); pair != nil; pair = pair.Next() {

		mime := pair.Value.(*mimeTypeConfig)

		res = append(res, &registrypb.MimeTypeInfo{
			MimeType:           mime.MimeType,
			Ext:                mime.Extension,
			Name:               mime.Name,
			Description:        mime.Description,
			Icon:               mime.Icon,
			AppProviders:       mime.apps.getOrderedProviderByPriority(),
			AllowCreation:      mime.AllowCreation,
			DefaultApplication: mime.DefaultApp,
		})

	}

	return res, nil
}

func (h providerHeap) getOrderedProviderByPriority() []*registrypb.ProviderInfo {
	providers := make([]*registrypb.ProviderInfo, 0, h.Len())
	for _, pp := range h {
		providers = append(providers, pp.provider)
	}
	return providers
}

func getIndex(h providerHeap, s *registrypb.ProviderInfo) (int, bool) {
	for i, e := range h {
		if equalsProviderInfo(e.provider, s) {
			return i, true
		}
	}
	return -1, false
}

func (m *manager) SetDefaultProviderForMimeType(ctx context.Context, mimeType string, p *registrypb.ProviderInfo) error {
	mimeInterface, ok := m.mimetypes.Get(mimeType)
	if ok {
		mime := mimeInterface.(*mimeTypeConfig)
		mime.DefaultApp = p.Address

		m.registerProvider(p, mime)
	} else {
		// the mime type should be already registered as config in the AppRegistry
		// we will create a new entry fot the mimetype, but leaving a warning for
		// future log inspection for weird behaviour
		log.Warn().Msgf("config for mimetype '%s' not found while setting a new default AppProvider", mimeType)
		m.mimetypes.Set(mimeType, dummyMimeType(mimeType, []*registrypb.ProviderInfo{p}))
	}
	return nil
}

func (m *manager) registerProvider(p *registrypb.ProviderInfo, mime *mimeTypeConfig) {
	m.AddProvider(context.Background(), p)
}

func dummyMimeType(m string, apps []*registrypb.ProviderInfo) *mimeTypeConfig {
	appsHeap := providerHeap{}
	for _, p := range apps {
		prio, err := strconv.ParseUint(getPriority(p), 10, 64)
		if err != nil {
			// TODO: maybe add some log here, providers might get lost
			continue
		}
		heap.Push(&appsHeap, providerWithPriority{
			provider: p,
			priority: prio,
		})
	}

	return &mimeTypeConfig{
		MimeType: m,
		apps:     appsHeap,
		//Extension: "", // there is no meaningful general extension, so omit it
		//Name:        "", // there is no meaningful general name, so omit it
		//Description: "", // there is no meaningful general description, so omit it
	}
}

func (m *manager) GetDefaultProviderForMimeType(ctx context.Context, mimeType string) (*registrypb.ProviderInfo, error) {
	m.RLock()
	defer m.RUnlock()

	mimeInterface, ok := m.mimetypes.Get(mimeType)
	if ok {
		mime := mimeInterface.(*mimeTypeConfig)
		// default by provider address
		if p, ok := m.providers[mime.DefaultApp]; ok {
			return p.(*registrypb.ProviderInfo), nil
		}

		// default by provider name
		for _, p := range m.providers {
			if p.(*registrypb.ProviderInfo).Name == mime.DefaultApp {
				return p.(*registrypb.ProviderInfo), nil
			}
		}
	}

	return nil, errtypes.NotFound("default application provider not set for mime type " + mimeType)
}

func equalsProviderInfo(p1, p2 *registrypb.ProviderInfo) bool {
	return p1.Name == p2.Name
}

type providerWithPriority struct {
	provider *registrypb.ProviderInfo
	priority uint64
}

type providerHeap []providerWithPriority

func (h providerHeap) Len() int {
	return len(h)
}

func (h providerHeap) Less(i, j int) bool {
	return h[i].priority > h[j].priority
}

func (h providerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *providerHeap) Push(x interface{}) {
	*h = append(*h, x.(providerWithPriority))
	fmt.Printf("Heap len: %d\n", h.Len())
}

func (h *providerHeap) Pop() interface{} {
	last := len(*h) - 1
	x := (*h)[last]
	*h = (*h)[:last]
	return x
}
