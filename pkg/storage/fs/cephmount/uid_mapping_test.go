package cephmount

import (
	"os"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIDMapping(t *testing.T) {
	// Create a temporary test directory
	tempDir, cleanup := GetTestDir(t, "cephmount-uid-mapping")
	defer cleanup()

	// Set environment variable to use tempDir as chroot
	originalChrootDir := os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR")
	os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", tempDir)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("CEPHMOUNT_TEST_CHROOT_DIR")
		} else {
			os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	ctx := ContextWithTestLogger(t)

	// Create a minimal cephmount filesystem for testing UID mapping
	config := map[string]any{
		"nobody_uid":               65534,
		"nobody_gid":               65534,
		"testing_allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	}

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)

	cephmountFS, ok := fs.(*cephmountfs)
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

		uid, gid := cephmountFS.threadPool.mapUserToUIDGID(rootUser)
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

		uid, gid := cephmountFS.threadPool.mapUserToUIDGID(regularUser)
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

		uid, gid := cephmountFS.threadPool.mapUserToUIDGID(nobodyUser)
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

		uid, gid := cephmountFS.threadPool.mapUserToUIDGID(userWithoutUID)
		assert.Equal(t, 1000, uid, "Non-root user with UID 0 should get default 1000")
		assert.Equal(t, 1000, gid, "Non-root user with GID 0 should get default 1000")
	})
}
