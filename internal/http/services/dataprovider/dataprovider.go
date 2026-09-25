// Copyright 2018-2024 CERN
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

package dataprovider

import (
	"context"
	"fmt"
	"net/http"

	svcregistry "github.com/cs3org/reva/v3/pkg/registry"
	datatxregistry "github.com/cs3org/reva/v3/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the data provider is served.
const mount = "/data"

func init() {
	global.Register("dataprovider", New)
}

type config struct {
	Driver   string                    `docs:"localhome;The storage driver to be used."                                                  mapstructure:"driver"`
	Drivers  map[string]map[string]any `docs:"url:pkg/storage/fs/localhome/localhome.go;The configuration for the storage driver"        mapstructure:"drivers"`
	DataTXs  map[string]map[string]any `docs:"url:pkg/rhttp/datatx/manager/simple/simple.go;The configuration for the data tx protocols" mapstructure:"data_txs"`
	Timeout  int64                     `mapstructure:"timeout"`
	Insecure bool                      `docs:"false;Whether to skip certificate checks when sending requests."                           mapstructure:"insecure"`
	// MountID ties this data provider to the storage provider serving the same
	// storage, so the storage provider can discover it through the registry.
	MountID string `docs:"-;The mount id of the storage this data provider serves." mapstructure:"mount_id"`
	// PublicURL is the externally reachable base URL, advertised in the registry.
	// If empty, consumers reconstruct it from scheme+address+prefix.
	PublicURL string `mapstructure:"public_url"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "localhome"
	}
}

type svc struct {
	conf    *config
	storage storage.FS
	dataTXs map[string]http.Handler
}

// New returns a new datasvc.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	fs, err := getFS(ctx, &c)
	if err != nil {
		return nil, err
	}

	dataTXs, err := getDataTXs(ctx, &c, fs)
	if err != nil {
		return nil, err
	}

	s := &svc{
		storage: fs,
		conf:    &c,
		dataTXs: dataTXs,
	}

	return s, nil
}

// RegistryMetadata advertises the mount affinity, the path the service is
// served under and the externally reachable URL, so a storage provider can
// discover this data provider through the registry.
func (s *svc) RegistryMetadata() map[string]string {
	m := map[string]string{svcregistry.MetaPrefix: mount}
	if s.conf.MountID != "" {
		m[svcregistry.MetaMountID] = s.conf.MountID
	}
	if s.conf.PublicURL != "" {
		m[svcregistry.MetaPublicURL] = s.conf.PublicURL
	}
	return m
}

func getFS(ctx context.Context, c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func getDataTXs(ctx context.Context, c *config, fs storage.FS) (map[string]http.Handler, error) {
	if c.DataTXs == nil {
		c.DataTXs = make(map[string]map[string]any)
	}
	if len(c.DataTXs) == 0 {
		c.DataTXs["simple"] = make(map[string]any)
		c.DataTXs["spaces"] = make(map[string]any)
		c.DataTXs["tus"] = make(map[string]any)
	}

	txs := make(map[string]http.Handler)
	for t := range c.DataTXs {
		if f, ok := datatxregistry.NewFuncs[t]; ok {
			if tx, err := f(ctx, c.DataTXs[t]); err == nil {
				if handler, err := tx.Handler(fs); err == nil {
					txs[t] = handler
				}
			}
		}
	}
	return txs, nil
}

func (s *svc) Close() error {
	return nil
}

// Routes mounts one subtree per data transfer protocol. They are mounts
// because each protocol handler owns its URL space - tus in particular is an
// unrouted handler from its own library - and each is given the path with its
// own prefix stripped, as it expects. Anything not addressed to a protocol
// goes through the direct HTTP one.
func (s *svc) Routes(r *router.Router) {
	for name, h := range s.dataTXs {
		prefix := mount + "/" + name
		var opts []router.Option
		if name == "tus" {
			// tus authenticates with the transfer token it was handed.
			opts = append(opts, router.Unprotected())
		}
		r.Mount(prefix, http.StripPrefix(prefix, h), opts...)
	}

	if h, ok := s.dataTXs["simple"]; ok {
		r.Mount(mount, http.StripPrefix(mount, h))
	}
}
