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
	"strings"
)

// Operator represents a complete operator including its sites managed by Mentix.
type Operator struct {
	ID            string
	Name          string
	Homepage      string
	Email         string
	HelpdeskEmail string
	SecurityEmail string

	Sites      []*Site
	Properties map[string]string
}

// AddSite adds a new site; if a site with the same ID already exists, the existing one is overwritten.
func (op *Operator) AddSite(site *Site) {
	if siteExisting := op.FindSite(site.ID); siteExisting != nil {
		*siteExisting = *site
	} else {
		op.Sites = append(op.Sites, site)
	}
}

// RemoveSite removes the provided site.
func (op *Operator) RemoveSite(id string) {
	if site := op.FindSite(id); site != nil {
		for idx, siteExisting := range op.Sites {
			if siteExisting == site {
				lastIdx := len(op.Sites) - 1
				op.Sites[idx] = op.Sites[lastIdx]
				op.Sites[lastIdx] = nil
				op.Sites = op.Sites[:lastIdx]
				break
			}
		}
	}
}

// FindSite searches for a site with the given ID.
func (op *Operator) FindSite(id string) *Site {
	for _, site := range op.Sites {
		if strings.EqualFold(site.ID, id) {
			return site
		}
	}
	return nil
}

// Verify checks if the operator data is valid.
func (op *Operator) Verify() error {
	// Verify data
	if op.Name == "" {
		return fmt.Errorf("operator name missing")
	}
	if op.Email == "" {
		return fmt.Errorf("operator email missing")
	}

	// Verify sites
	for _, site := range op.Sites {
		if err := site.Verify(); err != nil {
			return err
		}
	}

	return nil
}

// InferMissingData infers missing data from other data where possible.
func (op *Operator) InferMissingData() {
	// Infer missing data
	if op.Name == "" {
		op.Name = op.ID
	}
	if op.HelpdeskEmail == "" {
		op.HelpdeskEmail = op.Email
	}
	if op.SecurityEmail == "" {
		op.SecurityEmail = op.Email
	}

	// Infer missing for sites
	for _, site := range op.Sites {
		site.InferMissingData()
	}
}
