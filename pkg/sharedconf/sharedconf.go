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

package sharedconf

import (
	"github.com/mitchellh/mapstructure"
)

var sharedConf = &conf{}

type conf struct {
	JWTSecret  string `mapstructure:"jwt_secret"`
	GatewaySVC string `mapstructure:"gatewaysvc"`
}

// Decode decodes the configuration.
func Decode(v interface{}) error {
	if err := mapstructure.Decode(v, sharedConf); err != nil {
		return err
	}

	return nil
}

// GetJWTSecret returns the package level configured jwt secret if not overwriten.
func GetJWTSecret(val string) string {
	if val == "" {
		return sharedConf.JWTSecret
	}
	return val
}

// GetGatewaySVC returns the package level configured gateway service if not overwriten.
func GetGatewaySVC(val string) string {
	if val == "" {
		return sharedConf.GatewaySVC
	}
	return val
}
