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

package utils

import (
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	"github.com/google/go-cmp/cmp"
)

// NewEmptyGroup creates an empty group
func NewEmptyGroup() *grouppb.Group {
	return &grouppb.Group{}
}

// IsEmptyGroup returns true if the group is empty
func IsEmptyGroup(group *grouppb.Group) bool {
	return cmp.Equal(group, NewEmptyGroup())
}
