// Copyright 2018-2020 CERN
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
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/datatx"
	datatxregistry "github.com/cs3org/reva/pkg/rhttp/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("dataprovider", New)
}

type config struct {
	Prefix  string                            `mapstructure:"prefix" docs:"data;The prefix to be used for this HTTP service"`
	Driver  string                            `mapstructure:"driver" docs:"local;The storage driver to be used."`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers" docs:"url:docs/config/packages/storage/fs;The configuration for the storage driver"`
	DataTX  string                            `mapstructure:"datatx" docs:"simple;The data transfer protocol to use"`
	DataTXs map[string]map[string]interface{} `mapstructure:"datatxs" docs:"url:docs/config/packages/rhttp/datatx;The data transfer protocol to use"`
}

func (c *config) init() {
	if c.Prefix == "" {
		c.Prefix = "data"
	}

	if c.Driver == "" {
		c.Driver = "local"
	}

}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
	datatx  datatx.DataTX
}

// New returns a new datasvc
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	fs, err := getFS(conf)
	if err != nil {
		return nil, err
	}

	datatx, err := getDataTX(conf)
	if err != nil {
		return nil, err
	}

	s := &svc{
		storage: fs,
		datatx:  datatx,
		conf:    conf,
	}

	if err := s.setHandler(); err != nil {
		return nil, err
	}

	return s, err
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{}
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() error {
	h, err := s.datatx.Handler(s.conf.Prefix, s.storage)
	if err != nil {
		return err
	}

	s.handler = h
	return nil
}

func getDataTX(c *config) (datatx.DataTX, error) {
	if f, ok := datatxregistry.NewFuncs[c.Driver]; ok {
		return f(c.DataTXs[c.DataTX])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}
