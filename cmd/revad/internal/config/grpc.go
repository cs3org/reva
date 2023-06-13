package config

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type GRPC struct {
	Address string `mapstructure:"address" key:"address"`

	services    map[string]ServicesConfig `key:"services"`
	middlewares map[string]map[string]any `key:"middlewares"`
}

type ServicesConfig []*DriverConfig

func (c ServicesConfig) DriversNumber() int { return len(c) }

type DriverConfig struct {
	Config  map[string]any
	Address string
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

func (c *Config) parseGRPC() error {
	cfg, ok := c.raw["grpc"]
	if !ok {
		return nil
	}
	var grpc GRPC
	if err := mapstructure.Decode(cfg, &grpc); err != nil {
		return errors.Wrap(err, "config: error decoding grpc config")
	}

	// parse services
	svcCfg, ok := cfg.(map[string]any)["services"].(map[string]any)
	if !ok {
		return errors.New("grpc.services must be a map")
	}

	services := make(map[string]ServicesConfig)
	for name, cfg := range svcCfg {
		// cfg can be a list or a map
		cfgLst, ok := cfg.([]map[string]any)
		if ok {
			s, err := newSvcConfigFromList(cfgLst)
			if err != nil {
				return err
			}
			services[name] = s
			continue
		}
		cfgMap, ok := cfg.(map[string]any)
		if !ok {
			return fmt.Errorf("grpc.services.%s must be a list or a map. got %T", name, cfg)
		}
		services[name] = newSvcConfigFromMap(cfgMap)
	}
	grpc.services = services
	c.GRPC = &grpc

	for _, c := range grpc.services {
		for _, cfg := range c {
			cfg.Address = addressForService(grpc.Address, cfg.Config)
		}
	}
	return nil
}

type Service struct {
	Address string
	Name    string
	Config  map[string]any

	raw *DriverConfig
}

func (s *Service) SetAddress(address string) {
	s.raw.Address = address
}

type ServiceFunc func(*Service)

// ForEach iterates to each service/driver calling the function f.
func (g *GRPC) ForEach(f ServiceFunc) {
	for name, c := range g.services {
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

func addressForService(global string, cfg map[string]any) string {
	if address, ok := cfg["address"].(string); ok {
		return address
	}
	return global
}
