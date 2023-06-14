package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type GRPC struct {
	Address          string `mapstructure:"address" key:"address"`
	Network          string `mapstructure:"network" key:"network"`
	ShutdownDeadline int    `mapstructure:"shutdown_deadline" key:"shutdown_deadline"`
	EnableReflection bool   `mapstructure:"enable_reflection" key:"enable_reflection"`

	Services     map[string]ServicesConfig `key:"services"`
	Interceptors map[string]map[string]any `key:"interceptors"`

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
