// Copyright 2018-2023 CERN
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

	"github.com/cs3org/reva/pkg/appctx"
	datatxregistry "github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	global.Register("dataprovider", New)
}

type config struct {
	Prefix   string                            `mapstructure:"prefix" docs:"data;The prefix to be used for this HTTP service"`
	Driver   string                            `mapstructure:"driver" docs:"localhome;The storage driver to be used."`
	Drivers  map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:pkg/storage/fs/localhome/localhome.go;The configuration for the storage driver"`
	DataTXs  map[string]map[string]interface{} `mapstructure:"data_txs" docs:"url:pkg/rhttp/datatx/manager/simple/simple.go;The configuration for the data tx protocols"`
	Timeout  int64                             `mapstructure:"timeout"`
	Insecure bool                              `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "data"
	}
	if c.Driver == "" {
		c.Driver = "localhome"
	}
}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
	dataTXs map[string]http.Handler
}

// New returns a new datasvc.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
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

	err = s.setHandler()
	return s, err
}

func getFS(ctx context.Context, c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func getDataTXs(ctx context.Context, c *config, fs storage.FS) (map[string]http.Handler, error) {
	if c.DataTXs == nil {
		c.DataTXs = make(map[string]map[string]interface{})
	}
	if len(c.DataTXs) == 0 {
		c.DataTXs["simple"] = make(map[string]interface{})
		c.DataTXs["spaces"] = make(map[string]interface{})
		c.DataTXs["tus"] = make(map[string]interface{})
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

func (s *svc) Unprotected() []string {
	return []string{
		"/tus",
	}
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() error {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Msgf("dataprovider routing: path=%s", r.URL.Path)

		head, tail := router.ShiftPath(r.URL.Path)

		if handler, ok := s.dataTXs[head]; ok {
			r.URL.Path = tail
			handler.ServeHTTP(w, r)
			return
		}

		// If we don't find a prefix match for any of the protocols, upload the resource
		// through the direct HTTP protocol
		if handler, ok := s.dataTXs["simple"]; ok {
			handler.ServeHTTP(w, r)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	})

	return nil
}
