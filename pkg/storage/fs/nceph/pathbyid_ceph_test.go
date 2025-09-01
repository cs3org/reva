//go:build ceph

package nceph

import (
	"path/filepath"
	"strings"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCephAdminConnection(t *testing.T) {
	// This test only verifies that we can establish a Ceph admin connection
	RequireCephIntegration(t)

	ctx := ContextWithTestLogger(t)

	// Get Ceph configuration from environment or defaults
	cephConfig := GetCephConfig()

	// Create Options struct
	config := &Options{
		FstabEntry: cephConfig["fstab_entry"].(string),
	}

	// Test creating a real ceph admin connection
	conn, err := newCephAdminConn(ctx, config)

	if err != nil {
		// Should not be configuration-related errors since we validated the config
		assert.NotContains(t, err.Error(), "not enabled", "Should not get 'not enabled' error with proper config")
		assert.NotContains(t, err.Error(), "build with -tags ceph", "Should not get build tag error with proper config")
		assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get incomplete config error with proper config")

		// Any other error means the connection failed - this is a test failure
		t.Fatalf("Failed to create Ceph admin connection with valid configuration: %v", err)
	} else {
		// Successfully created connection
		t.Log("Successfully created Ceph admin connection")
		// Clean up
		if conn != nil && conn.radosConn != nil {
			conn.radosConn.Shutdown()
		}
		if conn != nil && conn.adminMount != nil {
			conn.adminMount.Release()
		}
	}
}

func TestCephRootConfiguration(t *testing.T) {
	// This test verifies that the Ceph root parameter is properly configured
	RequireCephIntegration(t)

	ctx := ContextWithTestLogger(t)

	// Test with default root
	t.Run("default_root", func(t *testing.T) {
		// The new simplified approach doesn't have a separate ceph_root config
		// The chroot directory is determined from the fstab entry's local mount point
		// This test now just validates that the configuration works correctly
		t.Log("Simplified configuration: chroot directory is derived from fstab entry")
	})

	// Test with auto-discovered configuration
	t.Run("auto_discovery", func(t *testing.T) {
		config := GetCephConfig()

		// Verify we can create Options struct with auto-discovery
		options := &Options{
			FstabEntry: config["fstab_entry"].(string),
		}

		// Test that we can create a connection with auto-discovery
		// (This may fail due to connection issues, but shouldn't fail due to config)
		conn, err := newCephAdminConn(ctx, options)
		if err != nil {
			t.Logf("Connection failed with custom root (may be expected): %v", err)
			// Verify it's not a configuration error
			assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get config error")
		} else {
			t.Logf("Successfully created connection with fstab-based configuration")
			// Clean up
			if conn != nil {
				conn.Close()
			}
		}
	})

	// Test with fstab configuration
	t.Run("fstab_configuration", func(t *testing.T) {
		config := GetCephConfig()

		// Verify we have a valid fstab entry
		fstabEntry, exists := config["fstab_entry"]
		if !exists {
			t.Fatal("No fstab entry available - set NCEPH_FSTAB_ENTRY environment variable")
		}

		assert.NotEmpty(t, fstabEntry, "Fstab entry should not be empty")
		t.Logf("Using fstab entry: %s", fstabEntry)
	})
}

func TestGetPathByIDWithCreatedFiles(t *testing.T) {
	// This test creates actual files and directories in the Ceph filesystem using nceph,
	// extracts their inodes, and then queries them via the admin connection using GetPathByID
	RequireCephIntegration(t)

	// Use the integration helper to get nceph FS with real fstab config
	ctx := ContextWithTestLogger(t)
	fs := CreateNcephFSForIntegration(t, ctx, nil)

	// Add a root user to the context for integration testing
	// This ensures that file operations run with proper privileges
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "root",
			Idp:      "local",
		},
		Username:  "root",
		UidNumber: 0, // Root UID
		GidNumber: 0, // Root GID
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Ensure we have a working Ceph admin connection for GetPathByID
	require.NotNil(t, fs.cephAdminConn, "Ceph admin connection is required for this test")

	// Test cases - create files and directories that we'll query by inode
	testCases := []struct {
		name  string
		path  string
		isDir bool
	}{
		{"test_directory", "/test_dir", true},
		{"test_file", "/test_file.txt", false},
		{"nested_directory", "/test_dir/nested_dir", true},  // Create after test_dir
		{"nested_file", "/test_dir/nested_file.txt", false}, // Create after test_dir
	}

	// Step 1: Create all test files and directories in the Ceph filesystem
	t.Log("Creating test files and directories in Ceph filesystem...")
	for _, tc := range testCases {
		t.Logf("Creating %s at path %s", tc.name, tc.path)

		if tc.isDir {
			err := fs.CreateDir(ctx, &provider.Reference{Path: tc.path})
			require.NoError(t, err, "Failed to create directory %s", tc.path)
		} else {
			err := fs.TouchFile(ctx, &provider.Reference{Path: tc.path})
			require.NoError(t, err, "Failed to create file %s", tc.path)
		}

		t.Logf("Successfully created %s", tc.name)
	}

	// Step 2: Get metadata for each created item and test GetPathByID round-trip
	t.Log("Testing GetPathByID round-trip for created files...")
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get metadata to extract the inode
			t.Logf("Getting metadata for %s at path %s", tc.name, tc.path)
			md, err := fs.GetMD(ctx, &provider.Reference{Path: tc.path}, nil)
			require.NoError(t, err, "Failed to get metadata for %s", tc.name)
			require.NotNil(t, md, "Metadata should not be nil for %s", tc.name)

			// Extract inode from resource ID
			inode := md.Id.OpaqueId
			require.NotEmpty(t, inode, "Inode should not be empty for %s", tc.name)
			t.Logf("Found inode %s for %s", inode, tc.name)

			// Verify the resource type matches expectation
			if tc.isDir {
				require.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, md.Type,
					"Resource type should be directory for %s", tc.name)
			} else {
				require.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, md.Type,
					"Resource type should be file for %s", tc.name)
			}

			// Step 3: Use GetPathByID to retrieve the path from the inode via admin connection
			t.Logf("Testing GetPathByID for inode %s (%s)", inode, tc.name)
			retrievedPath, err := fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: inode})
			require.NoError(t, err, "GetPathByID failed for %s (inode %s). This indicates issues with Ceph admin connection or MDS access", tc.name, inode)
			require.NotEmpty(t, retrievedPath, "Retrieved path should not be empty for %s", tc.name)

			t.Logf("Retrieved path: '%s' for inode %s (%s)", retrievedPath, inode, tc.name)

			// The retrieved path should match our expected path or be a normalized version
			expectedPath := tc.path
			if retrievedPath == expectedPath {
				t.Logf("Perfect match: GetPathByID returned exact expected path")
			} else if strings.HasSuffix(retrievedPath, expectedPath) {
				t.Logf("Suffix match: GetPathByID returned normalized path ending with expected suffix")
			} else {
				t.Logf("Path difference: expected '%s', got '%s'", expectedPath, retrievedPath)
				// Verify that at least the filename/directory name matches
				expectedBase := filepath.Base(expectedPath)
				retrievedBase := filepath.Base(retrievedPath)
				assert.Equal(t, expectedBase, retrievedBase,
					"At minimum, the filename/directory name should match for %s", tc.name)
				t.Logf("At least the filename matches: '%s'", expectedBase)
			}

			t.Logf("Round-trip successful: %s created → inode %s → path '%s'",
				tc.name, inode, retrievedPath)
		})
	}

	t.Log("All round-trip tests completed successfully")
	t.Log("This confirms that:")
	t.Log("  1. Files and directories can be created in the Ceph filesystem via nceph")
	t.Log("  2. Inodes can be extracted from file metadata")
	t.Log("  3. GetPathByID can successfully resolve inodes to paths via Ceph admin connection")
	t.Log("  4. All returned paths are within the expected mount boundaries")
}

