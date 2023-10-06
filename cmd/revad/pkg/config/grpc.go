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

// GRPC holds the configuration for the GRPC services.
type GRPC struct {
	Address          Address `key:"address"           mapstructure:"address"`
	Network          string  `default:"tcp"           key:"network"                    mapstructure:"network"`
	ShutdownDeadline int     `key:"shutdown_deadline" mapstructure:"shutdown_deadline"`
	EnableReflection bool    `key:"enable_reflection" mapstructure:"enable_reflection"`

	Services     map[string]ServicesConfig `key:"services"     mapstructure:"-"`
	Interceptors map[string]map[string]any `key:"interceptors" mapstructure:"-"`

	iterableImpl
}

func (g *GRPC) services() map[string]ServicesConfig     { return g.Services }
func (g *GRPC) interceptors() map[string]map[string]any { return g.Interceptors }

func (c *Config) parseGRPC(raw map[string]any) error {
	cfg, ok := raw["grpc"]
	if !ok {
		return nil
	}
	if err := mapstructure.Decode(cfg, c.GRPC); err != nil {
		return errors.Wrap(err, "config: error decoding grpc config")
	}

	cfgGRPC, ok := cfg.(map[string]any)
	if !ok {
		return errors.New("grpc must be a map")
	}

	services, err := parseServices("grpc", cfgGRPC)
	if err != nil {
		return err
	}

	interceptors, err := parseMiddlwares(cfgGRPC, "interceptors")
	if err != nil {
		return err
	}

	c.GRPC.Services = services
	c.GRPC.Interceptors = interceptors
	c.GRPC.iterableImpl = iterableImpl{c.GRPC}

	for _, svc := range c.GRPC.Services {
		for _, cfg := range svc {
			cfg.Address = addressForService(c.GRPC.Address, cfg.Config)
			cfg.Network = networkForService(c.HTTP.Network, cfg.Config)
		}
	}
	return nil
}
