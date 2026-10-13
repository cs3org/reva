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
	"slices"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/utils"
)

// MergeRolesWithPermissions adds to perms what the space roles give to the
// user, either directly or through one of their groups.
func MergeRolesWithPermissions(perms *provider.ResourcePermissions, roles []*provider.SpaceRole, user *userpb.User) {
	for _, role := range roles {
		for _, recipient := range role.GetRecipients() {
			userID, groupID := utils.ExtractGranteeID(recipient)
			if utils.UserEqual(userID, user.GetId()) || (groupID != nil && slices.Contains(user.GetGroups(), groupID.GetOpaqueId())) {
				mergeRolePermissions(perms, role.GetPermissionSet())
				break
			}
		}
	}
}

// mergeRolePermissions only takes the permissions that the storage cannot
// hold: the storage already knows who may read or write the files.
func mergeRolePermissions(perms, rolePerms *provider.ResourcePermissions) {
	perms.AddGrant = perms.AddGrant || rolePerms.GetAddGrant()
	perms.RemoveGrant = perms.RemoveGrant || rolePerms.GetRemoveGrant()
	perms.UpdateGrant = perms.UpdateGrant || rolePerms.GetUpdateGrant()
	perms.ListGrants = perms.ListGrants || rolePerms.GetListGrants()
	perms.DenyGrant = perms.DenyGrant || rolePerms.GetDenyGrant()
	perms.GetQuota = perms.GetQuota || rolePerms.GetGetQuota()
	perms.ListFileVersions = perms.ListFileVersions || rolePerms.GetListFileVersions()
	perms.RestoreFileVersion = perms.RestoreFileVersion || rolePerms.GetRestoreFileVersion()
}