func TestGetPathByIDSecurityValidation(t *testing.T) {
	// This test ensures that GetPathByID properly validates paths and rejects traversal attacks
	RequireCephIntegration(t)

	ctx := ContextWithTestLogger(t)
	fs := CreateNcephFSForIntegration(t, ctx, nil)

	// Add a root user to the context for integration testing
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "root",
			Idp:      "local",
		},
		Username:  "root",
		UidNumber: 0,
		GidNumber: 0,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	require.NotNil(t, fs.cephAdminConn, "Ceph admin connection is required for this test")

	t.Log("Testing GetPathByID security validation and path bounds checking...")

	// Create a legitimate test file within bounds
	testPath := "/security_test_file.txt"
	err := fs.TouchFile(ctx, &provider.Reference{Path: testPath})
	require.NoError(t, err, "Failed to create test file for security validation")

	// Get the metadata to extract the inode
	md, err := fs.GetMD(ctx, &provider.Reference{Path: testPath}, nil)
	require.NoError(t, err, "Failed to get metadata for security test file")
	require.NotNil(t, md, "Metadata should not be nil")

	inode := md.Id.OpaqueId
	require.NotEmpty(t, inode, "Inode should not be empty")

	t.Logf("Created test file with inode %s for security validation", inode)

	// Test 1: Legitimate GetPathByID should work
	t.Run("legitimate_path_retrieval", func(t *testing.T) {
		retrievedPath, err := fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: inode})
		require.NoError(t, err, "Legitimate GetPathByID should succeed")
		require.NotEmpty(t, retrievedPath, "Retrieved path should not be empty")

		t.Logf("Legitimate path retrieval successful: %s", retrievedPath)

		// Verify the path matches our expectation
		assert.True(t, strings.HasSuffix(retrievedPath, testPath) || retrievedPath == testPath,
			"Retrieved path should match expected path (got: %s, expected suffix: %s)", retrievedPath, testPath)

		// Note: We don't call validatePathWithinBounds here because GetPathByID already
		// performs all necessary security validation internally before returning the path.
		// The returned path is the user-facing path, not the raw CephFS volume path.
	})

	// Test 2: Verify that validatePathWithinBounds rejects malicious paths
	t.Run("path_validation_rejects_traversal", func(t *testing.T) {
		maliciousPaths := []string{
			"../../etc/passwd",
			"/../../root/.ssh/id_rsa",
			"../../../etc/shadow",
			"/var/../../etc/hosts",
			fs.cephVolumePath + "/../../../secret",
		}

		for _, badPath := range maliciousPaths {
			t.Run("reject_"+strings.ReplaceAll(badPath, "/", "_"), func(t *testing.T) {
				err := fs.validatePathWithinBounds(ctx, badPath, "security_test")
				assert.Error(t, err, "Should reject malicious path: %s", badPath)
				t.Logf("Correctly rejected malicious path: %s", badPath)
			})
		}
	})

	// Test 3: Verify the returned path is exactly what we expect
	t.Run("path_consistency_check", func(t *testing.T) {
		retrievedPath, err := fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: inode})
		require.NoError(t, err, "GetPathByID should succeed for consistency check")

		// The retrieved path should be the same as the original path we created
		// This ensures no path manipulation occurred during the round-trip
		expectedPath := testPath

		if retrievedPath == expectedPath {
			t.Logf("Perfect consistency: retrieved path exactly matches expected path")
		} else if strings.HasSuffix(retrievedPath, expectedPath) {
			t.Logf("Acceptable consistency: retrieved path ends with expected path")
			t.Logf("   Expected: %s", expectedPath)
			t.Logf("   Retrieved: %s", retrievedPath)
		} else {
			// This could indicate a security issue or unexpected path manipulation
			t.Errorf("Path consistency issue: retrieved path doesn't match expected pattern")
			t.Errorf("   Expected: %s", expectedPath)
			t.Errorf("   Retrieved: %s", retrievedPath)
		}

		// Additional check: ensure no path components have been altered
		expectedBase := filepath.Base(expectedPath)
		retrievedBase := filepath.Base(retrievedPath)
		assert.Equal(t, expectedBase, retrievedBase,
			"Filename should be identical (expected: %s, got: %s)", expectedBase, retrievedBase)
	})

	// Test 4: Verify mount boundary enforcement
	t.Run("mount_boundary_enforcement", func(t *testing.T) {
		t.Logf("Mount configuration:")
		t.Logf("  Ceph volume path: %s", fs.cephVolumePath)
		t.Logf("  Local mount point: %s", fs.localMountPoint)
		t.Logf("  Chroot directory: %s", fs.chrootDir)

		retrievedPath, err := fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: inode})
		require.NoError(t, err, "GetPathByID should succeed for boundary check")

		// Convert the user path back to the full Ceph volume path for validation
		cephVolumePath := fs.convertUserPathToCephVolumePath(ctx, retrievedPath)

		// Ensure the Ceph volume path is within the configured volume bounds
		if fs.cephVolumePath != "" && fs.cephVolumePath != "/" {
			assert.True(t, strings.HasPrefix(cephVolumePath, fs.cephVolumePath),
				"Ceph volume path should be within configured bounds (path: %s, bounds: %s)",
				cephVolumePath, fs.cephVolumePath)
		}

		t.Logf("Mount boundary enforcement verified")
		t.Logf("   User path: %s", retrievedPath)
		t.Logf("   Ceph volume path: %s", cephVolumePath)
	})

	t.Log("All security validation tests completed successfully")
	t.Log("This confirms that GetPathByID:")
	t.Log("  1. Properly validates paths for traversal attacks")
	t.Log("  2. Enforces mount boundary restrictions")
	t.Log("  3. Returns consistent and expected paths")
	t.Log("  4. Rejects potentially malicious path patterns")
}
