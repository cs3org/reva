package nceph

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvironmentVariableChroot demonstrates the NCEPH_TEST_CHROOT_DIR environment variable
// functionality that allows overriding the chroot directory for testing without polluting
// the Options configuration struct.
func TestEnvironmentVariableChroot(t *testing.T) {
	// Create two temporary directories for testing
	tempDir1, cleanup1 := GetTestDir(t, "envvar-test-1")
	defer cleanup1()
	
	tempDir2, cleanup2 := GetTestDir(t, "envvar-test-2")
	defer cleanup2()

	// Create a test file in tempDir1
	testFile1 := filepath.Join(tempDir1, "test1.txt")
	err := os.WriteFile(testFile1, []byte("content1"), 0644)
	require.NoError(t, err)

	// Create a test file in tempDir2
	testFile2 := filepath.Join(tempDir2, "test2.txt")
	err = os.WriteFile(testFile2, []byte("content2"), 0644)
	require.NoError(t, err)

	t.Run("without_environment_variable", func(t *testing.T) {
		// Clear any existing environment variable
		originalValue := os.Getenv("NCEPH_TEST_CHROOT_DIR")
		os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		defer func() {
			if originalValue != "" {
				os.Setenv("NCEPH_TEST_CHROOT_DIR", originalValue)
			}
		}()

		// Standard config without fstab (local mode)
		config := map[string]interface{}{
			"allow_local_mode": true,
		}

		// CreateNcephFSForTesting should use the provided localMountPoint (tempDir1)
		fs := CreateNcephFSForTesting(t, "/volumes/test", tempDir1, config)
		
		// Verify it's using tempDir1 by checking it can access test1.txt
		assert.Equal(t, tempDir1, fs.chrootDir, "Should use tempDir1 as chroot directory")
		
		t.Logf("✅ Without environment variable:")
		t.Logf("   Chroot directory: %s", fs.chrootDir)
		t.Logf("   Local mount point: %s", fs.localMountPoint)
	})

	t.Run("with_environment_variable_override", func(t *testing.T) {
		// Set environment variable to override chroot directory
		os.Setenv("NCEPH_TEST_CHROOT_DIR", tempDir2)
		defer os.Unsetenv("NCEPH_TEST_CHROOT_DIR")

		// Same config as before
		config := map[string]interface{}{
			"allow_local_mode": true,
		}

		// Create filesystem - the environment variable should override the localMountPoint parameter
		fs, err := New(context.Background(), config)
		require.NoError(t, err, "New should succeed with environment variable override")
		
		ncephFS := fs.(*ncephfs)
		
		// Verify it's using tempDir2 (from environment variable) instead of tempDir1
		assert.Equal(t, tempDir2, ncephFS.chrootDir, "Should use tempDir2 from environment variable")
		
		t.Logf("✅ With environment variable override:")
		t.Logf("   Environment variable: NCEPH_TEST_CHROOT_DIR=%s", tempDir2)
		t.Logf("   Chroot directory: %s", ncephFS.chrootDir)
		t.Logf("   Original local mount point: (empty in local mode)")
	})

	t.Run("environment_variable_takes_precedence", func(t *testing.T) {
		// Set environment variable
		os.Setenv("NCEPH_TEST_CHROOT_DIR", tempDir2)
		defer os.Unsetenv("NCEPH_TEST_CHROOT_DIR")

		// Even with a fstab entry that would normally determine the chroot,
		// the environment variable should take precedence
		config := map[string]interface{}{
			"fstabentry": "cephminiflax.cern.ch:6789:/volumes/_nogroup/admin /mnt/different_mount ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.key,conf=/etc/ceph/ceph.conf 0 2",
			"allow_local_mode": true,
		}

		fs, err := New(context.Background(), config)
		require.NoError(t, err, "New should succeed")
		
		ncephFS := fs.(*ncephfs)
		
		// Environment variable should override everything
		assert.Equal(t, tempDir2, ncephFS.chrootDir, "Environment variable should override fstab-derived chroot")
		assert.Equal(t, "/mnt/different_mount", ncephFS.localMountPoint, "Should still parse local mount point from fstab")
		
		t.Logf("✅ Environment variable precedence test:")
		t.Logf("   Fstab local mount: /mnt/different_mount")
		t.Logf("   Environment override: %s", tempDir2)
		t.Logf("   Actual chroot directory: %s", ncephFS.chrootDir)
		t.Logf("   ✅ Environment variable takes precedence!")
	})
}

// TestEnvironmentVariableDocumentation provides documentation for the environment variable feature
func TestEnvironmentVariableDocumentation(t *testing.T) {
	t.Log("📖 NCEPH_TEST_CHROOT_DIR Environment Variable Documentation:")
	t.Log("")
	t.Log("Purpose:")
	t.Log("  - Allows overriding the chroot directory for testing purposes")
	t.Log("  - Does NOT pollute the Options configuration struct")
	t.Log("  - Keeps production configuration clean and simple")
	t.Log("")
	t.Log("Usage:")
	t.Log("  export NCEPH_TEST_CHROOT_DIR=/path/to/test/directory")
	t.Log("  go test ./pkg/storage/fs/nceph")
	t.Log("")
	t.Log("Behavior:")
	t.Log("  - If set, overrides any chroot directory determination")
	t.Log("  - Takes precedence over fstab-derived local mount points")
	t.Log("  - Takes precedence over default /tmp/nceph-test in local mode")
	t.Log("  - Only affects the chroot jail location, not other configuration")
	t.Log("")
	t.Log("Benefits:")
	t.Log("  - Clean separation between production config and testing needs")
	t.Log("  - Sysadmin configuration remains simple and uncluttered")
	t.Log("  - Test flexibility without config complexity")
}
