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

const (
	// SiteTypeScienceMesh flags a site as being part of the mesh.
	SiteTypeScienceMesh = iota
	// SiteTypeCommunity flags a site as being a community site.
	SiteTypeCommunity
)

type SiteType int

// Site represents a single site managed by Mentix.
type Site struct {
	Type SiteType `json:"-"`

	Name         string
	FullName     string
	Organization string
	Domain       string
	Homepage     string
	Email        string
	Description  string
	Country      string
	CountryCode  string
	Location     string
	Latitude     float32
	Longitude    float32

	Services   []*Service
	Properties map[string]string
}
