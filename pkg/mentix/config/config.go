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

	Connectors struct {
		GOCDB struct {
			Address string `mapstructure:"address"`
			Scope   string `mapstructure:"scope"`
		} `mapstructure:"gocdb"`

		LocalFile struct {
			File string `mapstructure:"file"`
		} `mapstructure:"localfile"`
	} `mapstructure:"connectors"`

	UpdateInterval string `mapstructure:"update_interval"`

	Exporters struct {
		WebAPI struct {
			Endpoint string `mapstructure:"endpoint"`
		} `mapstructure:"webapi"`

		CS3API struct {
			Endpoint string `mapstructure:"endpoint"`
		} `mapstructure:"cs3api"`

		SiteLocations struct {
			Endpoint string `mapstructure:"endpoint"`
		} `mapstructure:"siteloc"`

		PrometheusSD struct {
			MetricsOutputFile  string `mapstructure:"metrics_output_file"`
			BlackboxOutputFile string `mapstructure:"blackbox_output_file"`
		} `mapstructure:"promsd"`
	} `mapstructure:"exporters"`

	// Internal settings
	EnabledConnectors []string `mapstructure:"-"`
	EnabledImporters  []string `mapstructure:"-"`
	EnabledExporters  []string `mapstructure:"-"`
}

// Init sets sane defaults.
func (c *Configuration) Init() {
	if c.Prefix == "" {
		c.Prefix = "mentix"
	}
	// TODO(daniel): add default that works out of the box
}
