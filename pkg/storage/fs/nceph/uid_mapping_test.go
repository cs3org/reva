package nceph

import (
	"os"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIDMapping(t *testing.T) {
	// Create a temporary test directory
	tempDir, cleanup := GetTestDir(t, "nceph-uid-mapping")
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

	ctx := ContextWithTestLogger(t)

	// Create a minimal nceph filesystem for testing UID mapping
	config := map[string]interface{}{
		"nobody_uid":       65534,
		"nobody_gid":       65534,
		"allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	}

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)

	ncephFS, ok := fs.(*ncephfs)
	require.True(t, ok)

	t.Run("root_user_mapping", func(t *testing.T) {
		// Test that root user maps to UID 0
		rootUser := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "root",
				Idp:      "local",
			},
			Username:  "root",
			UidNumber: 0,
			GidNumber: 0,
		}

		uid, gid := ncephFS.threadPool.mapUserToUIDGID(rootUser)
		assert.Equal(t, 0, uid, "Root user should map to UID 0")
		assert.Equal(t, 0, gid, "Root user should map to GID 0")
	})

	t.Run("regular_user_mapping", func(t *testing.T) {
		// Test that a regular user maps to their specified UID/GID
		regularUser := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: 1234,
			GidNumber: 5678,
		}

		uid, gid := ncephFS.threadPool.mapUserToUIDGID(regularUser)
		assert.Equal(t, 1234, uid, "Regular user should map to their specified UID")
		assert.Equal(t, 5678, gid, "Regular user should map to their specified GID")
	})

	t.Run("nobody_user_mapping", func(t *testing.T) {
		// Test that nobody user maps to configured nobody UID/GID
		nobodyUser := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "nobody",
				Idp:      "local",
			},
			Username:  "nobody",
			UidNumber: 999, // This should be ignored
			GidNumber: 999, // This should be ignored
		}

		uid, gid := ncephFS.threadPool.mapUserToUIDGID(nobodyUser)
		assert.Equal(t, 65534, uid, "Nobody user should map to configured nobody UID")
		assert.Equal(t, 65534, gid, "Nobody user should map to configured nobody GID")
	})

	t.Run("user_without_uid_mapping", func(t *testing.T) {
		// Test that user without username-based mapping gets default 1000, 1000
		// These users have UID/GID 0 but aren't explicitly root by username
		userWithoutUID := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "nouid",
				Idp:      "local",
			},
			Username:  "nouid", // Not "root", so should get defaults despite UID 0
			UidNumber: 0,
			GidNumber: 0,
		}

		uid, gid := ncephFS.threadPool.mapUserToUIDGID(userWithoutUID)
		assert.Equal(t, 1000, uid, "Non-root user with UID 0 should get default 1000")
		assert.Equal(t, 1000, gid, "Non-root user with GID 0 should get default 1000")
	})
}
