// Copyright 2020 CERN
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

package conversions

import (
	"fmt"
)

// Permissions reflects the CRUD permissions used in the OCS sharing API
type Permissions uint

const (
	// PermissionInvalid represents an invalid permission
	PermissionInvalid Permissions = 0
	// PermissionRead grants read permissions on a resource
	PermissionRead Permissions = 1 << (iota - 1)
	// PermissionWrite grants write permissions on a resource
	PermissionWrite
	// PermissionCreate grants create permissions on a resource
	PermissionCreate
	// PermissionDelete grants delete permissions on a resource
	PermissionDelete
	// PermissionShare grants share permissions on a resource
	PermissionShare
	// PermissionDeny grants permissions to deny access on a resource
	// The recipient of the resource will then have PermissionNone.
	PermissionDeny
	// PermissionNone grants no permissions on a resource
	PermissionNone
	// PermissionMax is to be used within value range checks
	PermissionMax Permissions = (1 << (iota - 1)) - 1
	// PermissionAll grants all permissions on a resource
	PermissionAll = PermissionMax - PermissionNone
	// PermissionMin is to be used within value range checks
	PermissionMin = PermissionRead
)

var (
	// ErrPermissionNotInRange defines a permission specific error.
	ErrPermissionNotInRange = fmt.Errorf("The provided permission is not between %d and %d", PermissionMin, PermissionMax)
)

// NewPermissions creates a new Permissions instance.
// The value must be in the valid range.
func NewPermissions(val int) (Permissions, error) {
	if val == int(PermissionInvalid) {
		return PermissionInvalid, fmt.Errorf("permissions %d out of range %d - %d", val, PermissionMin, PermissionMax)
	} else if val < int(PermissionInvalid) || int(PermissionMax) < val {
		return PermissionInvalid, ErrPermissionNotInRange
	}
	return Permissions(val), nil
}

// Contain tests if the permissions contain another one.
func (p Permissions) Contain(other Permissions) bool {
	return p&other == other
}
