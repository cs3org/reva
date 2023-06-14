package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type HTTP struct {
	Network  string `mapstructure:"network" key:"network"`
	Address  string `mapstructure:"address" key:"address"`
	CertFile string `mapstructure:"certfile" key:"certfile"`
	KeyFile  string `mapstructure:"keyfile" key:"keyfile"`

	_services    map[string]ServicesConfig `key:"services"`
	_middlewares map[string]map[string]any `key:"middlewares"`

	iterableImpl
}

func (h *HTTP) services() map[string]ServicesConfig     { return h._services }
func (h *HTTP) interceptors() map[string]map[string]any { return h._middlewares }

func (c *Config) parseHTTP() error {
	cfg, ok := c.raw["http"]
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

	http._services = services
	http._middlewares = middlewares
	http.iterableImpl = iterableImpl{&http}
	c.HTTP = &http

	for _, c := range http._services {
		for _, cfg := range c {
			cfg.Address = addressForService(http.Address, cfg.Config)
		}
	}
	return nil
}
