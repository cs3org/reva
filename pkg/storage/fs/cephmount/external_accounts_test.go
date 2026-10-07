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

package cephmount

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testExternalAccount = "guest@example.org"

// newAuthTestFS builds a tree with a shared folder, a file inside it, and an
// unshared file next to it.
func newAuthTestFS(t *testing.T) (*cephmountfs, context.Context) {
	t.Helper()

	tempDir, cleanup := GetTestDir(t, "external-auth")
	t.Cleanup(cleanup)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "shared", "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "shared", "sub", "deep.txt"), []byte("deep"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "private.txt"), []byte("private"), 0644))

	ctx := ContextWithTestLogger(t)
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{"testing_allow_local_mode": true}, "/volumes/_nogroup/test", tempDir)
	return fs, ctx
}

// grantTo writes the grant xattr directly, bypassing AddGrant so the test does
// not depend on setfacl being able to name the service account uid.
func grantTo(t *testing.T, fs *cephmountfs, chrootPath, perms string) {
	t.Helper()
	require.NoError(t, xattr.Set(filepath.Join(fs.chrootDir, chrootPath), xattrExtShare+testExternalAccount, []byte(perms)))
}

func externalCtx(ctx context.Context) context.Context {
	return appctx.ContextSetUser(ctx, &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "external.example.org",
			OpaqueId: testExternalAccount,
			Type:     userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT,
		},
		Username: testExternalAccount,
	})
}

func TestExternalAccountGrantAppliesToDescendants(t *testing.T) {
	fs, _ := newAuthTestFS(t)
	grantTo(t, fs, "shared", "r-x")

	tests := []struct {
		path    string
		granted bool
	}{
		{path: "shared", granted: true},
		{path: "shared/sub", granted: true},
		{path: "shared/sub/deep.txt", granted: true},
		{path: "private.txt", granted: false},
		{path: ".", granted: false},
		// A path that does not exist resolves against its nearest existing
		// ancestor, which is what uploads of new files rely on.
		{path: "shared/sub/new.txt", granted: true},
		{path: "new-at-root.txt", granted: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			perms, found := fs.externalAccountGrant(testExternalAccount, tt.path)
			assert.Equal(t, tt.granted, found)
			if tt.granted {
				assert.True(t, perms.Stat)
				assert.True(t, perms.ListContainer)
				assert.False(t, perms.InitiateFileUpload)
			}
		})
	}
}

func TestAuthorizeExternalOnlyAppliesToExternalAccounts(t *testing.T) {
	fs, ctx := newAuthTestFS(t)

	// A regular user is never blocked here: their own uid is real and the kernel
	// decides, exactly as before external accounts existed.
	require.NoError(t, fs.authorizeExternal(ctx, "private.txt", "GetMD", canStat))
	require.NoError(t, fs.authorizeExternal(appctx.ContextSetUser(ctx, GetCurrentTestUser(t)), "private.txt", "GetMD", canStat))

	// An external account without any grant is refused.
	err := fs.authorizeExternal(externalCtx(ctx), "private.txt", "GetMD", canStat)
	require.Error(t, err)
	assert.IsType(t, errtypes.PermissionDenied(""), err)
}

func TestAuthorizeExternalHonoursGrantedPermissions(t *testing.T) {
	fs, ctx := newAuthTestFS(t)
	ctx = externalCtx(ctx)
	grantTo(t, fs, "shared", "r-x")

	// Read-only grant: reads pass, writes do not.
	require.NoError(t, fs.authorizeExternal(ctx, "shared/sub/deep.txt", "Download", canDownload))
	require.NoError(t, fs.authorizeExternal(ctx, "shared", "ListFolder", canList))

	for _, op := range []struct {
		name    string
		allowed func(*provider.ResourcePermissions) bool
	}{
		{"Upload", canUpload},
		{"Delete", canDelete},
		{"CreateDir", canCreate},
		{"AddGrant", canAddGrant},
	} {
		t.Run(op.name, func(t *testing.T) {
			err := fs.authorizeExternal(ctx, "shared/sub/deep.txt", op.name, op.allowed)
			require.Error(t, err)
			assert.IsType(t, errtypes.PermissionDenied(""), err)
		})
	}

	// Widening the grant lets the writes through.
	grantTo(t, fs, "shared", "rwx")
	require.NoError(t, fs.authorizeExternal(ctx, "shared/sub/deep.txt", "Upload", canUpload))
	require.NoError(t, fs.authorizeExternal(ctx, "shared/sub", "Delete", canDelete))
}

