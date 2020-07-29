// Copyright 2018-2020 CERN
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

// Configuration holds the general Mentix configuration.
type Configuration struct {
	Prefix string `mapstructure:"prefix"`

	Connector      string   `mapstructure:"connector"`
	Exporters      []string `mapstructure:"exporters"`
	UpdateInterval string   `mapstructure:"update_interval"`

	GOCDB struct {
		Address string `mapstructure:"address"`
		Scope   string `mapstructure:"scope"`
	} `mapstructure:"gocdb"`

	WebAPI struct {
		Endpoint string `mapstructure:"endpoint"`
	} `yaml:"webapi"`

	CS3API struct {
		Endpoint string `mapstructure:"endpoint"`
	} `yaml:"cs3api"`

	SiteLocations struct {
		Endpoint string `mapstructure:"endpoint"`
	} `yaml:"siteloc"`

	PrometheusFileSD struct {
		OutputFile string `mapstructure:"output_file"`
	} `mapstructure:"prom_filesd"`
}

// Init sets sane defaults
func (c *Configuration) Init() {
	if c.Prefix == "" {
		c.Prefix = "mentix"
	}
	// TODO(daniel): add default that works out of the box
}
