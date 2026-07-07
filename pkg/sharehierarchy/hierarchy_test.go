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

package sharehierarchy_test

import (
	"context"
	"strconv"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/sharehierarchy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pathMap builds a static path resolver from a map of opaqueId → path.
func pathMap(m map[string]string) func(context.Context, *provider.ResourceId) (string, error) {
	return func(_ context.Context, id *provider.ResourceId) (string, error) {
		if p, ok := m[id.OpaqueId]; ok {
			return p, nil
		}
		return "", errtypes.NotFound(id.OpaqueId)
	}
}

// makeShare creates a minimal collaboration.Share for testing.
func makeShare(id int, opaqueId string, perms *provider.ResourcePermissions) *collaboration.Share {
	return &collaboration.Share{
		Id: &collaboration.ShareId{OpaqueId: strconv.Itoa(id)},
		ResourceId: &provider.ResourceId{
			StorageId: "instance1",
			SpaceId:   "space1",
			OpaqueId:  opaqueId,
		},
		Permissions: &collaboration.SharePermissions{Permissions: perms},
	}
}

func makeUserShare(id int, opaqueId string, perms *provider.ResourcePermissions, userID string) *collaboration.Share {
	share := makeShare(id, opaqueId, perms)
	share.Grantee = &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: userID},
		},
	}
	return share
}

func makeGroupShare(id int, opaqueId string, perms *provider.ResourcePermissions, groupID string) *collaboration.Share {
	share := makeShare(id, opaqueId, perms)
	share.Grantee = &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
		Id: &provider.Grantee_GroupId{
			GroupId: &grouppb.GroupId{OpaqueId: groupID},
		},
	}
	return share
}

var (
	readPerms = &provider.ResourcePermissions{Stat: true, ListContainer: true, InitiateFileDownload: true}
	rwPerms   = &provider.ResourcePermissions{Stat: true, ListContainer: true, InitiateFileDownload: true, InitiateFileUpload: true, CreateContainer: true}
	denyPerms = &provider.ResourcePermissions{} // all false = active denial
)

