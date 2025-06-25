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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"context"
	"errors"
	"slices"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	libregraph "github.com/owncloud/libre-graph-api-go"
	"google.golang.org/protobuf/proto"
)

const (
	// UnifiedRoleViewerID Unified role viewer id.
	UnifiedRoleViewerID = "b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"
	// UnifiedRoleSpaceViewerID Unified role space viewer id.
	UnifiedRoleSpaceViewerID = "a8d5fe5e-96e3-418d-825b-534dbdf22b99"
	// UnifiedRoleEditorID Unified role editor id.
	UnifiedRoleEditorID = "fb6c3e19-e378-47e5-b277-9732f9de6e21"
	// UnifiedRoleSpaceEditorID Unified role space editor id.
	UnifiedRoleSpaceEditorID = "58c63c02-1d89-4572-916a-870abc5a1b7d"
	// UnifiedRoleFileEditorID Unified role file editor id.
	UnifiedRoleFileEditorID = "2d00ce52-1fc2-4dbc-8b95-a73b73395f5a"
	// UnifiedRoleEditorLiteID Unified role editor-lite id.
	UnifiedRoleEditorLiteID = "1c996275-f1c9-4e71-abdf-a42f6495e960"
	// UnifiedRoleManagerID Unified role manager id.
	UnifiedRoleManagerID = "312c0871-5ef7-4b3a-85b6-0e4074c64049"
	// UnifiedRoleSecureViewerID Unified role secure viewer id.
	UnifiedRoleSecureViewerID = "aa97fe03-7980-45ac-9e50-b325749fd7e6"

	// UnifiedRoleConditionDrive defines constraint that matches a Driveroot/Spaceroot
	UnifiedRoleConditionDrive = "exists @Resource.Root"
	// UnifiedRoleConditionFolder defines constraints that matches a DriveItem representing a Folder
	UnifiedRoleConditionFolder = "exists @Resource.Folder"
	// UnifiedRoleConditionFile defines a constraint that matches a DriveItem representing a File
	UnifiedRoleConditionFile = "exists @Resource.File"

	DriveItemPermissionsCreate = "libre.graph/driveItem/permissions/create"
	DriveItemChildrenCreate    = "libre.graph/driveItem/children/create"
	DriveItemStandardDelete    = "libre.graph/driveItem/standard/delete"
	DriveItemPathRead          = "libre.graph/driveItem/path/read"
	DriveItemQuotaRead         = "libre.graph/driveItem/quota/read"
	DriveItemContentRead       = "libre.graph/driveItem/content/read"
	DriveItemUploadCreate      = "libre.graph/driveItem/upload/create"
	DriveItemPermissionsRead   = "libre.graph/driveItem/permissions/read"
	DriveItemChildrenRead      = "libre.graph/driveItem/children/read"
	DriveItemVersionsRead      = "libre.graph/driveItem/versions/read"
	DriveItemDeletedRead       = "libre.graph/driveItem/deleted/read"
	DriveItemPathUpdate        = "libre.graph/driveItem/path/update"
	DriveItemPermissionsDelete = "libre.graph/driveItem/permissions/delete"
	DriveItemDeletedDelete     = "libre.graph/driveItem/deleted/delete"
	DriveItemVersionsUpdate    = "libre.graph/driveItem/versions/update"
	DriveItemDeletedUpdate     = "libre.graph/driveItem/deleted/update"
	DriveItemBasicRead         = "libre.graph/driveItem/basic/read"
	DriveItemPermissionsUpdate = "libre.graph/driveItem/permissions/update"
	DriveItemPermissionsDeny   = "libre.graph/driveItem/permissions/deny"
)

var legacyNames map[string]string = map[string]string{
	UnifiedRoleViewerID: conversions.RoleViewer,
	// in the V1 api the "spaceviewer" role was call "viewer" and the "spaceeditor" was "editor",
	// we need to stay compatible with that
	UnifiedRoleSpaceViewerID: "viewer",
	UnifiedRoleSpaceEditorID: "editor",
	UnifiedRoleEditorID:      conversions.RoleEditor,
	UnifiedRoleFileEditorID:  conversions.RoleFileEditor,
	// UnifiedRoleEditorLiteID:   conversions.RoleEditorLite,
	UnifiedRoleManagerID: conversions.RoleManager,
}

// NewViewerUnifiedRole creates a viewer role.
func NewViewerUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewViewerRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleViewerID),
		Description: proto.String("View and download."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionFile),
			},
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionFolder),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewSpaceViewerUnifiedRole creates a spaceviewer role
func NewSpaceViewerUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewViewerRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleSpaceViewerID),
		Description: proto.String("View and download."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionDrive),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewEditorUnifiedRole creates an editor role.
func NewEditorUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewEditorRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleEditorID),
		Description: proto.String("View, download, upload, edit, add and delete."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionFolder),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewSpaceEditorUnifiedRole creates an editor role
func NewSpaceEditorUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewEditorRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleSpaceEditorID),
		Description: proto.String("View, download, upload, edit, add and delete."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionDrive),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewFileEditorUnifiedRole creates a file-editor role
func NewFileEditorUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewFileEditorRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleFileEditorID),
		Description: proto.String("View, download and edit."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionFile),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewManagerUnifiedRole creates a manager role
func NewManagerUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewManagerRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleManagerID),
		Description: proto.String("View, download, upload, edit, add, delete and manage members."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionDrive),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewUploaderUnifiedRole creates an uploader role
func NewUploaderUnifiedRole() *libregraph.UnifiedRoleDefinition {
	r := conversions.NewUploaderRole()
	return &libregraph.UnifiedRoleDefinition{
		Id:          proto.String(UnifiedRoleManagerID),
		Description: proto.String("Upload only."),
		DisplayName: displayName(r),
		RolePermissions: []libregraph.UnifiedRolePermission{
			{
				AllowedResourceActions: convert(r),
				Condition:              proto.String(UnifiedRoleConditionDrive),
			},
		},
		LibreGraphWeight: proto.Int32(0),
	}
}

// NewUnifiedRoleFromID returns a unified role definition from the provided id
func NewUnifiedRoleFromID(id string) (*libregraph.UnifiedRoleDefinition, error) {
	for _, definition := range GetBuiltinRoleDefinitionList() {
		if definition.GetId() != id {
			continue
		}

		return definition, nil
	}

	return nil, errors.New("role not found")
}

// GetApplicableRoleDefinitionsForActions returns a list of role definitions
// that match the provided actions and constraints
func GetApplicableRoleDefinitionsForActions(actions []string) []*libregraph.UnifiedRoleDefinition {
	builtin := GetBuiltinRoleDefinitionList()
	definitions := make([]*libregraph.UnifiedRoleDefinition, 0, len(builtin))

	for _, definition := range builtin {
		var definitionMatch bool

		for _, permission := range definition.GetRolePermissions() {

			for i, action := range permission.GetAllowedResourceActions() {
				if !slices.Contains(actions, action) {
					break
				}
				if i == len(permission.GetAllowedResourceActions())-1 {
					definitionMatch = true
				}
			}

			if definitionMatch {
				break
			}
		}

		if definitionMatch {
			definitions = append(definitions, definition)
		}

	}

	return definitions
}

// PermissionsToCS3ResourcePermissions converts the provided libregraph UnifiedRolePermissions to a cs3 ResourcePermissions
func PermissionsToCS3ResourcePermissions(unifiedRolePermissions []libregraph.UnifiedRolePermission) *provider.ResourcePermissions {
	p := &provider.ResourcePermissions{}

	for _, permission := range unifiedRolePermissions {
		for _, allowedResourceAction := range permission.AllowedResourceActions {
			switch allowedResourceAction {
			case DriveItemPermissionsCreate:
				p.AddGrant = true
			case DriveItemChildrenCreate:
				p.CreateContainer = true
			case DriveItemStandardDelete:
				p.Delete = true
			case DriveItemPathRead:
				p.GetPath = true
			case DriveItemQuotaRead:
				p.GetQuota = true
			case DriveItemContentRead:
				p.InitiateFileDownload = true
			case DriveItemUploadCreate:
				p.InitiateFileUpload = true
			case DriveItemPermissionsRead:
				p.ListGrants = true
			case DriveItemChildrenRead:
				p.ListContainer = true
			case DriveItemVersionsRead:
				p.ListFileVersions = true
			case DriveItemDeletedRead:
				p.ListRecycle = true
			case DriveItemPathUpdate:
				p.Move = true
			case DriveItemPermissionsDelete:
				p.RemoveGrant = true
			case DriveItemDeletedDelete:
				p.PurgeRecycle = true
			case DriveItemVersionsUpdate:
				p.RestoreFileVersion = true
			case DriveItemDeletedUpdate:
				p.RestoreRecycleItem = true
			case DriveItemBasicRead:
				p.Stat = true
			case DriveItemPermissionsUpdate:
				p.UpdateGrant = true
			case DriveItemPermissionsDeny:
				p.DenyGrant = true
			}
		}
	}

	return p
}

