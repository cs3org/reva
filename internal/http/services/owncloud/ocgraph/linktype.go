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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/spaces/

package ocgraph

import (
	"context"
	"errors"

	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/storage/utils/grants"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

// NoPermissionMatchError is the message returned by a failed conversion
const NoPermissionMatchError = "no matching permission set found"

// LinkType contains cs3 permissions and a libregraph
// linktype reference
type LinkType struct {
	Permissions *provider.ResourcePermissions
	linkType    libregraph.SharingLinkType
}

// GetPermissions returns the cs3 permissions type
func (l *LinkType) GetPermissions() *provider.ResourcePermissions {
	if l != nil {
		return l.Permissions
	}
	return nil
}

// SharingLinkTypeFromCS3Permissions creates a libregraph link type
// It returns a list of libregraph actions when the conversion is not possible
func SharingLinkTypeFromCS3Permissions(ctx context.Context, permissions *linkv1beta1.PublicSharePermissions) (*libregraph.SharingLinkType, []string) {
	if permissions == nil {
		return nil, nil
	}

	var lt libregraph.SharingLinkType

	if grants.PermissionsEqual(permissions.GetPermissions(), NewViewLinkPermissionSet().GetPermissions()) {
		lt = libregraph.VIEW
	} else if grants.PermissionsEqual(permissions.GetPermissions(), NewFolderEditLinkPermissionSet().GetPermissions()) ||
		grants.PermissionsEqual(permissions.GetPermissions(), NewFileEditLinkPermissionSet().GetPermissions()) {
		lt = libregraph.EDIT
	} else if grants.PermissionsEqual(permissions.GetPermissions(), NewFolderDropLinkPermissionSet().GetPermissions()) {
		lt = libregraph.CREATE_ONLY
	} else if grants.PermissionsEqual(permissions.GetPermissions(), NewFolderDropLinkPermissionSet().GetPermissions()) {
		lt = libregraph.UPLOAD
	} else {
		return nil, CS3ResourcePermissionsToLibregraphActions(permissions.GetPermissions())
	}
	return &lt, nil
}

// CS3ResourcePermissionsFromSharingLink creates a cs3 resource permissions type
// it returns an error when the link type is not allowed or empty
func CS3ResourcePermissionsFromSharingLink(linkType libregraph.SharingLinkType, info provider.ResourceType) (*provider.ResourcePermissions, error) {
	switch linkType {
	case "":
		return nil, errors.New("link type is empty")
	case libregraph.VIEW:
		return NewViewLinkPermissionSet().GetPermissions(), nil
	case libregraph.EDIT:
		if info == provider.ResourceType_RESOURCE_TYPE_FILE {
			return NewFileEditLinkPermissionSet().GetPermissions(), nil
		}
		return NewFolderEditLinkPermissionSet().GetPermissions(), nil
	case libregraph.CREATE_ONLY:
		if info == provider.ResourceType_RESOURCE_TYPE_FILE {
			return nil, errors.New(NoPermissionMatchError)
		}
		return NewFolderDropLinkPermissionSet().GetPermissions(), nil
	case libregraph.UPLOAD:
		if info == provider.ResourceType_RESOURCE_TYPE_FILE {
			return nil, errors.New(NoPermissionMatchError)
		}
		return NewFolderDropLinkPermissionSet().GetPermissions(), nil
	case libregraph.INTERNAL:
		return NewInternalLinkPermissionSet().GetPermissions(), nil
	default:
		return nil, errors.New(NoPermissionMatchError)
	}
}

// NewInternalLinkPermissionSet creates cs3 permissions for the internal link type
func NewInternalLinkPermissionSet() *LinkType {
	return &LinkType{
		Permissions: &provider.ResourcePermissions{},
		linkType:    libregraph.INTERNAL,
	}
}

// NewViewLinkPermissionSet creates cs3 permissions for the view link type
func NewViewLinkPermissionSet() *LinkType {
	return &LinkType{
		Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
		linkType:    libregraph.VIEW,
	}
}

// NewFileEditLinkPermissionSet creates cs3 permissions for the file edit link type
func NewFileEditLinkPermissionSet() *LinkType {
	return &LinkType{
		Permissions: permissions.NewFileEditorRole().CS3ResourcePermissions(),
		linkType:    libregraph.EDIT,
	}
}

// NewFolderEditLinkPermissionSet creates cs3 permissions for the folder edit link type
func NewFolderEditLinkPermissionSet() *LinkType {
	return &LinkType{
		Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
		linkType:    libregraph.EDIT,
	}
}

// NewFolderDropLinkPermissionSet creates cs3 permissions for the folder createOnly link type
func NewFolderDropLinkPermissionSet() *LinkType {
	return &LinkType{
		Permissions: permissions.NewUploaderRole().CS3ResourcePermissions(),
		linkType:    libregraph.CREATE_ONLY,
	}
}

// GetAvailableLinkTypes returns a slice of all available link types
func GetAvailableLinkTypes() []*LinkType {
	return []*LinkType{
		NewInternalLinkPermissionSet(),
		NewViewLinkPermissionSet(),
		NewFileEditLinkPermissionSet(),
		NewFolderEditLinkPermissionSet(),
		NewFolderDropLinkPermissionSet(),
	}
}
