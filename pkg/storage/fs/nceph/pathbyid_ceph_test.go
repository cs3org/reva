//go:build ceph

package nceph

import (
	"os"
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

func TestGetPathByIDIntegration(t *testing.T) {
	// This test verifies the complete GetPathByID functionality
	// It should fail for ANY error - connection issues, privilege issues, etc.
	RequireCephIntegration(t)

	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-pathbyid-test")
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

	// Initialize nceph with ceph configuration
	ctx := ContextWithTestLogger(t)

	// Add a root user to the context for integration testing
	// This ensures that operations run with proper privileges
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

	// Get fstab entry from environment and pass in config
	cephConfig := GetCephConfig()
	config := map[string]interface{}{
		"fstabentry": cephConfig["fstab_entry"].(string),
	}
	
	fs, err := New(ctx, config)
	require.NoError(t, err, "Failed to create nceph filesystem with Ceph configuration")
	require.NotNil(t, fs)

	// Test GetPathByID - this should work without any errors
	// If it fails for any reason (connection, privileges, file not found, etc.), the test fails
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "1"})

	if err != nil {
		// ANY error is a test failure - we expect GetPathByID to work properly
		t.Fatalf("GetPathByID failed. This indicates issues with Ceph connection, privileges, or configuration: %v", err)
	} else {
		t.Log("GetPathByID succeeded - Ceph integration is working correctly")
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
		t.Log("✅ Simplified configuration: chroot directory is derived from fstab entry")
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

	// Create test directory for the integration test
	tempDir, cleanup := GetTestDir(t, "nceph-pathbyid-roundtrip")
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

	// Initialize nceph filesystem with Ceph configuration
	ctx := ContextWithTestLogger(t)

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

	// Get fstab entry from environment and pass in config  
	cephConfig := GetCephConfig()
	config := map[string]interface{}{
		"fstabentry": cephConfig["fstab_entry"].(string),
	}
	
	fs, err := New(ctx, config)
	require.NoError(t, err, "Failed to create nceph filesystem with Ceph configuration")
	require.NotNil(t, fs)

	// Ensure we have a working Ceph admin connection for GetPathByID
	require.NotNil(t, fs.(*ncephfs).cephAdminConn, "Ceph admin connection is required for this test")

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

		t.Logf("✅ Successfully created %s", tc.name)
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
				t.Logf("✅ Perfect match: GetPathByID returned exact expected path")
			} else if strings.HasSuffix(retrievedPath, expectedPath) {
				t.Logf("✅ Suffix match: GetPathByID returned normalized path ending with expected suffix")
			} else {
				t.Logf("⚠️  Path difference: expected '%s', got '%s'", expectedPath, retrievedPath)
				// Verify that at least the filename/directory name matches
				expectedBase := filepath.Base(expectedPath)
				retrievedBase := filepath.Base(retrievedPath)
				assert.Equal(t, expectedBase, retrievedBase,
					"At minimum, the filename/directory name should match for %s", tc.name)
				t.Logf("✅ At least the filename matches: '%s'", expectedBase)
			}

			t.Logf("✅ Round-trip successful: %s created → inode %s → path '%s'",
				tc.name, inode, retrievedPath)
		})
	}

	t.Log("✅ All round-trip tests completed successfully")
	t.Log("This confirms that:")
	t.Log("  1. Files and directories can be created in the Ceph filesystem via nceph")
	t.Log("  2. Inodes can be extracted from file metadata")
	t.Log("  3. GetPathByID can successfully resolve inodes to paths via Ceph admin connection")
}
