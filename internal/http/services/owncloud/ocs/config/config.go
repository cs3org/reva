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

import (
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
	"github.com/cs3org/reva/pkg/sharedconf"
)

// Config holds the config options that need to be passed down to all ocs handlers
type Config struct {
	Prefix       string                `mapstructure:"prefix"`
	Config       data.ConfigData       `mapstructure:"config"`
	Capabilities data.CapabilitiesData `mapstructure:"capabilities"`
	GatewaySvc   string                `mapstructure:"gatewaysvc"`
	DisableTus   bool                  `mapstructure:"disable_tus"`
}

// Init sets sane defaults
func (c *Config) Init() {
	if c.Prefix == "" {
		c.Prefix = "ocs"
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}
