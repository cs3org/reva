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
	"os"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func externalGrantee(opaqueID string, t userv1beta1.UserType) *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{UserId: &userv1beta1.UserId{
			Idp:      "external.example.org",
			OpaqueId: opaqueID,
			Type:     t,
		}},
	}
}

func TestExternalAccountQualifier(t *testing.T) {
	tests := []struct {
		name     string
		grantee  *provider.Grantee
		expected string
		isExtern bool
	}{
		{
			name:     "lightweight account",
			grantee:  externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT),
			expected: "guest@example.org",
			isExtern: true,
		},
		{
			name:     "federated account",
			grantee:  externalGrantee("remote@other.org", userv1beta1.UserType_USER_TYPE_FEDERATED),
			expected: "remote@other.org",
			isExtern: true,
		},
		{
			name:    "primary account is resolved locally",
			grantee: externalGrantee("alice", userv1beta1.UserType_USER_TYPE_PRIMARY),
		},
		{
			name:    "unspecified type is resolved locally",
			grantee: externalGrantee("alice", userv1beta1.UserType_USER_TYPE_INVALID),
		},
		{
			name: "group grantee",
			grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
			},
		},
		{
			name:    "user grantee without id",
			grantee: &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER},
		},
		{
			name: "nil grantee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qualifier, ok := externalAccountQualifier(tt.grantee)
			assert.Equal(t, tt.isExtern, ok)
			assert.Equal(t, tt.expected, qualifier)
		})
	}
}

// newGrantTestFS returns a filesystem rooted at a temporary directory holding a
// single file and directory, both reachable through their external paths.
func newGrantTestFS(t *testing.T) *cephmountfs {
	t.Helper()

	tempDir, cleanup := GetTestDir(t, "external-account-grants")
	t.Cleanup(cleanup)

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "shared.txt"), []byte("content"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(tempDir, "shared-dir"), 0755))

	config := map[string]any{"testing_allow_local_mode": true}
	return CreateCephMountFSForTesting(t, ContextWithTestLogger(t), config, "/volumes/_nogroup/test", tempDir)
}

func TestExternalAccountGrantRoundTrip(t *testing.T) {
	for _, target := range []string{"/shared.txt", "/shared-dir"} {
		t.Run(target, func(t *testing.T) {
			fs := newGrantTestFS(t)
			ctx := ContextWithTestLogger(t)
			ref := &provider.Reference{Path: target}
			fullPath := filepath.Join(fs.chrootDir, fs.toChroot(target))

			grant := &provider.Grant{
				Grantee: externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT),
				Permissions: &provider.ResourcePermissions{
					Stat:                 true,
					GetPath:              true,
					InitiateFileDownload: true,
					ListContainer:        true,
				},
			}

			// The grant lands in an xattr, never in a POSIX ACL: a lightweight
			// account has no entry in /etc/passwd to resolve.
			require.NoError(t, fs.AddGrant(ctx, ref, grant))

			value, err := xattr.Get(fullPath, xattrExtShare+"guest@example.org")
			require.NoError(t, err, "grant should be stored as an xattr")
			assert.Equal(t, "r-x", string(value))

			glist, err := fs.ListGrants(ctx, ref)
			require.NoError(t, err)
			require.Len(t, glist, 1)
			got := glist[0]
			assert.Equal(t, provider.GranteeType_GRANTEE_TYPE_USER, got.Grantee.Type)
			assert.Equal(t, "guest@example.org", got.Grantee.GetUserId().OpaqueId)
			assert.Equal(t, userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT, got.Grantee.GetUserId().Type)
			assert.True(t, got.Permissions.Stat)
			assert.True(t, got.Permissions.InitiateFileDownload)
			assert.True(t, got.Permissions.ListContainer)
			assert.False(t, got.Permissions.InitiateFileUpload)

			// UpdateGrant overwrites the stored permissions.
			grant.Permissions.InitiateFileUpload = true
			require.NoError(t, fs.UpdateGrant(ctx, ref, grant))
			value, err = xattr.Get(fullPath, xattrExtShare+"guest@example.org")
			require.NoError(t, err)
			assert.Equal(t, "rwx", string(value))

			require.NoError(t, fs.RemoveGrant(ctx, ref, grant))
			_, err = xattr.Get(fullPath, xattrExtShare+"guest@example.org")
			assert.ErrorIs(t, err, xattr.ENOATTR, "grant xattr should be gone")

			glist, err = fs.ListGrants(ctx, ref)
			require.NoError(t, err)
			assert.Empty(t, glist)
		})
	}
}

