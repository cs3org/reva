// Copyright 2018-2023 CERN
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

import "strings"

const (
	// PropertyOperator identifies the operator property.
	PropertyOperator = "operator"
	// PropertySiteID identifies the site ID property.
	PropertySiteID = "site_id"
	// PropertyOrganization identifies the organization property.
	PropertyOrganization = "organization"

	// PropertyAPIVersion identifies the API version property.
	PropertyAPIVersion = "api_version"

	// PropertyInfrastructure identifies the infrastructure type of a site.
	PropertyInfrastructure = "infrastructure"
	// PropertyCertification identifies the certification status of a site.
	PropertyCertification = "certification"
	// PropertyIsInProduction identifies if a service is in production.
	PropertyIsInProduction = "in_production"
	// PropertyIsBeta identifies if a service is in beta.
	PropertyIsBeta = "beta"
)

// GetPropertyValue performs a case-insensitive search for the given property.
func GetPropertyValue(props map[string]string, id string, defValue string) string {
	for key := range props {
		if strings.EqualFold(key, id) {
			return props[key]
		}
	}

	return defValue
}

// SetPropertyValue sets a property value.
func SetPropertyValue(props *map[string]string, id string, value string) {
	// If the provided properties map is nil, create an empty one
	if *props == nil {
		*props = make(map[string]string)
	}

	(*props)[strings.ToUpper(id)] = value
}
