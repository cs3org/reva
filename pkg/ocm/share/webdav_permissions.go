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

package share

import (
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// ParseWebDAVPermissions translates one explicit WebDAV wire permission set.
func ParseWebDAVPermissions(names []string) (*ocm.SharePermissions, error) {
	var read, create, update, deleteResource, reshare bool
	for _, name := range names {
		switch name {
		case "read":
			read = true
		case "write":
			create, update, deleteResource = true, true, true
		case "create":
			create = true
		case "update":
			update = true
		case "delete":
			deleteResource = true
		case "share":
			reshare = true
		case "view":
			return nil, errtypes.NotSupported("Webapp permission in a WebDAV snapshot")
		default:
			return nil, errtypes.BadRequest("invalid WebDAV permission")
		}
	}
	// CS3 upload cannot distinguish creation from overwrite.
	if create != update {
		return nil, errtypes.NotSupported("asymmetric WebDAV file upload permissions")
	}
	return &ocm.SharePermissions{
		Permissions: &provider.ResourcePermissions{
			GetPath:              read || deleteResource || create,
			Stat:                 read || deleteResource || create,
			ListContainer:        read,
			InitiateFileDownload: read,
			InitiateFileUpload:   update,
			CreateContainer:      create,
			Delete:               deleteResource,
			DeleteContainer:      deleteResource,
		},
		Reshare: reshare,
	}, nil
}

// WebDAVPermissionNames serializes WebDAV rights without promoting a subset to write.
func WebDAVPermissionNames(p *provider.ResourcePermissions, reshare bool) []string {
	out := []string{}
	if p.GetInitiateFileDownload() {
		out = append(out, "read")
	}
	create, update, deleteResource := p.GetCreateContainer(), p.GetInitiateFileUpload(), p.GetDelete()
	if create && update && deleteResource {
		out = append(out, "write")
	} else {
		if create {
			out = append(out, "create")
		}
		if update {
			out = append(out, "update")
		}
		if deleteResource {
			out = append(out, "delete")
		}
	}
	if reshare {
		out = append(out, "share")
	}
	return out
}

// SameWebDAVOperations compares the operations represented by the wire
// vocabulary. The live provider can also have native MOVE authority, and
// a received grant can store an independent container-delete bit.
func SameWebDAVOperations(a, b *provider.ResourcePermissions) bool {
	return a.GetGetPath() == b.GetGetPath() &&
		a.GetStat() == b.GetStat() &&
		a.GetListContainer() == b.GetListContainer() &&
		a.GetInitiateFileDownload() == b.GetInitiateFileDownload() &&
		a.GetInitiateFileUpload() == b.GetInitiateFileUpload() &&
		a.GetCreateContainer() == b.GetCreateContainer() &&
		a.GetDelete() == b.GetDelete()
}
