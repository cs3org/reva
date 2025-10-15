// Copyright 2018-2024 CERN
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
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
)

// Role is a set of ocs permissions and cs3 resource permissions under a common name.
type Role struct {
	Name                   string
	cS3ResourcePermissions *provider.ResourcePermissions
	ocsPermissions         Permissions
}

const (
	// RoleViewer grants non-editor role on a resource.
	RoleViewer = "viewer"
	// RoleReader grants non-editor role on a resource.
	RoleReader = "reader"
	// RoleEditor grants editor permission on a resource, including folders.
	RoleEditor = "editor"
	// RoleFileEditor grants editor permission on a single file.
	RoleFileEditor = "file-editor"
	// RoleUploader grants uploader permission to upload onto a resource.
	RoleUploader = "uploader"
	// RoleManager grants manager permissions on a resource. Semantically equivalent to co-owner.
	RoleManager = "manager"
	// RoleUnknown is used for unknown roles.
	RoleUnknown = "unknown"
	// RoleLegacy provides backwards compatibility.
	RoleLegacy = "legacy"
	// RoleDenied grants no permission at all on a resource.
	RoleDenied = "denied"
)

// CS3ResourcePermissions for the role.
func (r *Role) CS3ResourcePermissions() *provider.ResourcePermissions {
	return r.cS3ResourcePermissions
}

// OCSPermissions for the role.
func (r *Role) OCSPermissions() Permissions {
	return r.ocsPermissions
}

// WebDAVPermissions returns the webdav permissions used in propfinds, eg. "WCKDNVR"
// D = delete
// NV = update (renameable moveable)
// W = update (files only)
// CK = create (folders only)
// S = Shared
// R = Shareable
// M = Mounted
// Z = Deniable
// O = Openable.
func (r *Role) WebDAVPermissions(isDir, isShared, isMountpoint, isPublic, isOpenable bool) string {
	var b strings.Builder
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

	if r.ocsPermissions.Contain(PermissionDeny) && !isPublic {
		fmt.Fprintf(&b, "Z")
	}

	if isOpenable && !isDir {
		fmt.Fprintf(&b, "O")
	}

	return b.String()
}

// RoleFromName creates a role from the name.
func RoleFromName(name string) *Role {
	switch name {
	case RoleDenied:
		return NewDeniedRole()
	case RoleViewer:
		return NewViewerRole()
	case RoleReader:
		return NewViewerRole()
	case RoleEditor:
		return NewEditorRole()
	case RoleFileEditor:
		return NewFileEditorRole()
	case RoleUploader:
		return NewUploaderRole()
	case RoleManager:
		return NewManagerRole()
	default:
		return NewUnknownRole()
	}
}

// NewUnknownRole creates an unknown role. An Unknown role has no permissions over a cs3 resource nor any ocs endpoint.
func NewUnknownRole() *Role {
	return &Role{
		Name:                   RoleUnknown,
		cS3ResourcePermissions: &provider.ResourcePermissions{},
		ocsPermissions:         PermissionInvalid,
	}
}

// NewDeniedRole creates a fully denied role.
func NewDeniedRole() *Role {
	return &Role{
		Name:                   RoleDenied,
		cS3ResourcePermissions: &provider.ResourcePermissions{},
		ocsPermissions:         PermissionNone,
	}
}

// NewViewerRole creates a viewer role.
func NewViewerRole() *Role {
	return &Role{
		Name: RoleViewer,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			GetPath:              true,
			InitiateFileDownload: true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
		},
		ocsPermissions: PermissionRead,
	}
}

// NewEditorRole creates an editor role.
func NewEditorRole() *Role {
	return &Role{
		Name: RoleEditor,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			GetPath:              true,
			InitiateFileDownload: true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			InitiateFileUpload:   true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
			CreateContainer:      true,
			Delete:               true,
			Move:                 true,
			PurgeRecycle:         true,
		},
		ocsPermissions: PermissionRead | PermissionCreate | PermissionWrite | PermissionDelete,
	}
}

// NewFileEditorRole creates a file-editor role.
func NewFileEditorRole() *Role {
	return &Role{
		Name: RoleFileEditor,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			GetPath:              true,
			InitiateFileDownload: true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			InitiateFileUpload:   true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
		},
		ocsPermissions: PermissionRead | PermissionWrite,
	}
}

// NewUploaderRole creates an uploader role.
func NewUploaderRole() *Role {
	return &Role{
		Name: RoleUploader,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			Stat:               true,
			ListContainer:      true,
			GetPath:            true,
			InitiateFileUpload: true,
		},
		ocsPermissions: PermissionCreate,
	}
}

// NewManagerRole creates an editor role.
func NewManagerRole() *Role {
	return &Role{
		Name: RoleManager,
		cS3ResourcePermissions: &provider.ResourcePermissions{
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			ListGrants:           true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListRecycle:          true,
			Stat:                 true,
			InitiateFileUpload:   true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
			Move:                 true,
			CreateContainer:      true,
			Delete:               true,
			PurgeRecycle:         true,

			// these permissions only make sense to enforce them in the root of the storage space.
			AddGrant:    true, // managers can add users to the space
			RemoveGrant: true, // managers can remove users from the space
			UpdateGrant: true,
			DenyGrant:   true,
		},
		ocsPermissions: PermissionAll,
	}
}

// RoleFromOCSPermissions tries to map ocs permissions to a role.
func RoleFromOCSPermissions(p Permissions) *Role {
	if p.Contain(PermissionNone) {
		return NewDeniedRole()
	}
	if p.Contain(PermissionRead) {
		if p.Contain(PermissionWrite) && p.Contain(PermissionCreate) && p.Contain(PermissionDelete) {
			if p.Contain(PermissionShare) {
				return NewManagerRole()
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
	return NewDeniedRole()
}

// RoleFromResourcePermissions tries to map cs3 resource permissions to a role.
func RoleFromResourcePermissions(rp *provider.ResourcePermissions) *Role {
	r := &Role{
		Name:                   RoleUnknown,
		ocsPermissions:         PermissionInvalid,
		cS3ResourcePermissions: rp,
	}
	if rp == nil {
		return r
	}
	if grants.PermissionsEqual(rp, &provider.ResourcePermissions{}) {
		r.ocsPermissions = PermissionNone
		r.Name = RoleDenied
		return r
	}
	if rp.ListContainer &&
		rp.Stat &&
		rp.GetPath &&
		rp.InitiateFileDownload {
		r.ocsPermissions |= PermissionRead
	}
	if rp.InitiateFileUpload {
		r.ocsPermissions |= PermissionWrite
	}
	if rp.ListContainer &&
		rp.Stat &&
		rp.InitiateFileUpload {
		r.ocsPermissions |= PermissionCreate
	}
	if rp.Delete {
		r.ocsPermissions |= PermissionDelete
	}
	if rp.AddGrant &&
		rp.RemoveGrant &&
		rp.UpdateGrant {
		r.ocsPermissions |= PermissionShare
	}
	if rp.DenyGrant {
		r.ocsPermissions |= PermissionDeny
	}

	if r.ocsPermissions.Contain(PermissionRead) {
		if r.ocsPermissions.Contain(PermissionWrite) {
			r.Name = RoleFileEditor
			if r.ocsPermissions.Contain(PermissionCreate) && r.ocsPermissions.Contain(PermissionDelete) {
				r.Name = RoleEditor
				if r.ocsPermissions.Contain(PermissionShare) {
					r.Name = RoleManager
				}
			}
			return r // file-editor, editor or collaborator
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