func TestRemoveExternalAccountGrantIsIdempotent(t *testing.T) {
	fs := newGrantTestFS(t)
	ctx := ContextWithTestLogger(t)

	grant := &provider.Grant{
		Grantee:     externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_FEDERATED),
		Permissions: &provider.ResourcePermissions{},
	}

	// Removing a grant that was never added must not fail.
	require.NoError(t, fs.RemoveGrant(ctx, &provider.Reference{Path: "/shared.txt"}, grant))
}

// A failure to read the grants must not be reported as a resource without any:
// the caller cannot tell the two apart, and an empty list reads as "not shared".
func TestListGrantsFailsInsteadOfReportingNoGrants(t *testing.T) {
	fs := newGrantTestFS(t)
	ctx := ContextWithTestLogger(t)
	ref := &provider.Reference{Path: "/shared.txt"}

	require.NoError(t, fs.AddGrant(ctx, ref, &provider.Grant{
		Grantee:     externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT),
		Permissions: &provider.ResourcePermissions{Stat: true},
	}))

	// The resource disappears under the listing.
	require.NoError(t, os.Remove(filepath.Join(fs.chrootDir, fs.toChroot("/shared.txt"))))

	glist, err := fs.ListGrants(ctx, ref)
	require.Error(t, err, "reading the grants of a resource that is gone must fail")
	assert.Empty(t, glist)
	assert.Implements(t, (*errtypes.IsNotFound)(nil), err, "a resource that is gone is a NotFound, not an internal error")
}

func TestExternalAccountGrantsAreNotArbitraryMetadata(t *testing.T) {
	fs := newGrantTestFS(t)
	ctx := ContextWithTestLogger(t)
	ref := &provider.Reference{Path: "/shared.txt"}
	chrootPath := fs.toChroot("/shared.txt")

	require.NoError(t, fs.AddGrant(ctx, ref, &provider.Grant{
		Grantee:     externalGrantee("guest@example.org", userv1beta1.UserType_USER_TYPE_LIGHTWEIGHT),
		Permissions: &provider.ResourcePermissions{Stat: true},
	}))
	require.NoError(t, fs.SetArbitraryMetadata(ctx, ref, &provider.ArbitraryMetadata{
		Metadata: map[string]string{"colour": "blue"},
	}))

	md := fs.readArbitraryMetadata(chrootPath, nil)
	assert.Equal(t, "blue", md["colour"])
	assert.NotContains(t, md, "reva.extshare.guest@example.org", "grants must not leak into arbitrary metadata")

	// The reserved keys cannot be written or cleared through the metadata API,
	// which would otherwise silently revoke or forge a share.
	err := fs.SetArbitraryMetadata(ctx, ref, &provider.ArbitraryMetadata{
		Metadata: map[string]string{"reva.extshare.guest@example.org": "rwx"},
	})
	assert.Error(t, err)

	err = fs.UnsetArbitraryMetadata(ctx, ref, []string{"reva.extshare.guest@example.org"})
	assert.Error(t, err)

	value, err := xattr.Get(filepath.Join(fs.chrootDir, chrootPath), xattrExtShare+"guest@example.org")
	require.NoError(t, err)
	assert.Equal(t, "r--", string(value), "the stored grant must be untouched")
}
