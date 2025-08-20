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
	"os"
	"path/filepath"
	"testing"
)

// GetTestDir returns a directory for testing. It checks for the NCEPH_TEST_DIR
// environment variable. If set, it creates a subdirectory within that path.
// If not set, it falls back to creating a temporary directory.
//
// The returned cleanup function should be called to remove the test directory
// unless NCEPH_TEST_DIR is set and NCEPH_TEST_PRESERVE is also set to "true".
//
// Usage:
//
//	testDir, cleanup := GetTestDir(t, "test-prefix")
//	defer cleanup()
//
// Environment variables:
//
//	NCEPH_TEST_DIR: Base directory for tests (e.g., "/mnt/ceph/test")
//	NCEPH_TEST_PRESERVE: If "true", preserves test directories when NCEPH_TEST_DIR is set
func GetTestDir(t *testing.T, prefix string) (string, func()) {
	baseDir := os.Getenv("NCEPH_TEST_DIR")
	preserve := os.Getenv("NCEPH_TEST_PRESERVE") == "true"

	if baseDir == "" {
		// Use temporary directory as fallback
		tmpDir, err := os.MkdirTemp("", prefix)
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		return tmpDir, func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("Warning: failed to remove temp dir %s: %v", tmpDir, err)
			}
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("Failed to create base test dir %s: %v", baseDir, err)
	}

	// Create unique subdirectory within the base directory
	testDir, err := os.MkdirTemp(baseDir, prefix+"-")
	if err != nil {
		t.Fatalf("Failed to create test dir in %s: %v", baseDir, err)
	}

	t.Logf("Using test directory: %s", testDir)

	cleanup := func() {
		if preserve {
			t.Logf("Preserving test directory: %s", testDir)
			return
		}
		if err := os.RemoveAll(testDir); err != nil {
			t.Logf("Warning: failed to remove test dir %s: %v", testDir, err)
		}
	}

	return testDir, cleanup
}

// SetupTestDir is a convenience function that calls GetTestDir and sets up
// appropriate permissions for the returned directory. It also handles the
// common pattern of changing ownership to allow test users to write.
//
// The uid and gid parameters specify the desired ownership. If uid is 0,
// no ownership change is attempted.
func SetupTestDir(t *testing.T, prefix string, uid, gid int) (string, func()) {
	testDir, cleanup := GetTestDir(t, prefix)

	// Set permissions
	if err := os.Chmod(testDir, 0755); err != nil {
		cleanup()
		t.Fatalf("Failed to set permissions on test dir %s: %v", testDir, err)
	}

	// Change ownership if requested and we're running as root
	if uid != 0 {
		if err := os.Chown(testDir, uid, gid); err != nil {
			// Only log warning, don't fail the test
			t.Logf("Warning: failed to change ownership of %s to %d:%d: %v", testDir, uid, gid, err)
		}
	}

	return testDir, cleanup
}

// GetTestSubDir creates a subdirectory within an existing test directory.
// This is useful when you need multiple directories for a single test.
func GetTestSubDir(t *testing.T, baseDir, subDirName string) string {
	subDir := filepath.Join(baseDir, subDirName)
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory %s: %v", subDir, err)
	}
	return subDir
}
