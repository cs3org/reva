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

package ocmshares

import (
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

func TestGetRoleTreatsWebappUploadAsEditor(t *testing.T) {
	share := &ocm.Share{
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebappOptions{
					WebappOptions: &ocm.WebappAccessMethod{
						Permissions: &ocm.SharePermissions{
							Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
						},
					},
				},
			},
		},
	}

	role, roleStr := getRole(share)
	if role != authpb.Role_ROLE_EDITOR {
		t.Fatalf("getRole() role = %v, want %v", role, authpb.Role_ROLE_EDITOR)
	}
	if roleStr != "editor" {
		t.Fatalf("getRole() roleStr = %q, want %q", roleStr, "editor")
	}
}

func TestGetRoleTreatsWebappReadAsViewer(t *testing.T) {
	share := &ocm.Share{
		AccessMethods: []*ocm.AccessMethod{
			{
				Term: &ocm.AccessMethod_WebappOptions{
					WebappOptions: &ocm.WebappAccessMethod{
						Permissions: &ocm.SharePermissions{
							Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
						},
					},
				},
			},
		},
	}

	role, roleStr := getRole(share)
	if role != authpb.Role_ROLE_VIEWER {
		t.Fatalf("getRole() role = %v, want %v", role, authpb.Role_ROLE_VIEWER)
	}
	if roleStr != "viewer" {
		t.Fatalf("getRole() roleStr = %q, want %q", roleStr, "viewer")
	}
}
