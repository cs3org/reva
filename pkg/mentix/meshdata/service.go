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

package meshdata

// Service represents a service managed by Mentix.
type Service struct {
	*ServiceEndpoint

	Host                string
	AdditionalEndpoints []*ServiceEndpoint
}

// ServiceType represents a service type managed by Mentix.
type ServiceType struct {
	Name        string
	Description string
}

// ServiceEndpoint represents a service endpoint managed by Mentix.
type ServiceEndpoint struct {
	Type        *ServiceType
	Name        string
	URL         string
	IsMonitored bool
	Properties  map[string]string
}
