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

const (
	// EndpointPanel is the endpoint of the web interface panel.
	EndpointPanel = "/panel"

	// EndpointList is the endpoint path for listing all stored accounts.
	EndpointList = "/list"

	// EndpointCreate is the endpoint path for account creation.
	EndpointCreate = "/create"
	// EndpointUpdate is the endpoint path for account updates.
	EndpointUpdate = "/update"
	// EndpointRemove is the endpoint path for account removal.
	EndpointRemove = "/remove"

	// EndpointAuthorize is the endpoint path for account authorization
	EndpointAuthorize = "/authorize"
)