// CS3ResourcePermissionsToLibregraphActions converts the provided cs3 ResourcePermissions to a list of
// libregraph actions
func CS3ResourcePermissionsToLibregraphActions(p *provider.ResourcePermissions) (actions []string) {
	if p.GetAddGrant() {
		actions = append(actions, DriveItemPermissionsCreate)
	}
	if p.GetCreateContainer() {
		actions = append(actions, DriveItemChildrenCreate)
	}
	if p.GetDelete() {
		actions = append(actions, DriveItemStandardDelete)
	}
	if p.GetGetPath() {
		actions = append(actions, DriveItemPathRead)
	}
	if p.GetGetQuota() {
		actions = append(actions, DriveItemQuotaRead)
	}
	if p.GetInitiateFileDownload() {
		actions = append(actions, DriveItemContentRead)
	}
	if p.GetInitiateFileUpload() {
		actions = append(actions, DriveItemUploadCreate)
	}
	if p.GetListGrants() {
		actions = append(actions, DriveItemPermissionsRead)
	}
	if p.GetListContainer() {
		actions = append(actions, DriveItemChildrenRead)
	}
	if p.GetListFileVersions() {
		actions = append(actions, DriveItemVersionsRead)
	}
	if p.GetListRecycle() {
		actions = append(actions, DriveItemDeletedRead)
	}
	if p.GetMove() {
		actions = append(actions, DriveItemPathUpdate)
	}
	if p.GetRemoveGrant() {
		actions = append(actions, DriveItemPermissionsDelete)
	}
	if p.GetPurgeRecycle() {
		actions = append(actions, DriveItemDeletedDelete)
	}
	if p.GetRestoreFileVersion() {
		actions = append(actions, DriveItemVersionsUpdate)
	}
	if p.GetRestoreRecycleItem() {
		actions = append(actions, DriveItemDeletedUpdate)
	}
	if p.GetStat() {
		actions = append(actions, DriveItemBasicRead)
	}
	if p.GetUpdateGrant() {
		actions = append(actions, DriveItemPermissionsUpdate)
	}
	if p.GetDenyGrant() {
		actions = append(actions, DriveItemPermissionsDeny)
	}
	return actions
}

func GetLegacyName(role libregraph.UnifiedRoleDefinition) string {
	return legacyNames[role.GetId()]
}

// CS3ResourcePermissionsToUnifiedRole tries to find the UnifiedRoleDefinition that matches the supplied
// CS3 ResourcePermissions.
func CS3ResourcePermissionsToUnifiedRole(ctx context.Context, p *provider.ResourcePermissions) *libregraph.UnifiedRoleDefinition {
	log := appctx.GetLogger(ctx)
	role := conversions.RoleFromResourcePermissions(p)
	log.Debug().Interface("role", role).Interface("perms", p).Msg("Converting cs3 resource permissions to unified role")
	return ocsRoleUnifiedRole[role.Name]
}

func displayName(role *conversions.Role) *string {
	if role == nil {
		return nil
	}

	// linter wants this to be a var
	canEdit := "Can edit"

	var displayName string
	switch role.Name {
	case conversions.RoleViewer:
		displayName = "Can view"
	case conversions.RoleEditor:
		displayName = canEdit
	case conversions.RoleFileEditor:
		displayName = canEdit
	case conversions.RoleManager:
		displayName = "Can manage"
	default:
		return nil
	}
	return proto.String(displayName)
}

func convert(role *conversions.Role) []string {
	actions := make([]string, 0, 8)
	if role == nil && role.CS3ResourcePermissions() == nil {
		return actions
	}
	return CS3ResourcePermissionsToLibregraphActions(role.CS3ResourcePermissions())
}

func GetAllowedResourceActions(role *libregraph.UnifiedRoleDefinition, condition string) []string {
	for _, p := range role.GetRolePermissions() {
		if p.GetCondition() == condition {
			return p.GetAllowedResourceActions()
		}
	}
	return []string{}
}

func GetBuiltinRoleDefinitionList() []*libregraph.UnifiedRoleDefinition {
	return []*libregraph.UnifiedRoleDefinition{
		NewViewerUnifiedRole(),
		NewEditorUnifiedRole(),

		// We currently don't support these roles (e.g.
		// the manager role supposes you can add members to a folder,
		// which is a concept we don't have at the moment)
		// Since this function is used to tell the front-end which
		// roles are supported, we have commented them out for the time being

		//NewFileEditorUnifiedRole(),
		//NewManagerUnifiedRole(),
	}
}

var ocsRoleUnifiedRole = map[string]*libregraph.UnifiedRoleDefinition{
	conversions.RoleViewer:       NewViewerUnifiedRole(),
	conversions.RoleReader:       NewViewerUnifiedRole(),
	conversions.RoleEditor:       NewEditorUnifiedRole(),
	conversions.RoleFileEditor:   NewFileEditorUnifiedRole(),
	conversions.RoleCollaborator: NewManagerUnifiedRole(),
	conversions.RoleUploader:     NewUploaderUnifiedRole(),
	conversions.RoleManager:      NewManagerUnifiedRole(),
}

func UnifiedRoleIDToDefinition(unifiedRoleID string) (*libregraph.UnifiedRoleDefinition, bool) {
	switch unifiedRoleID {
	case UnifiedRoleViewerID:
		return NewViewerUnifiedRole(), true
	case UnifiedRoleSpaceViewerID:
		return NewViewerUnifiedRole(), true
	case UnifiedRoleEditorID:
		return NewEditorUnifiedRole(), true
	case UnifiedRoleSpaceEditorID:
		return NewEditorUnifiedRole(), true
	case UnifiedRoleFileEditorID:
		return NewEditorUnifiedRole(), true
	case UnifiedRoleEditorLiteID:
		return NewEditorUnifiedRole(), true
	case UnifiedRoleManagerID:
		return NewManagerUnifiedRole(), true
	case UnifiedRoleSecureViewerID:
		return NewViewerUnifiedRole(), true
	default:
		return nil, false
	}
}
