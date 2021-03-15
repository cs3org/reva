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

// Package conversions sits between CS3 type definitions and OCS API Responses
package conversions

import (
	"fmt"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Role describes the interface to transform different permission sets into each other
type Role struct {
	Name                   string
	cS3ResourcePermissions *provider.ResourcePermissions
	ocsPermissions         Permissions
}

const (
	// RoleUnknown is used for unknown roles
	RoleUnknown string = "unknown"
	// RoleLegacy provides backwards compatibility
	RoleLegacy string = "legacy"
	// RoleViewer grants non-editor role on a resource
	RoleViewer string = "viewer"
	// RoleEditor grants editor permission on a resource, including folders
	RoleEditor string = "editor"
	// RoleFileEditor grants editor permission on a single file
	RoleFileEditor string = "file-editor"
	// RoleCoowner grants owner permissions on a resource
	RoleCoowner string = "coowner"
	// RoleUploader FIXME: uploader role with only write permission can use InitiateFileUpload, not anything else
	RoleUploader string = "uploader"
)

// CS3ResourcePermissions for the role
func (r *Role) CS3ResourcePermissions() *provider.ResourcePermissions {
	return r.cS3ResourcePermissions
}

// OCSPermissions for the role
func (r *Role) OCSPermissions() Permissions {
	return r.ocsPermissions
}

// WebDAVPermissions returns the webdav permissions used in propfinds, eg. "WCKDNVR"
/*
	from https://github.com/owncloud/core/blob/10715e2b1c85fc3855a38d2b1fe4426b5e3efbad/apps/dav/lib/Files/PublicFiles/SharedNodeTrait.php#L196-L215

		$p = '';
		if ($node->isDeletable() && $this->checkSharePermissions(Constants::PERMISSION_DELETE)) {
			$p .= 'D';
		}
		if ($node->isUpdateable() && $this->checkSharePermissions(Constants::PERMISSION_UPDATE)) {
			$p .= 'NV'; // Renameable, Moveable
		}
		if ($node->getType() === \OCP\Files\FileInfo::TYPE_FILE) {
			if ($node->isUpdateable() && $this->checkSharePermissions(Constants::PERMISSION_UPDATE)) {
				$p .= 'W';
			}
		} else {
			if ($node->isCreatable() && $this->checkSharePermissions(Constants::PERMISSION_CREATE)) {
				$p .= 'CK';
			}
		}

*/
// D = delete
// NV = update (renameable moveable)
// W = update (files only)
// CK = create (folders only)
// S = Shared
// R = Shareable
// M = Mounted
func (r *Role) WebDAVPermissions(isDir, isShared, isMountpoint, isPublic bool) string {
	var b strings.Builder
	// b.Grow(7)
	if !isPublic && isShared {
		fmt.Fprintf(&b, "S")
	}
	if r.ocsPermissions.Contain(PermissionShare) {
		fmt.Fprintf(&b, "R")
	}
	if !isPublic && isMountpoint {
		fmt.Fprintf(&b, "M")
	}
	if r.ocsPermissions.Contain(PermissionDelete) {
		fmt.Fprintf(&b, "D") // TODO oc10 shows received shares as deletable
	}
	if r.ocsPermissions.Contain(PermissionWrite) {
		fmt.Fprintf(&b, "NV")
		if !isDir {
			fmt.Fprintf(&b, "W")
		}
	}
	if isDir && r.ocsPermissions.Contain(PermissionCreate) {
		fmt.Fprintf(&b, "CK")
	}
	return b.String()
}

// RoleFromName creates a role from the name
func RoleFromName(name string) *Role {
	switch name {
	case RoleViewer:
		return NewViewerRole()
	case RoleEditor:
		return NewEditorRole()
	case RoleFileEditor:
		return NewFileEditorRole()
	case RoleCoowner:
		return NewCoownerRole()
	case RoleUploader:
		return NewUploaderRole()
	}
	return NewUnknownRole()
}

// NewUnknownRole creates an unknown role
func NewUnknownRole() *Role {
	return &Role{
		Name:                   RoleUnknown,
		cS3ResourcePermissions: &provider.ResourcePermissions{},
		ocsPermissions:         PermissionInvalid,
	}
}

// NewViewerRole creates a viewer role
func NewViewerRole() *Role {
	return &Role{
		Name: RoleViewer,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			// read
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			ListGrants:           true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
		},
		ocsPermissions: PermissionRead,
	}
}

// NewEditorRole creates an editor role
func NewEditorRole() *Role {
	return &Role{
		Name: RoleEditor,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			// read
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			ListGrants:           true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,

			// write
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,

			// create
			CreateContainer: true,

			// delete
			Delete: true,

			// not sure where to put these, but they are part of an editor
			Move:         true,
			PurgeRecycle: true,
		},
		ocsPermissions: PermissionRead | PermissionCreate | PermissionWrite | PermissionDelete,
	}
}

// NewFileEditorRole creates a file-editor role
func NewFileEditorRole() *Role {
	return &Role{
		Name: RoleEditor,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			// read
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			ListGrants:           true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,

			// write
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,
		},
		ocsPermissions: PermissionRead | PermissionWrite,
	}
}

// NewCoownerRole creates a coowner role
func NewCoownerRole() *Role {
	return &Role{
		Name: RoleCoowner,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			// read
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			ListGrants:           true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,

			// write
			InitiateFileUpload: true,
			RestoreFileVersion: true,
			RestoreRecycleItem: true,

			// create
			CreateContainer: true,

			// delete
			Delete: true,

			// not sure where to put these, but they are part of an editor
			Move:         true,
			PurgeRecycle: true,

			// grants
			AddGrant:    true,
			UpdateGrant: true,
			RemoveGrant: true,
		},
		ocsPermissions: PermissionAll,
	}
}

