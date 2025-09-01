package nceph

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateNcephFSForTesting creates an ncephfs instance for testing purposes using environment variables
// for chroot override, keeping the Options struct clean for production use.
func CreateNcephFSForTesting(t *testing.T, cephVolumePath, localMountPoint string, config map[string]interface{}) *ncephfs {
	// Set environment variable to override chroot directory for testing
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()
	
	// Set the test chroot directory
	os.Setenv("NCEPH_TEST_CHROOT_DIR", localMountPoint)

	// Build test configuration
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}
	testConfig["allow_local_mode"] = true

	// Create the filesystem using the standard New function
	ctx := context.Background()
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for testing")

	ncephFS := fs.(*ncephfs)
	
	// Override the discovered paths for testing
	ncephFS.cephVolumePath = cephVolumePath
	ncephFS.localMountPoint = localMountPoint
	
	return ncephFS
}

func NewForTesting(t *testing.T, ctx context.Context, config map[string]interface{}, cephVolumePath string, localMountPoint string) *ncephfs {
	// Set environment variable to override chroot directory for testing
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()
	
	// Set the test chroot directory
	os.Setenv("NCEPH_TEST_CHROOT_DIR", localMountPoint)

	// Build test configuration
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}
	testConfig["allow_local_mode"] = true

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create ncephfs for testing")

	ncephFS := fs.(*ncephfs)
	
	// Override the discovered paths for testing
	ncephFS.cephVolumePath = cephVolumePath
	ncephFS.localMountPoint = localMountPoint
	
	return ncephFS
}
