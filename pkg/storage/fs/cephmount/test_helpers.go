package cephmount

import (
	"context"
	"maps"
	"os"
	"os/user"
	"strconv"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/stretchr/testify/require"
)

// CreateCephMountFSForTesting creates an cephmountfs instance for unit tests.
// Unit tests use synthetic configuration and temporary directories.
// The CEPHMOUNT_FSTAB_ENTRY environment variable is ignored for unit tests.
func CreateCephMountFSForTesting(t *testing.T, ctx context.Context, config map[string]any, cephVolumePath string, localMountPoint string) *cephmountfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]any)
	maps.Copy(testConfig, config)
	// Unit tests always use local mode and ignore real fstab entries
	testConfig["testing_allow_local_mode"] = true
	// Don't set fstabentry for unit tests - they should be isolated
	delete(testConfig, "fstabentry")

	// Set the test chroot directory environment variable for unit tests
	originalChrootDir := os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR")
	os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", localMountPoint)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("CEPHMOUNT_TEST_CHROOT_DIR")
		} else {
			os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create cephmountfs for unit testing")

	cephmountFS := fs.(*cephmountfs)

	// Override the discovered paths for unit tests
	cephmountFS.cephVolumePath = cephVolumePath
	cephmountFS.localMountPoint = localMountPoint

	return cephmountFS
}

// GetCurrentTestUser returns the current user information for use in tests
func GetCurrentTestUser(t *testing.T) *userv1beta1.User {
	currentUser, err := user.Current()
	require.NoError(t, err, "failed to get current user")

	uid, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err, "failed to parse current user UID")

	gid, err := strconv.Atoi(currentUser.Gid)
	require.NoError(t, err, "failed to parse current user GID")

	return &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: currentUser.Username,
			Idp:      "local",
		},
		Username:  currentUser.Username,
		UidNumber: int64(uid),
		GidNumber: int64(gid),
	}
}

func NewForTesting(t *testing.T, ctx context.Context, config map[string]any, cephVolumePath string, localMountPoint string) *cephmountfs {
	var originalChrootDir string
	var needsRestore bool

	// Only override chroot if localMountPoint is provided (unit tests)
	if localMountPoint != "" {
		// This is a unit test - override the chroot directory
		originalChrootDir = os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR")
		os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", localMountPoint)
		needsRestore = true
	}
	// If localMountPoint is empty, this is an integration test - let it use the real mount point from fstab

	defer func() {
		if needsRestore {
			if originalChrootDir == "" {
				os.Unsetenv("CEPHMOUNT_TEST_CHROOT_DIR")
			} else {
				os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", originalChrootDir)
			}
		}
	}()

	// Build test configuration
	testConfig := make(map[string]any)
	maps.Copy(testConfig, config)

	// For unit tests, enable local mode
	if localMountPoint != "" {
		testConfig["testing_allow_local_mode"] = true
	}

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create cephmountfs for testing")

	cephmountFS := fs.(*cephmountfs)

	// Override the discovered paths only for unit tests
	if localMountPoint != "" {
		cephmountFS.cephVolumePath = cephVolumePath
		cephmountFS.localMountPoint = localMountPoint
	}

	return cephmountFS
}

// CreateCephMountFSForIntegration creates an cephmountfs instance for integration tests.
// Integration tests use the real fstab entry from CEPHMOUNT_FSTAB_ENTRY environment variable.
func CreateCephMountFSForIntegration(t *testing.T, ctx context.Context, config map[string]any) *cephmountfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]any)
	maps.Copy(testConfig, config)

	// Integration tests require a real fstab entry
	if config == nil || config["fstabentry"] == nil {
		// Try to get from environment if not provided in config
		if fstabEntry := os.Getenv("CEPHMOUNT_FSTAB_ENTRY"); fstabEntry != "" {
			testConfig["fstabentry"] = fstabEntry
		}
	}

	// Do NOT set testing_allow_local_mode for integration tests

	// Create the filesystem using the standard New function with real fstab entry
	fs, err := New(ctx, testConfig)
	require.NoError(t, err, "failed to create cephmountfs for integration testing")

	return fs.(*cephmountfs)
}
