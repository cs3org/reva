package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type Serverless struct {
	Services map[string]map[string]any `mapstructure:"services"`
}

func (c *Config) parseServerless(raw map[string]any) error {
	cfg, ok := raw["serverless"]
	if !ok {
		return nil
	}

	var s Serverless
	if err := mapstructure.Decode(cfg, &s); err != nil {
		return errors.Wrap(err, "config: error decoding serverless config")
	}

	c.Serverless = &s
	return nil
}

func (s *Serverless) ForEach(f func(name string, config map[string]any)) {
	for name, cfg := range s.Services {
		f(name, cfg)
	}
}
