package nceph

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCephVolumePathMapping(t *testing.T) {
	// Test the path mapping logic that converts between:
	// 1. Ceph volume paths (RADOS canonical form) - the common denominator
	// 2. Local filesystem paths (where Ceph is mounted locally)
	// 3. User-relative paths (what users see, relative to Root)

	testCases := []struct {
		name                string
		cephVolumePrefix    string // The Ceph volume path prefix (RADOS canonical)
		localMountPrefix    string // The local mount point prefix (where Ceph is mounted)
		cephVolumePath      string // Full Ceph volume path to test
		expectedUserPath    string // Expected user-relative path
		userPath            string // User-relative path to convert back
		expectedCephVolPath string // Expected full Ceph volume path
		description         string
	}{
		{
			name:                "basic_test_directory_mapping",
			cephVolumePrefix:    "/volumes/cephfs/app",
			localMountPrefix:    "/mnt/cephfs",
			cephVolumePath:      "/volumes/cephfs/app/test_dir/nested_file.txt",
			expectedUserPath:    "/test_dir/nested_file.txt",
			userPath:            "/test_dir/nested_file.txt",
			expectedCephVolPath: "/volumes/cephfs/app/test_dir/nested_file.txt",
			description:         "Maps test case from the issue: removes full test directory prefix",
		},
		{
			name:                "production_user_directory",
			cephVolumePrefix:    "/volumes/cephfs/app",
			localMountPrefix:    "/mnt/cephfs",
			cephVolumePath:      "/volumes/cephfs/app/users/alice/documents/report.pdf",
			expectedUserPath:    "/users/alice/documents/report.pdf",
			userPath:            "/users/alice/documents/report.pdf",
			expectedCephVolPath: "/volumes/cephfs/app/users/alice/documents/report.pdf",
			description:         "Production scenario with user-specific Ceph volume access",
		},
		{
			name:                "root_level_mount",
			cephVolumePrefix:    "/", // Use "/" instead of empty string to avoid defaulting
			localMountPrefix:    "/mnt/cephfs",
			cephVolumePath:      "/direct_file.txt",
			expectedUserPath:    "/direct_file.txt",
			userPath:            "/direct_file.txt",
			expectedCephVolPath: "/direct_file.txt",
			description:         "Direct access to Ceph volume root with root mount",
		},
		{
			name:                "deep_nested_structure",
			cephVolumePrefix:    "/volumes/cephfs/integration",
			localMountPrefix:    "/mnt/cephfs",
			cephVolumePath:      "/volumes/cephfs/integration/user1/projects/myproject/src/main.go",
			expectedUserPath:    "/user1/projects/myproject/src/main.go",
			userPath:            "/user1/projects/myproject/src/main.go",
			expectedCephVolPath: "/volumes/cephfs/integration/user1/projects/myproject/src/main.go",
			description:         "Deep nested structure with multiple directory levels",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test directory
			tempDir, cleanup := GetTestDir(t, "ceph-volume-path-mapping")
			defer cleanup()

			// Create filesystem with test configuration
			// localMountPrefix simulates where Ceph is mounted locally
			// Root simulates where we want to chroot (could be same or subdirectory)
			localMountPrefix := tc.localMountPrefix
			if localMountPrefix == "/mnt/cephfs" {
				// For testing, use tempDir as the mount point
				localMountPrefix = tempDir
			}

			config := map[string]interface{}{
				"chroot_dir": localMountPrefix, // Use the local mount point as chroot dir for this test
			}

			ctx := ContextWithTestLogger(t)
			ncephFS := NewForTesting(t, ctx, config, tc.cephVolumePrefix, localMountPrefix)

			// Test: Ceph volume path -> User path conversion
			// (Converting FROM common denominator TO user view)
			t.Run("volume_to_user", func(t *testing.T) {
				actualUserPath := ncephFS.convertCephVolumePathToUserPath(ctx, tc.cephVolumePath)
				assert.Equal(t, tc.expectedUserPath, actualUserPath,
					"%s: Ceph volume path '%s' should convert to user path '%s'",
					tc.description, tc.cephVolumePath, tc.expectedUserPath)
				t.Logf("✅ Volume→User: %s → %s", tc.cephVolumePath, actualUserPath)
			})

			// Test: User path -> Ceph volume path conversion
			// (Converting FROM user view TO common denominator)
			t.Run("user_to_volume", func(t *testing.T) {
				actualCephVolPath := ncephFS.convertUserPathToCephVolumePath(ctx, tc.userPath)
				assert.Equal(t, tc.expectedCephVolPath, actualCephVolPath,
					"%s: User path '%s' should convert to Ceph volume path '%s'",
					tc.description, tc.userPath, tc.expectedCephVolPath)
				t.Logf("✅ User→Volume: %s → %s", tc.userPath, actualCephVolPath)
			})

			// Test: Round-trip conversion preserves original paths
			t.Run("round_trip_consistency", func(t *testing.T) {
				// Convert user path -> Ceph volume path -> user path
				cephVolPath := ncephFS.convertUserPathToCephVolumePath(ctx, tc.userPath)
				finalUserPath := ncephFS.convertCephVolumePathToUserPath(ctx, cephVolPath)
				assert.Equal(t, tc.userPath, finalUserPath,
					"%s: Round-trip should preserve user path: %s → %s → %s",
					tc.description, tc.userPath, cephVolPath, finalUserPath)
				t.Logf("✅ Round-trip: %s → %s → %s", tc.userPath, cephVolPath, finalUserPath)
			})
		})
	}
}

