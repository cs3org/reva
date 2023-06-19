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
}

func newSvcConfigFromList(l []map[string]any) (ServicesConfig, error) {
	var cfg ServicesConfig
	for _, c := range l {
		cfg = append(cfg, &DriverConfig{Config: c})
	}
	return cfg, nil
}

func newSvcConfigFromMap(m map[string]any) ServicesConfig {
	s, _ := newSvcConfigFromList([]map[string]any{m})
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
			s, err := newSvcConfigFromList(cfgLst)
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
		services[name] = newSvcConfigFromMap(cfgMap)
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
	Name    string
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
	for name, c := range i.i.services() {
		for _, cfg := range c {
			f(&Service{
				raw:     cfg,
				Address: cfg.Address,
				Name:    name,
				Config:  cfg.Config,
			})
		}
	}
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