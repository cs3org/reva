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
			Existing:   RoleFromName("editor", true).CS3ResourcePermissions(),
			Requested:  nil,
			Sufficient: false,
		},
		{
			Existing:   nil,
			Requested:  RoleFromName("viewer", true).CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("editor", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("viewer", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("viewer", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("editor", true).CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("spaceviewer", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceeditor", true).CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("manager", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceeditor", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("spaceviewer", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("manager", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("manager", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("denied", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
		{
			Existing:   RoleFromName("spaceeditor", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("denied", true).CS3ResourcePermissions(),
			Sufficient: false,
		},
		{
			Existing:   RoleFromName("editor", true).CS3ResourcePermissions(),
			Requested:  RoleFromName("denied", true).CS3ResourcePermissions(),
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
			Requested:  RoleFromName("denied", true).CS3ResourcePermissions(),
			Sufficient: true,
		},
	}
	for _, test := range table {
		assert.Equal(t, test.Sufficient, SufficientCS3Permissions(test.Existing, test.Requested))
	}
}
