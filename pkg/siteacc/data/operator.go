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

// Operator represents the global operator-specific settings stored in the service.
type Operator struct {
	ID string `json:"id"`

	Sites []*Site `json:"sites"`
}

// Operators holds an array of operators.
type Operators = []*Operator

// Update copies the data of the given operator to this operator.
func (op *Operator) Update(other *Operator) error {
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
