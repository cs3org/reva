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
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type Config struct {
	GRPC       *GRPC       `key:"grpc"       mapsrtcuture:"-"`
	HTTP       *HTTP       `key:"http"       mapstructure:"-"`
	Serverless *Serverless `key:"serverless" mapstructure:"-"`
	Shared     *Shared     `key:"shared"     mapstructure:"shared" template:"-"`
	Log        *Log        `key:"log"        mapstructure:"log"    template:"-"`
	Core       *Core       `key:"core"       mapstructure:"core"   template:"-"`
	Vars       Vars        `key:"vars"       mapstructure:"vars"   template:"-"`
}

type Log struct {
	Output string `key:"output" mapstructure:"output"`
	Mode   string `key:"mode"   mapstructure:"mode"`
	Level  string `key:"level"  mapstructure:"level"`
}

type Shared struct {
	JWTSecret             string   `key:"jwt_secret"                mapstructure:"jwt_secret"`
	GatewaySVC            string   `key:"gatewaysvc"                mapstructure:"gatewaysvc"`
	DataGateway           string   `key:"datagateway"               mapstructure:"datagateway"`
	SkipUserGroupsInToken bool     `key:"skip_user_groups_in_token" mapstructure:"skip_user_groups_in_token"`
	BlockedUsers          []string `key:"blocked_users"             mapstructure:"blocked_users"`
}

type Core struct {
	MaxCPUs            string `key:"max_cpus"             mapstructure:"max_cpus"`
	TracingEnabled     bool   `key:"tracing_enabled"      mapstructure:"tracing_enabled"`
	TracingEndpoint    string `key:"tracing_endpoint"     mapstructure:"tracing_endpoint"`
	TracingCollector   string `key:"tracing_collector"    mapstructure:"tracing_collector"`
	TracingServiceName string `key:"tracing_service_name" mapstructure:"tracing_service_name"`
	TracingService     string `key:"tracing_service"      mapstructure:"tracing_service"`
}

type Vars map[string]any

// Load loads the configuration from the reader.
func Load(r io.Reader) (*Config, error) {
	var c Config
	var raw map[string]any
	if _, err := toml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, errors.Wrap(err, "config: error decoding toml data")
	}
	if err := c.parse(raw); err != nil {
		return nil, err
	}
	c.Vars = make(Vars)
	return &c, nil
}

func (c *Config) parse(raw map[string]any) error {
	if err := c.parseGRPC(raw); err != nil {
		return err
	}
	if err := c.parseHTTP(raw); err != nil {
		return err
	}
	if err := c.parseServerless(raw); err != nil {
		return err
	}
	if err := mapstructure.Decode(raw, c); err != nil {
		return err
	}
	return nil
}

func (c *Config) ApplyTemplates() error {
	return c.applyTemplateByType(reflect.ValueOf(c))
}

func (c *Config) lookup(key string) (any, error) {
	return lookupByType(key, reflect.ValueOf(c))
}