func TestExternalAccountOperationsAreDenied(t *testing.T) {
	fs, ctx := newAuthTestFS(t)
	ctx = externalCtx(ctx)
	grantTo(t, fs, "shared", "r-x")

	t.Run("unshared resource is invisible", func(t *testing.T) {
		_, err := fs.GetMD(ctx, &provider.Reference{Path: "/private.txt"}, nil)
		require.Error(t, err)
		assert.IsType(t, errtypes.PermissionDenied(""), err)

		_, err = fs.ListFolder(ctx, &provider.Reference{Path: "/"}, nil)
		require.Error(t, err)
		assert.IsType(t, errtypes.PermissionDenied(""), err)
	})

	t.Run("read-only grant refuses writes", func(t *testing.T) {
		err := fs.Delete(ctx, &provider.Reference{Path: "/shared/sub/deep.txt"})
		require.Error(t, err)
		assert.IsType(t, errtypes.PermissionDenied(""), err)

		_, err = fs.CreateDir(ctx, &provider.Reference{Path: "/shared/newdir"})
		require.Error(t, err)
		assert.IsType(t, errtypes.PermissionDenied(""), err)

		err = fs.SetArbitraryMetadata(ctx, &provider.Reference{Path: "/shared"}, &provider.ArbitraryMetadata{
			Metadata: map[string]string{"colour": "blue"},
		})
		require.Error(t, err)
		assert.IsType(t, errtypes.PermissionDenied(""), err)
	})

	t.Run("shared resource reports the granted permissions", func(t *testing.T) {
		ri, err := fs.GetMD(ctx, &provider.Reference{Path: "/shared/sub/deep.txt"}, nil)
		require.NoError(t, err)
		assert.Equal(t, "deep.txt", ri.Name)
		assert.True(t, ri.PermissionSet.Stat)
		assert.True(t, ri.PermissionSet.InitiateFileDownload)
		assert.False(t, ri.PermissionSet.InitiateFileUpload, "a read-only sharee must not be offered write actions")
		assert.False(t, ri.PermissionSet.Delete)
	})
}

// aclEntries returns the access ACL of path as getfacl reports it, numeric ids.
func aclEntries(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("getfacl", "--omit-header", "--numeric", path).CombinedOutput()
	require.NoError(t, err, "getfacl failed: %s", out)
	return string(out)
}

