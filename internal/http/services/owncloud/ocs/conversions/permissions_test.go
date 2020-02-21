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

func TestContain(t *testing.T) {
	table := map[int]Permissions{
		1:  PermissionRead,
		2:  PermissionWrite,
		4:  PermissionCreate,
		8:  PermissionDelete,
		16: PermissionShare,
		31: PermissionAll,
	}

	for key, value := range table {
		p, _ := NewPermissions(key)
		if !p.Contain(value) {
			t.Errorf("permissions %d should contain %d", p, value)
		}
	}
}

func TestContainWithMultiplePermissions(t *testing.T) {
	table := map[int][]Permissions{
		3: []Permissions{
			PermissionRead,
			PermissionWrite,
		},
		5: []Permissions{
			PermissionRead,
			PermissionCreate,
		},
		31: []Permissions{
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
		PermissionRead:                    RoleViewer,
		PermissionWrite:                   RoleEditor,
		PermissionShare:                   RoleCoowner,
		PermissionAll:                     RoleCoowner,
		PermissionRead | PermissionWrite:  RoleEditor,
		PermissionWrite | PermissionShare: RoleCoowner,
	}

	for permissions, role := range table {
		actual := Permissions2Role(permissions)
		checkRole(role, actual)
	}
}
