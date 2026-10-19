// Copyright 2018-2026 CERN
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

package spaces

import (
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"google.golang.org/protobuf/proto"
)

func TestMergeRolesWithPermissions(t *testing.T) {
	admin := &provider.ResourcePermissions{
		AddGrant: true, RemoveGrant: true, UpdateGrant: true, ListGrants: true, DenyGrant: true, GetQuota: true,
		ListFileVersions: true, RestoreFileVersion: true, Stat: true, InitiateFileUpload: true, Delete: true,
	}
	roles := []*provider.SpaceRole{
		{
			RoleName:      "admin",
			PermissionSet: admin,
			Recipients: []*provider.Grantee{{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
				Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: "project-admins"}},
			}},
		},
		{
			RoleName:      "owner",
			PermissionSet: admin,
			Recipients: []*provider.Grantee{{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{Idp: "cern", OpaqueId: "owner"}},
			}},
		},
	}
	spacePerms := &provider.ResourcePermissions{
		AddGrant: true, RemoveGrant: true, UpdateGrant: true, ListGrants: true, DenyGrant: true, GetQuota: true,
		ListFileVersions: true, RestoreFileVersion: true,
	}

	tests := []struct {
		description string
		user        *userpb.User
		expected    *provider.ResourcePermissions
	}{
		{
			description: "member of a recipient group",
			user:        &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "alice"}, Groups: []string{"other", "project-admins"}},
			expected:    spacePerms,
		},
		{
			description: "user recipient",
			user:        &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "owner"}},
			expected:    spacePerms,
		},
		{
			description: "neither a recipient nor in a recipient group",
			user:        &userpb.User{Id: &userpb.UserId{Idp: "cern", OpaqueId: "bob"}, Groups: []string{"other"}},
			expected:    &provider.ResourcePermissions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			perms := &provider.ResourcePermissions{}
			MergeRolesWithPermissions(perms, roles, tt.user)
			if !proto.Equal(perms, tt.expected) {
				t.Errorf("got %v, expected %v", perms, tt.expected)
			}
		})
	}
}
