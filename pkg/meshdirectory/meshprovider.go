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

// ServiceType describes a type of provided service
type ServiceType struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
}

// Service holds a service definition
type Service struct {
	Type                ServiceType `json:"Type"`
	Name                string      `json:"Name"`
	Path                string      `json:"Path"`
	IsMonitored         bool        `json:"IsMonitored"`
	Properties          interface{} `json:"Properties"`
	Host                string      `json:"Host"`
	AdditionalEndpoints interface{} `json:"AdditionalEndpoints"`
}

// MeshProvider contains information about a mesh provider site
type MeshProvider struct {
	Name         string      `json:"Name"`
	FullName     string      `json:"FullName"`
	Organization string      `json:"Organization"`
	Domain       string      `json:"Domain"`
	Homepage     string      `json:"Homepage"`
	Email        string      `json:"Email"`
	Description  string      `json:"Description"`
	Services     []Service   `json:"Services"`
	Properties   interface{} `json:"Properties"`
}

// MentixResponse holds Mentix API response
type MentixResponse struct {
	Sites        []MeshProvider `json:"Sites"`
	ServiceTypes []ServiceType  `json:"ServiceTypes"`
}

// MentixErrorResponse holds Mentix API errors
type MentixErrorResponse struct {
	Message string `json:"error"`
}

// Response service response
type Response struct {
	Status        int             `json:"status"`
	StatusMessage string          `json:"message"`
	Providers     *[]MeshProvider `json:"providers"`
}
