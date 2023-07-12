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

	datatxregistry "github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "dataprovider"

func init() {
	global.Register(name, New)
}

type config struct {
	Driver   string                            `mapstructure:"driver" docs:"localhome;The storage driver to be used."`
	Drivers  map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:pkg/storage/fs/localhome/localhome.go;The configuration for the storage driver"`
	DataTXs  map[string]map[string]interface{} `mapstructure:"data_txs" docs:"url:pkg/rhttp/datatx/manager/simple/simple.go;The configuration for the data tx protocols"`
	Timeout  int64                             `mapstructure:"timeout"`
	Insecure bool                              `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
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

	return s, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Register(r mux.Router) {
	r.Route("/data", func(r mux.Router) {
		for prot, handler := range s.dataTXs {
			r.Handle("/"+prot, handler)
		}
	})
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
