package nceph

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateNcephFSForTesting creates an ncephfs instance for testing purposes.
// If localMountPoint is provided, it will override the mount point for unit tests.
// If localMountPoint is empty, it will use the mount point from NCEPH_FSTAB_ENTRY for integration tests.
func CreateNcephFSForTesting(t *testing.T, cephVolumePath, localMountPoint string, config map[string]interface{}) *ncephfs {
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
	ctx := context.Background()
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
// It uses the real mount point from NCEPH_FSTAB_ENTRY without any overrides.
func CreateNcephFSForIntegration(t *testing.T, config map[string]interface{}) *ncephfs {
	// Build test configuration without local mode or overrides
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}
	
	// For integration tests, get fstab entry from environment if not provided in config
	if _, exists := testConfig["fstabentry"]; !exists {
		if fstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY"); fstabEntry != "" {
			testConfig["fstabentry"] = fstabEntry
		}
	}
	
	// Do NOT set allow_local_mode for integration tests

	// Create the filesystem using the standard New function with real fstab entry
	ctx := context.Background()
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for integration testing")

	return fs.(*ncephfs)
}
