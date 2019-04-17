// Copyright 2018-2019 CERN
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
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Network  string   `json:"network"`
	Address  string   `json:"address"`
	Services []string `json:"services"`

	AuthSVC struct {
		Driver  string      `json:"driver"`
		Options interface{} `json:"options"`
	} `json:"auth_svc"`

	StorageProviderSVC struct {
		TemporaryFolder string      `json:"temporary_folder"`
		Driver          string      `json:"driver"`
		Options         interface{} `json:"options"`
	} `json:"storage_provider_svc"`
}

func LoadFromFile(fn string) (*Config, error) {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