// NewUploaderRole creates an uploader role
func NewUploaderRole() *Role {
	return &Role{
		Name: RoleViewer,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			// he will need to make stat requests
			// TODO and List requests
			Stat:          true,
			ListContainer: true,
			// read
			GetPath: true,
			// mkdir
			CreateContainer: true,
			// upload
			InitiateFileUpload: true,
		},
		ocsPermissions: PermissionCreate,
	}
}

// RoleFromOCSPermissions tries to map ocs permissions to a role
func RoleFromOCSPermissions(p Permissions) *Role {
	if p.Contain(PermissionRead) {
		if p.Contain(PermissionWrite) && p.Contain(PermissionCreate) && p.Contain(PermissionDelete) {
			if p.Contain(PermissionShare) {
				return NewCoownerRole()
			}
			return NewEditorRole()
		}
		if p == PermissionRead {
			return NewViewerRole()
		}
	}
	if p == PermissionCreate {
		return NewUploaderRole()
	}
	// legacy
	return NewLegacyRoleFromOCSPermissions(p)
}

// NewLegacyRoleFromOCSPermissions tries to map a legacy combination of ocs permissions to cs3 resource permissions as a legacy role
func NewLegacyRoleFromOCSPermissions(p Permissions) *Role {
	r := &Role{
		Name:                   RoleLegacy, // TODO custom role?
		ocsPermissions:         p,
		cS3ResourcePermissions: &provider.ResourcePermissions{},
	}
	if p.Contain(PermissionRead) {
		r.cS3ResourcePermissions.ListContainer = true
		r.cS3ResourcePermissions.ListGrants = true
		r.cS3ResourcePermissions.ListFileVersions = true
		r.cS3ResourcePermissions.ListRecycle = true
		r.cS3ResourcePermissions.Stat = true
		r.cS3ResourcePermissions.GetPath = true
		r.cS3ResourcePermissions.GetQuota = true
		r.cS3ResourcePermissions.InitiateFileDownload = true
	}
	if p.Contain(PermissionWrite) {
		r.cS3ResourcePermissions.InitiateFileUpload = true
		r.cS3ResourcePermissions.RestoreFileVersion = true
		r.cS3ResourcePermissions.RestoreRecycleItem = true
	}
	if p.Contain(PermissionCreate) {
		r.cS3ResourcePermissions.Stat = true
		r.cS3ResourcePermissions.ListContainer = true
		r.cS3ResourcePermissions.CreateContainer = true
		// FIXME permissions mismatch: double check ocs create vs update file
		// - if the file exists the ocs api needs to check update permission,
		// - if the file does not exist  the ocs api needs to check update permission
		r.cS3ResourcePermissions.InitiateFileUpload = true
		if p.Contain(PermissionWrite) {
			r.cS3ResourcePermissions.Move = true // TODO move only when create and write?
		}
	}
	if p.Contain(PermissionDelete) {
		r.cS3ResourcePermissions.Delete = true
		r.cS3ResourcePermissions.PurgeRecycle = true
	}
	if p.Contain(PermissionShare) {
		r.cS3ResourcePermissions.AddGrant = true
		r.cS3ResourcePermissions.RemoveGrant = true // TODO when are you able to unshare / delete
		r.cS3ResourcePermissions.UpdateGrant = true
	}
	return r
}

// RoleFromResourcePermissions tries to map cs3 resource permissions to a role
func RoleFromResourcePermissions(rp *provider.ResourcePermissions) *Role {
	r := &Role{
		Name:                   RoleUnknown,
		ocsPermissions:         PermissionInvalid,
		cS3ResourcePermissions: rp,
	}
	if rp == nil {
		return r
	}
	if rp.ListContainer &&
		rp.ListGrants &&
		rp.ListFileVersions &&
		rp.ListRecycle &&
		rp.Stat &&
		rp.GetPath &&
		rp.GetQuota &&
		rp.InitiateFileDownload {
		r.ocsPermissions |= PermissionRead
	}
	if rp.InitiateFileUpload &&
		rp.RestoreFileVersion &&
		rp.RestoreRecycleItem {
		r.ocsPermissions |= PermissionWrite
	}
	if rp.ListContainer &&
		rp.Stat &&
		rp.CreateContainer &&
		rp.InitiateFileUpload {
		r.ocsPermissions |= PermissionCreate
	}
	if rp.Delete &&
		rp.PurgeRecycle {
		r.ocsPermissions |= PermissionDelete
	}
	if rp.AddGrant &&
		rp.RemoveGrant &&
		rp.UpdateGrant {
		r.ocsPermissions |= PermissionShare
	}
	if r.ocsPermissions.Contain(PermissionRead) {
		if r.ocsPermissions.Contain(PermissionWrite) && r.ocsPermissions.Contain(PermissionCreate) && r.ocsPermissions.Contain(PermissionDelete) {
			r.Name = RoleEditor
			if r.ocsPermissions.Contain(PermissionShare) {
				r.Name = RoleCoowner
			}
			return r // editor or coowner
		}
		if r.ocsPermissions == PermissionRead {
			r.Name = RoleViewer
			return r
		}
	}
	if r.ocsPermissions == PermissionCreate {
		r.Name = RoleUploader
		return r
	}
	r.Name = RoleLegacy
	// at this point other ocs permissions may have been mapped.
	// TODO what about even more granular cs3 permissions?, eg. only stat
	return r
}