// TestServiceAccountGetsRwxOnSpaceRoot covers the canonical project layout:
// world-readable directories down to the space root, the space root itself
// closed (other::--x). Sharing a file inside must give the service account rwx
// on the space root — that is the only directory reva touches.
func TestServiceAccountGetsRwxOnSpaceRoot(t *testing.T) {
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Skip("setfacl not available")
	}

	tempDir, cleanup := GetTestDir(t, "space-root-acl")
	t.Cleanup(cleanup)

	// <chroot>/c/myproj/file.txt with myproj closed like a project root.
	spaceRoot := filepath.Join(tempDir, "c", "myproj")
	require.NoError(t, os.MkdirAll(spaceRoot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(spaceRoot, "file.txt"), []byte("x"), 0644))
	require.NoError(t, os.Chmod(spaceRoot, 0711))
	t.Cleanup(func() { _ = os.Chmod(spaceRoot, 0755) })

	ctx := ContextWithTestLogger(t)
	// What the storage provider injects for this layout: roots at
	// /winspaces/c/<project> are two levels below the chroot once /winspaces is
	// trimmed off, so its space_depth of 3 arrives here as 2.
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{
		"testing_allow_local_mode": true,
		"space_depth":              2,
	}, "/volumes/_nogroup/test", tempDir)
	uidEntry := fmt.Sprintf("user:%d:rwx", fs.conf.ExternalAccountsUserUID)

	grant := &provider.Grant{
		Grantee:     externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT),
		Permissions: &provider.ResourcePermissions{Stat: true, InitiateFileDownload: true},
	}
	require.NoError(t, fs.AddGrant(ctx, &provider.Reference{Path: "/c/myproj/file.txt"}, grant))

	assert.Contains(t, aclEntries(t, spaceRoot), uidEntry,
		"the space root must carry the service account entry")
	assert.NotContains(t, aclEntries(t, filepath.Join(tempDir, "c")), "user:"+fmt.Sprint(fs.conf.ExternalAccountsUserUID),
		"world-readable directories must not be touched")

	// Removing the last grant clears the file's entry but leaves the space root:
	// other shares in the same project may still rely on it.
	require.NoError(t, fs.RemoveGrant(ctx, &provider.Reference{Path: "/c/myproj/file.txt"}, grant))
	assert.NotContains(t, aclEntries(t, filepath.Join(spaceRoot, "file.txt")), fmt.Sprint(fs.conf.ExternalAccountsUserUID))
	assert.Contains(t, aclEntries(t, spaceRoot), uidEntry)
}

func TestSpaceRootFor(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "space-root-for")
	t.Cleanup(cleanup)
	ctx := ContextWithTestLogger(t)

	// The canonical layout: roots at /winspaces/c/<project>, which a provider
	// mounted at /winspaces declares as space_depth 3 and injects here as 2.
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{
		"testing_allow_local_mode": true,
		"space_depth":              2,
	}, "/volumes/_nogroup/test", tempDir)

	tests := []struct {
		path string
		root string
		ok   bool
	}{
		{path: "c/myproj/file.txt", root: "c/myproj", ok: true},
		{path: "c/myproj/sub/deep.txt", root: "c/myproj", ok: true},
		{path: "c/myproj", ok: false}, // the space root itself gets its ACL as the shared resource
		{path: "c", ok: false},
		{path: ".", ok: false},
	}
	for _, tt := range tests {
		root, ok := fs.spaceRootFor(tt.path)
		assert.Equal(t, tt.ok, ok, tt.path)
		assert.Equal(t, tt.root, root, tt.path)
	}
}

// Without a space depth the chroot root must not be mistaken for a space root:
// the service account would be granted rwx on the whole mount.
func TestSpaceRootForWithoutSpaceDepth(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "space-root-no-depth")
	t.Cleanup(cleanup)
	ctx := ContextWithTestLogger(t)
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{"testing_allow_local_mode": true}, "/volumes/_nogroup/test", tempDir)

	require.Zero(t, fs.conf.SpaceDepth, "space_depth must not be defaulted by the driver")
	for _, p := range []string{"c/myproj/file.txt", "c/myproj", "c", "."} {
		root, ok := fs.spaceRootFor(p)
		assert.False(t, ok, p)
		assert.Empty(t, root, p)
	}
}

// A space root above the driver's own chroot is not a layout it can grant on.
func TestNegativeSpaceDepthIsRejected(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "space-depth-negative")
	t.Cleanup(cleanup)
	ctx := ContextWithTestLogger(t)

	t.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", tempDir)
	_, err := New(ctx, map[string]any{
		"testing_allow_local_mode": true,
		"space_depth":              -1,
	})
	require.Error(t, err, "a negative space_depth must not be accepted")
}

func TestExternalAccountMapsToServiceUID(t *testing.T) {
	fs, _ := newAuthTestFS(t)

	uid, gid := fs.threadPool.mapUserToUIDGID(&userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: testExternalAccount,
			Type:     userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT,
		},
		Username: testExternalAccount,
	})

	assert.Equal(t, fs.conf.ExternalAccountsUserUID, uid, "external accounts must act as the service account")
	assert.Equal(t, fs.conf.ExternalAccountsUserGID, gid)
	assert.NotEqual(t, 1000, uid, "external accounts must not fall through to the default uid")
}
