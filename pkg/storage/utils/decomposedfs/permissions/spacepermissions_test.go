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

package permissions

import (
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/conversions"
)

// TestIsManagerRejectsPartialGrantManagement ensures a single grant-management bit
// must not classify as manager, but the full triad must, without requiring DenyGrant
// (so coowner/legacy manager grants still pass).
func TestIsManagerRejectsPartialGrantManagement(t *testing.T) {
	cases := []struct {
		name string
		rp   *provider.ResourcePermissions
		want bool
	}{
		{"RemoveGrant only", &provider.ResourcePermissions{RemoveGrant: true}, false},
		{"AddGrant only", &provider.ResourcePermissions{AddGrant: true}, false},
		{"UpdateGrant only", &provider.ResourcePermissions{UpdateGrant: true}, false},
		{"Add+Remove without Update", &provider.ResourcePermissions{AddGrant: true, RemoveGrant: true}, false},
		{"full triad", &provider.ResourcePermissions{AddGrant: true, UpdateGrant: true, RemoveGrant: true}, true},
		{"manager role", conversions.NewManagerRole().CS3ResourcePermissions(), true},
		{"coowner role (no DenyGrant)", conversions.NewCoownerRole().CS3ResourcePermissions(), true},
	}
	for _, tc := range cases {
		if got := IsSpaceManager(tc.rp); got != tc.want {
			t.Errorf("IsSpaceManager(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestIsEditorAcceptsSpaceEditorVariants ensures every space-editor variant (and manager,
// a superset) classifies as editor, while viewers do not.
func TestIsEditorAcceptsSpaceEditorVariants(t *testing.T) {
	editors := map[string]*provider.ResourcePermissions{
		"space editor":                        conversions.NewSpaceEditorRole().CS3ResourcePermissions(),
		"space editor without versions":       conversions.NewSpaceEditorWithoutVersionsRole().CS3ResourcePermissions(),
		"space editor without trashbin":       conversions.NewSpaceEditorWithoutTrashbinRole().CS3ResourcePermissions(),
		"space editor without versions/trash": conversions.NewSpaceEditorWithoutVersionsWithoutTrashbinRole().CS3ResourcePermissions(),
		"manager":                             conversions.NewManagerRole().CS3ResourcePermissions(),
	}
	for name, rp := range editors {
		if !IsSpaceEditor(rp) {
			t.Errorf("IsSpaceEditor(%s) = false, want true", name)
		}
	}

	notEditors := map[string]*provider.ResourcePermissions{
		"space viewer":  conversions.NewSpaceViewerRole().CS3ResourcePermissions(),
		"secure viewer": conversions.NewSecureViewerRole().CS3ResourcePermissions(),
	}
	for name, rp := range notEditors {
		if IsSpaceEditor(rp) {
			t.Errorf("IsSpaceEditor(%s) = true, want false", name)
		}
	}
}

// TestIsSpaceViewerAcceptsSpaceRoleVariants ensures the space-viewer predicate accepts every
// space role that can read (all space roles carry ListGrants), including subset editors and
// the manager, while non-space and grant-only grants do not.
func TestIsSpaceViewerAcceptsSpaceRoleVariants(t *testing.T) {
	viewers := map[string]*provider.ResourcePermissions{
		"space viewer":                        conversions.NewSpaceViewerRole().CS3ResourcePermissions(),
		"space editor without versions/trash": conversions.NewSpaceEditorWithoutVersionsWithoutTrashbinRole().CS3ResourcePermissions(),
		"space editor":                        conversions.NewSpaceEditorRole().CS3ResourcePermissions(),
		"manager":                             conversions.NewManagerRole().CS3ResourcePermissions(),
	}
	for name, rp := range viewers {
		if !IsSpaceViewer(rp) {
			t.Errorf("IsSpaceViewer(%s) = false, want true", name)
		}
	}

	// secure viewer is a file/folder role (no ListGrants) and is not a space role
	notViewers := map[string]*provider.ResourcePermissions{
		"secure viewer":     conversions.NewSecureViewerRole().CS3ResourcePermissions(),
		"empty/denied":      {},
		"remove grant only": {RemoveGrant: true},
	}
	for name, rp := range notViewers {
		if IsSpaceViewer(rp) {
			t.Errorf("IsSpaceViewer(%s) = true, want false", name)
		}
	}
}
