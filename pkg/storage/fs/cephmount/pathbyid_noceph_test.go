//go:build !ceph

package cephmount

import (
	"os"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathByIDWithoutCeph(t *testing.T) {
	// Create test directory (configurable via CEPHMOUNT_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "cephmount-noceph-test")
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

	// Initialize cephmount without ceph configuration
	ctx := ContextWithTestLogger(t)
	config := map[string]any{
		"testing_allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	}

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)

	// Test that GetPathByID returns NotSupported when ceph is not enabled
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "123"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "build with -tags ceph")
	t.Logf("GetPathByID correctly returned NotSupported error: %v", err)
}

func TestCephAdminConnWithoutCeph(t *testing.T) {
	ctx := ContextWithTestLogger(t)

	// Test that newCephAdminConn returns NotSupported when ceph is not enabled
	config := &Options{
		FstabEntry: "cephfs.cephfs /mnt/cephfs ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.keyring,conf=/etc/ceph/ceph.conf 0 2",
	}

	_, err := newCephAdminConn(ctx, config)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
	t.Logf("newCephAdminConn correctly returned NotSupported error: %v", err)
}
