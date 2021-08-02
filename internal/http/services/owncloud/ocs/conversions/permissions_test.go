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
	"testing"
)

func TestNewPermissions(t *testing.T) {
	for val := int(PermissionRead); val <= int(PermissionAll); val++ {
		_, err := NewPermissions(val)
		if err != nil {
			t.Errorf("value %d should be a valid permissions", val)
		}
	}
}

func TestNewPermissionsWithInvalidValueShouldFail(t *testing.T) {
	vals := []int{
		int(PermissionInvalid),
		-1,
		int(PermissionAll) + 1,
	}
	for _, v := range vals {
		_, err := NewPermissions(v)
		if err == nil {
			t.Errorf("value %d should not be a valid permission", v)
		}
	}
}

func TestContainPermissionAll(t *testing.T) {
	table := map[int]Permissions{
		1:  PermissionRead,
		2:  PermissionWrite,
		4:  PermissionCreate,
		8:  PermissionDelete,
		16: PermissionShare,
		31: PermissionAll,
	}

	p, _ := NewPermissions(31) // all permissions should contain all other permissions
	for _, value := range table {
		if !p.Contain(value) {
			t.Errorf("permissions %d should contain %d", p, value)
		}
	}
}
func TestContainPermissionRead(t *testing.T) {
	table := map[int]Permissions{
		2:  PermissionWrite,
		4:  PermissionCreate,
		8:  PermissionDelete,
		16: PermissionShare,
		31: PermissionAll,
	}

	p, _ := NewPermissions(1) // read permission should not contain any other permissions
	if !p.Contain(PermissionRead) {
		t.Errorf("permissions %d should contain %d", p, PermissionRead)
	}
	for _, value := range table {
		if p.Contain(value) {
			t.Errorf("permissions %d should not contain %d", p, value)
		}
	}
}

func TestContainPermissionCustom(t *testing.T) {
	table := map[int]Permissions{
		2:  PermissionWrite,
		8:  PermissionDelete,
		31: PermissionAll,
	}

	p, _ := NewPermissions(21) // read, create & share permission
	if !p.Contain(PermissionRead) {
		t.Errorf("permissions %d should contain %d", p, PermissionRead)
	}
	if !p.Contain(PermissionCreate) {
		t.Errorf("permissions %d should contain %d", p, PermissionCreate)
	}
	if !p.Contain(PermissionShare) {
		t.Errorf("permissions %d should contain %d", p, PermissionShare)
	}
	for _, value := range table {
		if p.Contain(value) {
			t.Errorf("permissions %d should not contain %d", p, value)
		}
	}
}

func TestContainWithMultiplePermissions(t *testing.T) {
	table := map[int][]Permissions{
		3: {
			PermissionRead,
			PermissionWrite,
		},
		5: {
			PermissionRead,
			PermissionCreate,
		},
		31: {
			PermissionRead,
			PermissionWrite,
			PermissionCreate,
			PermissionDelete,
			PermissionShare,
		},
	}

	for key, value := range table {
		p, _ := NewPermissions(key)
		for _, v := range value {
			if !p.Contain(v) {
				t.Errorf("permissions %d should contain %d", p, v)
			}
		}
	}
}

func TestPermissions2Role(t *testing.T) {
	checkRole := func(expected, actual string) {
		if actual != expected {
			t.Errorf("Expected role %s actually got %s", expected, actual)
		}
	}

	table := map[Permissions]string{
		PermissionRead: RoleViewer,
		PermissionRead | PermissionWrite | PermissionCreate | PermissionDelete: RoleEditor,
		PermissionAll:                     RoleCoowner,
		PermissionWrite:                   RoleLegacy,
		PermissionShare:                   RoleLegacy,
		PermissionWrite | PermissionShare: RoleLegacy,
	}

	for permissions, role := range table {
		actual := RoleFromOCSPermissions(permissions).Name
		checkRole(role, actual)
	}
}
