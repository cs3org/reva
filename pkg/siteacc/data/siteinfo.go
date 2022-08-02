// Copyright 2018-2022 CERN
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

package data

import (
	"github.com/pkg/errors"
)

// SiteInformation holds the most basic information about a sites.
type SiteInformation struct {
	ID       string
	Name     string
	FullName string
}

// QuerySiteName uses Mentix to query the name of a sites given by its ID.
func QuerySiteName(siteID string, fullName bool, mentixHost, dataEndpoint string) (string, error) {
	ops, err := QueryAvailableOperators(mentixHost, dataEndpoint)
	if err != nil {
		return "", err
	}

	for _, op := range ops {
		for _, site := range op.Sites {
			if site.ID == siteID {
				if fullName {
					return site.FullName, nil
				}
				return site.Name, nil
			}
		}
	}

	return "", errors.Errorf("no sites with ID %v found", siteID)
}
