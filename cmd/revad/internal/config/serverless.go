package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// Serverless holds the configuration for the serverless services.
type Serverless struct {
	Services map[string]map[string]any `key:"services" mapstructure:"services"`
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

// ForEach iterates to each service calling the function f.
func (s *Serverless) ForEach(f func(name string, config map[string]any)) {
	for name, cfg := range s.Services {
		f(name, cfg)
	}
}
