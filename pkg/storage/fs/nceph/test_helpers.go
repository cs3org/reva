package nceph

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateNcephFSForTesting creates an ncephfs instance for unit tests.
// Unit tests use synthetic configuration and temporary directories.
// The NCEPH_FSTAB_ENTRY environment variable is ignored for unit tests.
func CreateNcephFSForTesting(t *testing.T, ctx context.Context, config map[string]interface{}, cephVolumePath string, localMountPoint string) *ncephfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}
	// Unit tests always use local mode and ignore real fstab entries
	testConfig["allow_local_mode"] = true
	// Don't set fstabentry for unit tests - they should be isolated
	delete(testConfig, "fstabentry")

	// Set the test chroot directory environment variable for unit tests
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	os.Setenv("NCEPH_TEST_CHROOT_DIR", localMountPoint)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for unit testing")

	ncephFS := fs.(*ncephfs)

	// Override the discovered paths for unit tests
	ncephFS.cephVolumePath = cephVolumePath
	ncephFS.localMountPoint = localMountPoint

	return ncephFS
}

func NewForTesting(t *testing.T, ctx context.Context, config map[string]interface{}, cephVolumePath string, localMountPoint string) *ncephfs {
	var originalChrootDir string
	var needsRestore bool

	// Only override chroot if localMountPoint is provided (unit tests)
	if localMountPoint != "" {
		// This is a unit test - override the chroot directory
		originalChrootDir = os.Getenv("NCEPH_TEST_CHROOT_DIR")
		os.Setenv("NCEPH_TEST_CHROOT_DIR", localMountPoint)
		needsRestore = true
	}
	// If localMountPoint is empty, this is an integration test - let it use the real mount point from fstab

	defer func() {
		if needsRestore {
			if originalChrootDir == "" {
				os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
			} else {
				os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
			}
		}
	}()

	// Build test configuration
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}

	// For unit tests, enable local mode
	if localMountPoint != "" {
		testConfig["allow_local_mode"] = true
	}

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for testing")

	ncephFS := fs.(*ncephfs)

	// Override the discovered paths only for unit tests
	if localMountPoint != "" {
		ncephFS.cephVolumePath = cephVolumePath
		ncephFS.localMountPoint = localMountPoint
	}

	return ncephFS
}

// CreateNcephFSForIntegration creates an ncephfs instance for integration tests.
// Integration tests use the real fstab entry from NCEPH_FSTAB_ENTRY environment variable.
func CreateNcephFSForIntegration(t *testing.T, ctx context.Context, config map[string]interface{}) *ncephfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}

	// Integration tests require a real fstab entry
	if config == nil || config["fstabentry"] == nil {
		// Try to get from environment if not provided in config
		if fstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY"); fstabEntry != "" {
			testConfig["fstabentry"] = fstabEntry
		}
	}

	// Do NOT set allow_local_mode for integration tests

	// Create the filesystem using the standard New function with real fstab entry
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for integration testing")

	return fs.(*ncephfs)
}
