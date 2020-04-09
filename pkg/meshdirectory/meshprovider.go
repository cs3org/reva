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

package meshdirectory

// MeshProvider holds provider metadata
type MeshProvider struct {
	ID          string `json:"id" mapstructure:"id"`
	Org         string `json:"org" mapstructure:"org"`
	Name        string `json:"name" mapstructure:"description"`
	Description string `json:"description" mapstructure:"description"`
	Domain      string `json:"domain" mapstructure:"domain"`
	Logo        string `json:"logo" mapstructure:"logo"`
	Homepage    string `json:"homepage" mapstructure:"homepage"`
	OCM         string `json:"ocm_api" mapstructure:"ocm_api"`
	Version     string `json:"api_version" mapstructure:"api_version"`
}
