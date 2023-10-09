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
	"os"
	"path/filepath"
	"reflect"

	"github.com/BurntSushi/toml"
	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// Config holds the reva configuration.
type Config struct {
	GRPC       *GRPC       `default:"{}" key:"grpc"       mapstructure:"-"`
	HTTP       *HTTP       `default:"{}" key:"http"       mapstructure:"-"`
	Serverless *Serverless `default:"{}" key:"serverless" mapstructure:"-"`
	Shared     *Shared     `default:"{}" key:"shared"     mapstructure:"shared"`
	Log        *Log        `default:"{}" key:"log"        mapstructure:"log"    template:"-"`
	Core       *Core       `default:"{}" key:"core"       mapstructure:"core"   template:"-"`
	Vars       Vars        `default:"{}" key:"vars"       mapstructure:"vars"   template:"-"`
}

// Log holds the configuration for the logger.
type Log struct {
	Output string `default:"stdout"  key:"output" mapstructure:"output"`
	Mode   string `default:"console" key:"mode"   mapstructure:"mode"`
	Level  string `default:"trace"   key:"level"  mapstructure:"level"`
}

// Shared holds the shared configuration.
type Shared struct {
	JWTSecret             string   `default:"changemeplease"                   key:"jwt_secret"                         mapstructure:"jwt_secret"`
	GatewaySVC            string   `default:"0.0.0.0:19000"                    key:"gatewaysvc"                         mapstructure:"gatewaysvc"`
	DataGateway           string   `default:"http://0.0.0.0:19001/datagateway" key:"datagateway"                        mapstructure:"datagateway"`
	SkipUserGroupsInToken bool     `key:"skip_user_groups_in_token"            mapstructure:"skip_user_groups_in_token"`
	BlockedUsers          []string `default:"[]"                               key:"blocked_users"                      mapstructure:"blocked_users"`
}

// Core holds the core configuration.
type Core struct {
	MaxCPUs            string `key:"max_cpus"             mapstructure:"max_cpus"`
	ConfigDumpFile     string `key:"config_dump_file"     mapstructure:"config_dump_file"`
	TracingEnabled     bool   `default:"true"             key:"tracing_enabled"               mapstructure:"tracing_enabled"`
	TracingEndpoint    string `default:"localhost:6831"   key:"tracing_endpoint"              mapstructure:"tracing_endpoint"`
	TracingCollector   string `key:"tracing_collector"    mapstructure:"tracing_collector"`
	TracingServiceName string `key:"tracing_service_name" mapstructure:"tracing_service_name"`
	TracingService     string `key:"tracing_service"      mapstructure:"tracing_service"`
}

// Vars holds the a set of configuration paramenters that
// can be references by other parts of the configuration.
type Vars map[string]any

// Lookuper is the interface for getting the value
// associated with a given key.
type Lookuper interface {
	// Lookup get the value associated to thye given key.
	// It returns ErrKeyNotFound if the key does not exists.
	Lookup(key string) (any, error)
}

func (c *Config) init() {
	if c.Core.ConfigDumpFile == "" {
		c.Core.ConfigDumpFile = filepath.Join(os.TempDir(), "reva-dump.toml")
	}
}

// Load loads the configuration from the reader.
func Load(r io.Reader) (*Config, error) {
	var c Config
	if err := defaults.Set(&c); err != nil {
		return nil, err
	}
	c.init()
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

// Lookup gets the value associated to the given key in the config.
// The key is in the form <subkey1>.<subkey2>[<index>], allowing accessing
// recursively the config on subfields, in case of maps or structs or
// types implementing the Getter interface, or elements in a list by the
// given index.
func (c *Config) Lookup(key string) (any, error) {
	// check thet key is valid, meaning it starts with one of
	// the fields of the config struct
	if !c.isValidKey(key) {
		return nil, nil
	}
	val, err := lookupByType(key, reflect.ValueOf(c))
	if err != nil {
		return nil, errors.Wrapf(err, "lookup: error on key '%s'", key)
	}
	return val, nil
}

func (c *Config) isValidKey(key string) bool {
	cmd, _, err := parseNext(key)
	if err != nil {
		return false
	}
	f, ok := cmd.(FieldByKey)
	if !ok {
		return false
	}
	k := f.Key
	e := reflect.TypeOf(c).Elem()
	for i := 0; i < e.NumField(); i++ {
		f := e.Field(i)
		prefix := f.Tag.Get("key")
		if prefix == "" || prefix == "-" {
			continue
		}
		if k == prefix {
			return true
		}
	}
	return false
}
