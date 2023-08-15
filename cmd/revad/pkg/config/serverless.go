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
func (s *Serverless) ForEach(f func(name string, config map[string]any) error) error {
	for name, cfg := range s.Services {
		if err := f(name, cfg); err != nil {
			return err
		}
	}
	return nil
}
