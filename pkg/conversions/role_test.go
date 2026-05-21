package conversions

import (
	"testing"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
)

func TestSufficientPermissions(t *testing.T) {
	type testData struct {
		Existing   *providerv1beta1.ResourcePermissions
		Requested  *providerv1beta1.ResourcePermissions
		Sufficient bool
	}
	table := []testData{
		{
			Existing:   nil,
			Requested:  nil,
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("editor").CS3ResourcePermissions(),
			Requested:  nil,
			Sufficient: false,
		},
		{
			Existing:   nil,
			Requested:  RoleFromName("viewer").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("editor").CS3ResourcePermissions(),
			Requested:  RoleFromName("viewer").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("viewer").CS3ResourcePermissions(),
			Requested:  RoleFromName("editor").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("spaceviewer").CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceeditor").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("manager").CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceeditor").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager").CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceviewer").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager").CS3ResourcePermissions(),
			Requested:  RoleFromName("manager").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager").CS3ResourcePermissions(),
			Requested:  RoleFromName("denied").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("spaceeditor").CS3ResourcePermissions(),
			Requested:  RoleFromName("denied").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("editor").CS3ResourcePermissions(),
			Requested:  RoleFromName("denied").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("secure-viewer").CS3ResourcePermissions(),
			Requested:  RoleFromName("secure-viewer").CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("secure-viewer").CS3ResourcePermissions(),
			Requested:  RoleFromName("viewer").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("secure-viewer").CS3ResourcePermissions(),
			Requested:  RoleFromName("editor").CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing: &providerv1beta1.ResourcePermissions{
				// all permissions, used for personal space owners
				AddGrant:             true,
				CreateContainer:      true,
				Delete:               true,
				GetPath:              true,
				GetQuota:             true,
				InitiateFileDownload: true,
				InitiateFileUpload:   true,
				ListContainer:        true,
				ListFileVersions:     true,
				ListGrants:           true,
				ListRecycle:          true,
				Move:                 true,
				PurgeRecycle:         true,
				RemoveGrant:          true,
				RestoreFileVersion:   true,
				RestoreRecycleItem:   true,
				Stat:                 true,
				UpdateGrant:          true,
				DenyGrant:            true,
			},
			Requested:  RoleFromName("denied").CS3ResourcePermissions(),
			Sufficient: true,
		},
	}
	for _, test := range table {
		assert.Equal(t, test.Sufficient, SufficientCS3Permissions(test.Existing, test.Requested))
	}
}

func TestNewSpaceEditorWithoutVersionsWithoutTrashbinRole(t *testing.T) {
	role := NewSpaceEditorWithoutVersionsWithoutTrashbinRole()
	p := role.CS3ResourcePermissions()

	assert.Equal(t, RoleSpaceEditorWithoutVersionsWithoutTrashbin, role.Name)

	// should have basic editor permissions
	assert.True(t, p.CreateContainer)
	assert.True(t, p.Delete)
	assert.True(t, p.GetPath)
	assert.True(t, p.GetQuota)
	assert.True(t, p.InitiateFileDownload)
	assert.True(t, p.InitiateFileUpload)
	assert.True(t, p.ListContainer)
	assert.True(t, p.ListGrants)
	assert.True(t, p.Move)
	assert.True(t, p.Stat)

	// should not have version permissions
	assert.False(t, p.ListFileVersions)
	assert.False(t, p.RestoreFileVersion)

	// should not have trashbin permissions
	assert.False(t, p.ListRecycle)
	assert.False(t, p.RestoreRecycleItem)
	assert.False(t, p.PurgeRecycle)
}

func TestRoleFromResourcePermissions_WithoutTrashbinRolesAreWritable(t *testing.T) {
	for _, constructor := range []func() *Role{
		NewSpaceEditorWithoutTrashbinRole,
		NewSpaceEditorWithoutVersionsWithoutTrashbinRole,
	} {
		role := constructor()
		got := RoleFromResourcePermissions(role.CS3ResourcePermissions(), false)
		assert.True(t, got.ocsPermissions.Contain(PermissionWrite),
			"expected PermissionWrite for role %s", role.Name)
		assert.Contains(t, got.WebDAVPermissions(false, false, false, false), "W",
			"expected W in WebDAV permissions for role %s", role.Name)
	}
}
