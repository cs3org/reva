//go:build ceph

package nceph

import (
	"context"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathByIDWithCeph(t *testing.T) {
	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-ceph-test")
	defer cleanup()

	// Initialize nceph with ceph configuration
	ctx := context.Background()
	config := map[string]interface{}{
		"root":           tempDir,
		"ceph_config":    "/etc/ceph/ceph.conf", // This would normally point to a real config
		"ceph_client_id": "admin",
		"ceph_keyring":   "/etc/ceph/ceph.client.admin.keyring",
	}

	fs, err := New(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, fs)

	// Test that GetPathByID with ceph support is available (even if it fails due to no actual ceph cluster)
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "123"})

	// With ceph build tag, we expect either success or a ceph-specific error, not NotSupported
	if err != nil {
		// The error should NOT be NotSupported when ceph is enabled
		assert.NotContains(t, err.Error(), "build with -tags ceph")
		assert.NotContains(t, err.Error(), "ceph support not enabled")

		// It should be a ceph-related error since we don't have a real cluster
		t.Logf("Expected ceph-related error (no real cluster): %v", err)
	}
}

func TestCephAdminConnWithCeph(t *testing.T) {
	ctx := context.Background()

	// Test that newCephAdminConn is available with ceph build tag
	config := &Options{
		CephConfig:   "/etc/ceph/ceph.conf",
		CephClientID: "admin",
		CephKeyring:  "/etc/ceph/ceph.client.admin.keyring",
	}

	// This will likely fail since we don't have a real ceph cluster, but it should not return "not enabled"
	_, err := newCephAdminConn(ctx, config)

	if err != nil {
		// Should not be a "not enabled" error
		assert.NotContains(t, err.Error(), "not enabled")
		assert.NotContains(t, err.Error(), "build with -tags ceph")
		t.Logf("Expected ceph connection error (no real cluster): %v", err)
	}
}
