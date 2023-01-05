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

package data

import (
	"github.com/pkg/errors"
)

// Operator represents the global operator-specific settings stored in the service.
type Operator struct {
	ID string `json:"id"`

	Sites []*Site `json:"sites"`
}

// Operators holds an array of operators.
type Operators = []*Operator

// Update copies the data of the given operator to this operator.
func (op *Operator) Update(other *Operator, credsPassphrase string) error {
	// Clear currently stored sites and clone over the new ones
	op.Sites = make([]*Site, 0, len(other.Sites))
	for _, otherSite := range other.Sites {
		site := otherSite.Clone(true)
		if err := site.Update(otherSite, credsPassphrase); err != nil {
			return errors.Wrapf(err, "unable to update site %v", site.ID)
		}
		op.Sites = append(op.Sites, site)
	}
	return nil
}

// Clone creates a copy of the operator; if eraseCredentials is set to true, the (test user) credentials will be cleared in the cloned object.
func (op *Operator) Clone(eraseCredentials bool) *Operator {
	clone := &Operator{
		ID:    op.ID,
		Sites: []*Site{},
	}

	// Clone sites
	for _, site := range op.Sites {
		clone.Sites = append(clone.Sites, site.Clone(eraseCredentials))
	}

	return clone
}

// NewOperator creates a new operator.
func NewOperator(id string) (*Operator, error) {
	op := &Operator{
		ID:    id,
		Sites: []*Site{},
	}
	return op, nil
}
