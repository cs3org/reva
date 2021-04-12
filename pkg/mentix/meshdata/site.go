// Copyright 2018-2021 CERN
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

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/accservice"
	"github.com/cs3org/reva/pkg/mentix/utils/network"
)

const (
	// SiteTypeScienceMesh flags a site as being part of the mesh.
	SiteTypeScienceMesh SiteType = iota
	// SiteTypeCommunity flags a site as being a community site.
	SiteTypeCommunity
)

// SiteType holds the type of a site.
type SiteType int

// Site represents a single site managed by Mentix.
type Site struct {
	// Internal settings
	Type         SiteType `json:"-"`
	IsAuthorized bool     `json:"-"`

	ID           string
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

// AddService adds a new service; if a service with the same name already exists, the existing one is overwritten.
func (site *Site) AddService(service *Service) {
	if serviceExisting := site.FindService(service.Name); serviceExisting != nil {
		*service = *serviceExisting
	} else {
		site.Services = append(site.Services, service)
	}
}

// RemoveService removes the provided service.
func (site *Site) RemoveService(name string) {
	if service := site.FindService(name); service != nil {
		for idx, serviceExisting := range site.Services {
			if serviceExisting == service {
				lastIdx := len(site.Services) - 1
				site.Services[idx] = site.Services[lastIdx]
				site.Services[lastIdx] = nil
				site.Services = site.Services[:lastIdx]
				break
			}
		}
	}
}

// FindService searches for a service with the given name.
func (site *Site) FindService(name string) *Service {
	for _, service := range site.Services {
		if strings.EqualFold(service.Name, name) {
			return service
		}
	}
	return nil
}

// Verify checks if the site data is valid.
func (site *Site) Verify() error {
	// Verify data
	if site.Name == "" {
		return fmt.Errorf("site name missing")
	}
	if site.Domain == "" && site.Homepage == "" {
		return fmt.Errorf("site URL missing")
	}

	// Verify services
	for _, service := range site.Services {
		if err := service.Verify(); err != nil {
			return err
		}
	}

	return nil
}

// InferMissingData infers missing data from other data where possible.
func (site *Site) InferMissingData() {
	// Infer missing data
	site.IsAuthorized = site.getAuthorizationStatus()

	if site.Homepage == "" {
		site.Homepage = fmt.Sprintf("http://www.%v", site.Domain)
	} else if site.Domain == "" {
		if URL, err := url.Parse(site.Homepage); err == nil {
			site.Domain = network.ExtractDomainFromURL(URL, false)
		}
	}

	// Infer missing for services
	for _, service := range site.Services {
		service.InferMissingData()
	}
}

func (site *Site) getAuthorizationStatus() bool {
	// ScienceMesh sites are always authorized
	if site.Type == SiteTypeScienceMesh {
		return true
	}

	// Use the accounts service to find out whether the site is authorized
	resp, err := accservice.Query("is-authorized", network.URLParams{"by": "siteid", "value": site.ID})
	if err == nil && resp.Success {
		if authorized, ok := resp.Data.(bool); ok {
			return authorized
		}
	}

	return false
}

// GetSiteTypeName returns the readable name of the given site type.
func GetSiteTypeName(siteType SiteType) string {
	switch siteType {
	case SiteTypeScienceMesh:
		return "sciencemesh"

	case SiteTypeCommunity:
		return "community"

	default:
		return "unknown"
	}
}
