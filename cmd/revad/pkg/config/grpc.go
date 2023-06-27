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
	Address          string `mapstructure:"address"           key:"address"           default:"0.0.0.0:19000"`
	Network          string `mapstructure:"network"           key:"network"           default:"tcp"`
	ShutdownDeadline int    `mapstructure:"shutdown_deadline" key:"shutdown_deadline"`
	EnableReflection bool   `mapstructure:"enable_reflection" key:"enable_reflection"`

	Services     map[string]ServicesConfig `mapstructure:"-" key:"services"`
	Interceptors map[string]map[string]any `mapstructure:"-" key:"interceptors"`

	iterableImpl
}

func (g *GRPC) services() map[string]ServicesConfig     { return g.Services }
func (g *GRPC) interceptors() map[string]map[string]any { return g.Interceptors }

func (c *Config) parseGRPC(raw map[string]any) error {
	cfg, ok := raw["grpc"]
	if !ok {
		return nil
	}
	var grpc GRPC
	if err := mapstructure.Decode(cfg, &grpc); err != nil {
		return errors.Wrap(err, "config: error decoding grpc config")
	}

	cfgGRPC, ok := cfg.(map[string]any)
	if !ok {
		return errors.New("grpc must be a map")
	}

	services, err := parseServices(cfgGRPC)
	if err != nil {
		return err
	}

	interceptors, err := parseMiddlwares(cfgGRPC, "interceptors")
	if err != nil {
		return err
	}

	grpc.Services = services
	grpc.Interceptors = interceptors
	grpc.iterableImpl = iterableImpl{&grpc}
	c.GRPC = &grpc

	for _, c := range grpc.Services {
		for _, cfg := range c {
			cfg.Address = addressForService(grpc.Address, cfg.Config)
		}
	}
	return nil
}
