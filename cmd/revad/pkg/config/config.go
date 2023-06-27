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
	"io"
	"reflect"

	"github.com/BurntSushi/toml"
	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// Config holds the reva configuration.
type Config struct {
	GRPC       *GRPC       `key:"grpc"       mapstructure:"-"      default:"{}"`
	HTTP       *HTTP       `key:"http"       mapstructure:"-"      default:"{}"`
	Serverless *Serverless `key:"serverless" mapstructure:"-"      default:"{}"`
	Shared     *Shared     `key:"shared"     mapstructure:"shared" default:"{}" template:"-"`
	Log        *Log        `key:"log"        mapstructure:"log"    default:"{}" template:"-"`
	Core       *Core       `key:"core"       mapstructure:"core"   default:"{}" template:"-"`
	Vars       Vars        `key:"vars"       mapstructure:"vars"   default:"{}" template:"-"`
}

// Log holds the configuration for the logger.
type Log struct {
	Output string `key:"output" mapstructure:"output" default:"stdout"`
	Mode   string `key:"mode"   mapstructure:"mode"   default:"console"`
	Level  string `key:"level"  mapstructure:"level"  default:"trace"`
}

// Shared holds the shared configuration.
type Shared struct {
	JWTSecret             string   `key:"jwt_secret"                mapstructure:"jwt_secret"                default:"changemeplease"`
	GatewaySVC            string   `key:"gatewaysvc"                mapstructure:"gatewaysvc"                default:"0.0.0.0:19000"`
	DataGateway           string   `key:"datagateway"               mapstructure:"datagateway"               default:"http://0.0.0.0:19001/datagateway"`
	SkipUserGroupsInToken bool     `key:"skip_user_groups_in_token" mapstructure:"skip_user_groups_in_token"`
	BlockedUsers          []string `key:"blocked_users"             mapstructure:"blocked_users"             default:"[]"`
}

// Core holds the core configuration.
type Core struct {
	MaxCPUs            string `key:"max_cpus"             mapstructure:"max_cpus"`
	TracingEnabled     bool   `key:"tracing_enabled"      mapstructure:"tracing_enabled"`
	TracingEndpoint    string `key:"tracing_endpoint"     mapstructure:"tracing_endpoint"`
	TracingCollector   string `key:"tracing_collector"    mapstructure:"tracing_collector"`
	TracingServiceName string `key:"tracing_service_name" mapstructure:"tracing_service_name"`
	TracingService     string `key:"tracing_service"      mapstructure:"tracing_service"`
}

// Vars holds the a set of configuration paramenters that
// can be references by other parts of the configuration.
type Vars map[string]any

type Lookuper interface {
	Lookup(key string) (any, error)
}

// Load loads the configuration from the reader.
func Load(r io.Reader) (*Config, error) {
	var c Config
	if err := defaults.Set(&c); err != nil {
		return nil, err
	}
	var raw map[string]any
	if _, err := toml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, errors.Wrap(err, "config: error decoding toml data")
	}
	if err := c.parse(raw); err != nil {
		return nil, err
	}
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

// ApplyTemplates applies the templates defined in the configuration,
// replacing the template string with the value pointed by the given key.
func (c *Config) ApplyTemplates(l Lookuper) error {
	return applyTemplateByType(l, nil, reflect.ValueOf(c))
}

// Dump returns the configuration as a map.
func (c *Config) Dump() map[string]any {
	v := dumpByType(reflect.ValueOf(c))
	dump, ok := v.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("dump should be a map: got %T", dump))
	}
	return dump
}

func (c *Config) Lookup(key string) (any, error) {
	return lookupByType(key, reflect.ValueOf(c))
}