func TestCephVolumePathConcept(t *testing.T) {
	// Test to document and validate the current simplified concept:
	// "Chroot-relative paths are used for all operations within the jail"

	tempDir, cleanup := GetTestDir(t, "ceph-volume-concept")
	defer cleanup()

	// Set environment variable to use tempDir as chroot
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	os.Setenv("NCEPH_TEST_CHROOT_DIR", tempDir)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	config := map[string]interface{}{
		"allow_local_mode": true, // Allow local mode for tests
	}

	ctx := ContextWithTestLogger(t)
	ncephFS := NewForTesting(t, ctx, config, "", tempDir)

	// Document the concept with clear examples
	t.Log("📖 Current NCeph Path Mapping Concept (Simplified):")
	t.Log("   1. All operations use chroot-relative paths within the jail")
	t.Log("   2. External user paths map to chroot-relative paths")
	t.Log("   3. No separate Ceph volume coordinates (removed for simplicity)")
	t.Log("")

	// Test the actual path conversion we have now: external to chroot and back
	// This is what actually matters for the current implementation
	externalUserPath := "/users/alice/documents/file.txt"
	chrootPath := ncephFS.toChroot(externalUserPath)
	backToExternal := ncephFS.fromChroot(chrootPath)

	t.Logf("📁 Current Implementation - Path conversion:")
	t.Logf("   External User Path:      %s", externalUserPath)
	t.Logf("   Chroot-relative Path:    %s", chrootPath)
	t.Logf("   Back to External:        %s", backToExternal)
	t.Logf("   Round-trip successful:   %v", externalUserPath == backToExternal)

	// These are the actual conversions that matter now
	assert.Equal(t, "users/alice/documents/file.txt", chrootPath)
	assert.Equal(t, externalUserPath, backToExternal)

	// Test another example
	externalRootPath := "/"
	chrootRootPath := ncephFS.toChroot(externalRootPath)
	backToExternalRoot := ncephFS.fromChroot(chrootRootPath)

	t.Logf("📁 Root directory conversion:")
	t.Logf("   External User Path:      %s", externalRootPath)
	t.Logf("   Chroot-relative Path:    %s", chrootRootPath)
	t.Logf("   Back to External:        %s", backToExternalRoot)

	assert.Equal(t, ".", chrootRootPath)
	assert.Equal(t, externalRootPath, backToExternalRoot)

	t.Log("✅ Simplified path conversion concept validated successfully")
}
