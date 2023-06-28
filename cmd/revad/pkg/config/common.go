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
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type iterable interface {
	services() map[string]ServicesConfig
	interceptors() map[string]map[string]any
}

type iterableImpl struct{ i iterable }

// ServicesConfig holds the configuration for reva services.
type ServicesConfig []*DriverConfig

// DriversNumber return the number of driver configured for the service.
func (c ServicesConfig) DriversNumber() int { return len(c) }

// DriverConfig holds the configuration for a driver.
type DriverConfig struct {
	Config  map[string]any `key:",squash"`
	Address string         `key:"address"`
	Network string         `key:"network"`
	Label   string         `key:"-"`
}

func (s *ServicesConfig) Add(svc string, c *DriverConfig) {
	l := len(*s)
	if l == 0 {
		// the label is simply the service name
		c.Label = svc
	} else {
		c.Label = label(svc, l)
		if l == 1 {
			(*s)[0].Label = label(svc, 0)
		}
	}
	*s = append(*s, c)
}

func newSvcConfigFromList(name string, l []map[string]any) (ServicesConfig, error) {
	cfg := make(ServicesConfig, 0, len(l))
	for _, c := range l {
		cfg.Add(name, &DriverConfig{Config: c})
	}
	return cfg, nil
}

func newSvcConfigFromMap(name string, m map[string]any) ServicesConfig {
	s, _ := newSvcConfigFromList(name, []map[string]any{m})
	return s
}

func parseServices(cfg map[string]any) (map[string]ServicesConfig, error) {
	// parse services
	svcCfg, ok := cfg["services"].(map[string]any)
	if !ok {
		return nil, errors.New("grpc.services must be a map")
	}

	services := make(map[string]ServicesConfig)
	for name, cfg := range svcCfg {
		// cfg can be a list or a map
		cfgLst, ok := cfg.([]map[string]any)
		if ok {
			s, err := newSvcConfigFromList(name, cfgLst)
			if err != nil {
				return nil, err
			}
			services[name] = s
			continue
		}
		cfgMap, ok := cfg.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("grpc.services.%s must be a list or a map. got %T", name, cfg)
		}
		services[name] = newSvcConfigFromMap(name, cfgMap)
	}

	return services, nil
}

func parseMiddlwares(cfg map[string]any, key string) (map[string]map[string]any, error) {
	m := make(map[string]map[string]any)

	mid, ok := cfg[key]
	if !ok {
		return m, nil
	}

	if err := mapstructure.Decode(mid, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Service contains the configuration for a service.
type Service struct {
	Address string
	Network string
	Name    string
	Label   string
	Config  map[string]any

	raw *DriverConfig
}

// SetAddress sets the address for the service in the configuration.
func (s *Service) SetAddress(address string) {
	s.Address = address
	s.raw.Address = address
}

// ServiceFunc is an helper function used to pass the service config
// to the ForEachService func.
type ServiceFunc func(*Service)

// Interceptor contains the configuration for an interceptor.
type Interceptor struct {
	Name   string
	Config map[string]any
}

// InterceptorFunc is an helper function used to pass the interface config
// to the ForEachInterceptor func.
type InterceptorFunc func(*Interceptor)

// ForEachService iterates to each service/driver calling the function f.
func (i iterableImpl) ForEachService(f ServiceFunc) {
	if i.i == nil {
		return
	}
	for name, c := range i.i.services() {
		for _, cfg := range c {
			f(&Service{
				raw:     cfg,
				Address: cfg.Address,
				Network: cfg.Network,
				Label:   cfg.Label,
				Name:    name,
				Config:  cfg.Config,
			})
		}
	}
}

func label(name string, i int) string {
	return fmt.Sprintf("%s_%d", name, i)
}

// ForEachInterceptor iterates to each middleware calling the function f.
func (i iterableImpl) ForEachInterceptor(f InterceptorFunc) {
	for name, c := range i.i.interceptors() {
		f(&Interceptor{
			Name:   name,
			Config: c,
		})
	}
}

func addressForService(global string, cfg map[string]any) string {
	if address, ok := cfg["address"].(string); ok {
		return address
	}
	return global
}

func networkForService(global string, cfg map[string]any) string {
	if network, ok := cfg["network"].(string); ok {
		return network
	}
	return global
}
