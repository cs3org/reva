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

package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// HTTP holds the configuration for the HTTP services.
type HTTP struct {
	Network  string `mapstructure:"network" key:"network"`
	Address  string `mapstructure:"address" key:"address"`
	CertFile string `mapstructure:"certfile" key:"certfile"`
	KeyFile  string `mapstructure:"keyfile" key:"keyfile"`

	Services    map[string]ServicesConfig `mapstructure:"-" key:"services"`
	Middlewares map[string]map[string]any `mapstructure:"-" key:"middlewares"`

	iterableImpl
}

func (h *HTTP) services() map[string]ServicesConfig     { return h.Services }
func (h *HTTP) interceptors() map[string]map[string]any { return h.Middlewares }

func (c *Config) parseHTTP(raw map[string]any) error {
	cfg, ok := raw["http"]
	if !ok {
		return nil
	}
	var http HTTP
	if err := mapstructure.Decode(cfg, &http); err != nil {
		return errors.Wrap(err, "config: error decoding http config")
	}

	cfgHTTP, ok := cfg.(map[string]any)
	if !ok {
		return errors.New("http must be a map")
	}

	services, err := parseServices(cfgHTTP)
	if err != nil {
		return err
	}

	middlewares, err := parseMiddlwares(cfgHTTP, "middlewares")
	if err != nil {
		return err
	}

	http.Services = services
	http.Middlewares = middlewares
	http.iterableImpl = iterableImpl{&http}
	c.HTTP = &http

	for _, c := range http.Services {
		for _, cfg := range c {
			cfg.Address = addressForService(http.Address, cfg.Config)
		}
	}
	return nil
}
