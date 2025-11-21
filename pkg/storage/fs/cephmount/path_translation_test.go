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

package cephmount

import (
	"os"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathTranslation(t *testing.T) {
	// Create test directory (configurable via CEPHMOUNT_TEST_DIR environment variable)
	tmpDir, cleanup := SetupTestDir(t, "cephmount-path-test", 1000, 1000)
	defer cleanup()

	// Set environment variable to use tmpDir as chroot
	originalChrootDir := os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR")
	os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", tmpDir)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("CEPHMOUNT_TEST_CHROOT_DIR")
		} else {
			os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create test context with user
	ctx := ContextWithTestLogger(t)
	user := &userv1beta1.User{
		Id:          &userv1beta1.UserId{OpaqueId: "testuser"},
		Username:    "testuser",
		DisplayName: "Test User",
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Create filesystem with environment variable chroot
	fs, err := New(ctx, map[string]any{
		"dataTx":                   false,
		"ceph_cfg":                 "",
		"testing_allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	})
	require.NoError(t, err)
	require.NotNil(t, fs)

	// Test that external paths are handled transparently
	tests := []struct {
		name         string
		externalPath string
		shouldExist  bool
	}{
		{
			name:         "root path",
			externalPath: "/",
			shouldExist:  true,
		},
		{
			name:         "subdirectory",
			externalPath: "/subdir",
			shouldExist:  false, // Will create it
		},
		{
			name:         "nested file",
			externalPath: "/subdir/file.txt",
			shouldExist:  false, // Will create it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &provider.Reference{Path: tt.externalPath}

			if !tt.shouldExist {
				// Create directory or file
				if tt.externalPath == "/subdir" {
					err = fs.CreateDir(ctx, ref)
					assert.NoError(t, err)
				} else if tt.externalPath == "/subdir/file.txt" {
					err = fs.TouchFile(ctx, ref)
					assert.NoError(t, err)
				}
			}

			// Get metadata using external path
			md, err := fs.GetMD(ctx, ref, []string{})
			assert.NoError(t, err)
			assert.NotNil(t, md)

			// Verify that the returned path matches the external path
			assert.Equal(t, tt.externalPath, md.Path, "External path should be preserved in response")

			// Verify the file actually exists in the chroot directory
			if tt.externalPath == "/" {
				// Root should always exist
				_, err = os.Stat(tmpDir)
				assert.NoError(t, err)
			} else if tt.externalPath == "/subdir" {
				// Should exist as tmpDir/subdir
				actualPath := filepath.Join(tmpDir, "subdir")
				info, err := os.Stat(actualPath)
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			} else if tt.externalPath == "/subdir/file.txt" {
				// Should exist as tmpDir/subdir/file.txt
				actualPath := filepath.Join(tmpDir, "subdir", "file.txt")
				info, err := os.Stat(actualPath)
				assert.NoError(t, err)
				assert.False(t, info.IsDir())
			}
		})
	}

	// Test ListFolder - should return external paths
	rootRef := &provider.Reference{Path: "/"}
	files, err := fs.ListFolder(ctx, rootRef, []string{})
	assert.NoError(t, err)

	// Should contain the subdirectory we created
	found := false
	for _, file := range files {
		if file.Path == "/subdir" {
			found = true
			assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, file.Type)
		}
	}
	assert.True(t, found, "ListFolder should return external paths")

	// Test listing subdirectory
	subdirRef := &provider.Reference{Path: "/subdir"}
	files, err = fs.ListFolder(ctx, subdirRef, []string{})
	assert.NoError(t, err)

	// Should contain the file we created
	found = false
	for _, file := range files {
		if file.Path == "/subdir/file.txt" {
			found = true
			assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, file.Type)
		}
	}
	assert.True(t, found, "ListFolder should return external paths for nested content")
}