func TestCheckForAdd_NoExistingShares(t *testing.T) {
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{})}
	result, err := checker.CheckGrantConsistency(context.Background(), "/a/b", readPerms, nil)
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_ParentR_NodeRW_OK(t *testing.T) {
	// P=R, N=RW → OK (new share escalates within parent's subtree)
	parent := makeShare(1, "inode-a", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a/b", rwPerms, []*collaboration.Share{parent})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_ParentR_NodeR_Conflict(t *testing.T) {
	// P=R, N=R → ShareParentConflict (already covered by parent)
	parent := makeUserShare(1, "inode-a", readPerms, "user1")
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", readPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	require.True(t, ok, "expected HierarchyConflictError, got %T: %v", err, err)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, permissions.UnifiedRoleViewerID, conflictErr.ConflictingShares[0].PermissionType)
	assert.Equal(t, "user1", conflictErr.ConflictingShares[0].Sharee)
	assert.Equal(t, sharehierarchy.ShareeTypeUser, conflictErr.ConflictingShares[0].ShareeType)
}

func TestCheckForAdd_ParentRW_NodeR_Conflict(t *testing.T) {
	// P=RW, N=R → ShareParentConflict (parent already grants more)
	parent := makeShare(1, "inode-a", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", readPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	require.True(t, ok)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, permissions.UnifiedRoleEditorID, conflictErr.ConflictingShares[0].PermissionType)
}

func TestCheckForAdd_GroupParentConflictIncludesShareeType(t *testing.T) {
	parent := makeGroupShare(1, "inode-a", readPerms, "group1")
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", readPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	require.True(t, ok)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, "group1", conflictErr.ConflictingShares[0].Sharee)
	assert.Equal(t, sharehierarchy.ShareeTypeGroup, conflictErr.ConflictingShares[0].ShareeType)
	assert.Contains(t, conflictErr.MarshalToJSON(), `"sharee":"group1"`)
	assert.Contains(t, conflictErr.MarshalToJSON(), `"sharee_type":"group"`)
}

func TestCheckForAdd_ParentRW_NodeRW_Conflict(t *testing.T) {
	// P=RW, N=RW → ShareParentConflict (redundant)
	parent := makeShare(1, "inode-a", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", rwPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	_ = conflictErr
	assert.True(t, ok)
}

func TestCheckForAdd_ChildRW_NodeR_ReApply(t *testing.T) {
	// N=R, C=RW → child goes to ToReapply (child keeps its higher explicit ACL)
	child := makeShare(2, "inode-ab", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	require.Len(t, result.ToReapply, 1)
	assert.Equal(t, "2", result.ToReapply[0].Share.Id.OpaqueId)
}

func TestCheckForAdd_ChildR_NodeR_ToDelete(t *testing.T) {
	// N=R, C=R → child is redundant; goes to ToDelete
	child := makeShare(2, "inode-ab", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
	assert.Equal(t, "2", result.ToDelete[0].Share.Id.OpaqueId)
	assert.Empty(t, result.ToReapply)
}

func TestNewChildConflictError_IncludesResolvedChildPath(t *testing.T) {
	child := makeShare(2, "inode-ab", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)

	conflictErr := sharehierarchy.NewChildConflictError(sharehierarchy.ChildConflictMessage(result.ToDelete), result.ToDelete)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, "/a/b", conflictErr.ConflictingShares[0].Path)
	assert.Equal(t, permissions.UnifiedRoleViewerID, conflictErr.ConflictingShares[0].PermissionType)
	assert.Contains(t, conflictErr.MarshalToJSON(), `"permission_type":"`+permissions.UnifiedRoleViewerID+`"`)
	assert.NotContains(t, conflictErr.MarshalToJSON(), `"permission_type":"R"`)
}

func TestNewChildConflictError_GroupShareIncludesShareeType(t *testing.T) {
	child := makeGroupShare(2, "inode-ab", readPerms, "group1")
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)

	conflictErr := sharehierarchy.NewChildConflictError(sharehierarchy.ChildConflictMessage(result.ToDelete), result.ToDelete)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, "group1", conflictErr.ConflictingShares[0].Sharee)
	assert.Equal(t, sharehierarchy.ShareeTypeGroup, conflictErr.ConflictingShares[0].ShareeType)
	assert.Contains(t, conflictErr.MarshalToJSON(), `"sharee":"group1"`)
	assert.Contains(t, conflictErr.MarshalToJSON(), `"sharee_type":"group"`)
}

func TestCheckForAdd_ChildRW_NodeRW_ToDelete(t *testing.T) {
	// N=RW, C=RW → child is redundant; goes to ToDelete
	child := makeShare(2, "inode-ab", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", rwPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
}

func TestCheckForAdd_ChildR_NodeRW_ToDelete(t *testing.T) {
	// N=RW, C=R → child would be effectively elevated; goes to ToDelete
	child := makeShare(2, "inode-ab", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", rwPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
}

func TestCheckForAdd_UnrelatedShare_Ignored(t *testing.T) {
	// Share on /b has no relationship to /a
	other := makeShare(3, "inode-b", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-b": "/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{other})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_DeepChild_ToDelete(t *testing.T) {
	// Deep child /a/b/c/d should still be detected
	child := makeShare(2, "inode-deep", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-deep": "/a/b/c/d"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
}

func TestCheckForAdd_MultipleChildren_MixedResult(t *testing.T) {
	// /a/b (RW) → ToReapply; /a/c (R) → ToDelete
	childRW := makeShare(1, "inode-ab", rwPerms)
	childR := makeShare(2, "inode-ac", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{
		"inode-ab": "/a/b",
		"inode-ac": "/a/c",
	})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{childRW, childR})
	require.NoError(t, err)
	require.Len(t, result.ToReapply, 1)
	assert.Equal(t, "1", result.ToReapply[0].Share.Id.OpaqueId)
	require.Len(t, result.ToDelete, 1)
	assert.Equal(t, "2", result.ToDelete[0].Share.Id.OpaqueId)
}

func TestCheckForAdd_SiblingPrefix_NotAncestor(t *testing.T) {
	// /ab/c must NOT be treated as a child of /a (prefix but not path ancestor)
	other := makeShare(3, "inode-abc", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-abc": "/ab/c"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{other})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_DenyParent_Conflict(t *testing.T) {
	// P=Deny, N=R → conflict (Deny is max; nothing escalates beyond it)
	parent := makeShare(1, "inode-a", denyPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", readPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	require.True(t, ok)
	require.Len(t, conflictErr.ConflictingShares, 1)
	assert.Equal(t, permissions.UnifiedRoleDenyAccessID, conflictErr.ConflictingShares[0].PermissionType)
}

func TestCheckForAdd_ParentRW_NodeDeny_OK(t *testing.T) {
	// P=RW, N=Deny → OK (Deny > RW in the ordering; new share escalates)
	parent := makeShare(1, "inode-a", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a/b", denyPerms, []*collaboration.Share{parent})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_ParentR_NodeDeny_OK(t *testing.T) {
	// P=R, N=Deny → OK (Deny > R)
	parent := makeShare(1, "inode-a", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a/b", denyPerms, []*collaboration.Share{parent})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_ParentDeny_NodeDeny_Conflict(t *testing.T) {
	// P=Deny, N=Deny → conflict (equal, not an escalation)
	parent := makeShare(1, "inode-a", denyPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", denyPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	_ = conflictErr
	assert.True(t, ok)
}

func TestCheckForAdd_ParentDeny_NodeRW_Conflict(t *testing.T) {
	// P=Deny, N=RW → conflict (RW < Deny; cannot de-escalate via a child share)
	parent := makeShare(1, "inode-a", denyPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-a": "/a"})}

	_, err := checker.CheckGrantConsistency(context.Background(), "/a/b", rwPerms, []*collaboration.Share{parent})
	require.Error(t, err)
	conflictErr, ok := err.(*sharehierarchy.HierarchyConflictError)
	_ = conflictErr
	assert.True(t, ok)
}

func TestCheckForAdd_ChildDeny_NodeR_ToReapply(t *testing.T) {
	// N=R, C=Deny → child has higher perms (Deny > R); must be re-applied
	child := makeShare(2, "inode-ab", denyPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	require.Len(t, result.ToReapply, 1)
	assert.Equal(t, "2", result.ToReapply[0].Share.Id.OpaqueId)
}

func TestCheckForAdd_ChildDeny_NodeRW_ToReapply(t *testing.T) {
	// N=RW, C=Deny → child has higher perms (Deny > RW); must be re-applied
	child := makeShare(2, "inode-ab", denyPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", rwPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	assert.Empty(t, result.ToDelete)
	require.Len(t, result.ToReapply, 1)
}

func TestCheckForAdd_ChildR_NodeDeny_ToDelete(t *testing.T) {
	// N=Deny, C=R → child has lower perms (R < Deny); delete it
	child := makeShare(2, "inode-ab", readPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", denyPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
	assert.Empty(t, result.ToReapply)
}

func TestCheckForAdd_ChildRW_NodeDeny_ToDelete(t *testing.T) {
	// N=Deny, C=RW → child has lower perms (RW < Deny); delete it
	child := makeShare(2, "inode-ab", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"inode-ab": "/a/b"})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", denyPerms, []*collaboration.Share{child})
	require.NoError(t, err)
	require.Len(t, result.ToDelete, 1)
	assert.Empty(t, result.ToReapply)
}

func TestIsStrictAncestor(t *testing.T) {
	tests := []struct {
		ancestor string
		child    string
		want     bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/a/b/c", true},
		{"/a/b", "/a/b/c", true},
		{"/a", "/a", false},    // same path
		{"/a/b", "/a", false},  // child is actually parent
		{"/a", "/ab/c", false}, // prefix but not path ancestor
		{"/a", "/b", false},    // unrelated
		{"/", "/a", true},      // root
	}
	for _, tt := range tests {
		t.Run(tt.ancestor+"→"+tt.child, func(t *testing.T) {
			// Test isStrictAncestor indirectly via CheckGrantConsistency:
			// a share at tt.ancestor is a parent of the new node at tt.child
			// only when isStrictAncestor(tt.ancestor, tt.child) is true.
			share := makeShare(1, "id", readPerms)
			checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{"id": tt.ancestor})}
			_, err := checker.CheckGrantConsistency(context.Background(), tt.child, rwPerms, []*collaboration.Share{share})
			// rwPerms escalates beyond readPerms, so a true ancestor produces no error.
			// A non-ancestor produces no error either, but also no ToDelete/ToReapply.
			// We detect ancestry by whether the share appears in the result at all.
			result, _ := checker.CheckGrantConsistency(context.Background(), tt.child, readPerms, []*collaboration.Share{share})
			var detected bool
			if err != nil {
				detected = true // ShareParentConflict means ancestor was found
			} else if result != nil && len(result.ToDelete)+len(result.ToReapply) > 0 {
				detected = true // child relationship found — not what we're testing here
			}
			// For the ancestor direction: use readPerms on node so same-or-lower triggers conflict.
			_, ancestorErr := checker.CheckGrantConsistency(context.Background(), tt.child, readPerms, []*collaboration.Share{share})
			got := ancestorErr != nil
			assert.Equal(t, tt.want, got, "ancestor=%q child=%q", tt.ancestor, tt.child)
			_ = detected
		})
	}
}

func TestToReapplySortedShallowestFirst(t *testing.T) {
	// Three children at different depths; all have higher perms (RW) than the new node (R).
	// ToReapply must come out sorted shallowest-first without any manual sorting by the caller.
	deep := makeShare(1, "deep", rwPerms)
	root := makeShare(2, "root-child", rwPerms)
	mid := makeShare(3, "mid", rwPerms)
	checker := &sharehierarchy.Checker{GetPath: pathMap(map[string]string{
		"deep":       "/a/b/c/d",
		"root-child": "/a/b",
		"mid":        "/a/b/c",
	})}

	result, err := checker.CheckGrantConsistency(context.Background(), "/a", readPerms, []*collaboration.Share{deep, root, mid})
	require.NoError(t, err)
	require.Len(t, result.ToReapply, 3)
	assert.Equal(t, "2", result.ToReapply[0].Share.Id.OpaqueId) // /a/b
	assert.Equal(t, "3", result.ToReapply[1].Share.Id.OpaqueId) // /a/b/c
	assert.Equal(t, "1", result.ToReapply[2].Share.Id.OpaqueId) // /a/b/c/d
}
