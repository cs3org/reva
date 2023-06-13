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
	"io"
	"reflect"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Config struct {
	raw map[string]any

	GRPC *GRPC `key:"grpc"`
	// Serverless *Serverless // TODO

	// TODO: add log, shared, core
}

type Serverless struct {
}

// Load loads the configuration from the reader.
func Load(r io.Reader) (*Config, error) {
	var c Config
	if _, err := toml.NewDecoder(r).Decode(&c.raw); err != nil {
		return nil, errors.Wrap(err, "config: error decoding toml data")
	}
	if err := c.parse(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) parse() error {
	if err := c.parseGRPC(); err != nil {
		return err
	}
	return nil
}

func (c *Config) ApplyTemplates() error {
	return nil
}

func (c *Config) lookup(key string) (any, error) {
	return lookupStruct(key, reflect.ValueOf(c))
}
