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

package nceph

import (
	"context"
	"os"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNCeph_BasicOperations(t *testing.T) {
	// Create a test directory (configurable via NCEPH_TEST_DIR environment variable)
	tmpDir, cleanup := SetupTestDir(t, "nceph_test", 1000, 1000)
	defer cleanup()

	// Set environment variable to use tmpDir as chroot
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	os.Setenv("NCEPH_TEST_CHROOT_DIR", tmpDir)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create nceph instance
	config := map[string]interface{}{
		"allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	}

	ctx := context.Background()

	// Add a test user to the context
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "test-user",
		},
		Username:  "testuser",
		UidNumber: 1000,
		GidNumber: 1000,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)

	// Test CreateHome - should return NotSupported
	err = fs.CreateHome(ctx)
	assert.Error(t, err, "CreateHome should return NotSupported error")

	// Test GetHome - should return NotSupported
	_, err = fs.GetHome(ctx)
	assert.Error(t, err, "GetHome should return NotSupported error")

	// Test CreateDir
	ref := &provider.Reference{Path: "/testdir"}
	err = fs.CreateDir(ctx, ref)
	assert.NoError(t, err)

	// Verify directory was created within the chroot
	// Since we're in a chroot jail, we check via the filesystem interface
	stat, err := fs.GetMD(ctx, ref, []string{})
	assert.NoError(t, err)
	assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, stat.Type)

	// Test TouchFile
	fileRef := &provider.Reference{Path: "/testfile.txt"}
	err = fs.TouchFile(ctx, fileRef)
	assert.NoError(t, err)

	// Test GetMD for file
	md, err := fs.GetMD(ctx, fileRef, []string{})
	assert.NoError(t, err)
	assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, md.Type)

	// Test GetMD again
	md, err = fs.GetMD(ctx, fileRef, nil)
	assert.NoError(t, err)
	assert.NotNil(t, md)
	assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, md.Type)

	// Test ListFolder
	homeRef := &provider.Reference{Path: "."}
	files, err := fs.ListFolder(ctx, homeRef, nil)
	assert.NoError(t, err)
	assert.Len(t, files, 2) // testdir and testfile.txt

	// Test Delete
	err = fs.Delete(ctx, fileRef)
	assert.NoError(t, err)

	// Verify file was deleted by trying to get metadata (should fail)
	_, err = fs.GetMD(ctx, fileRef, nil)
	assert.Error(t, err) // Should fail because file doesn't exist

	// Test Shutdown
	err = fs.Shutdown(ctx)
	assert.NoError(t, err)
}
