//go:build !ceph

package nceph

import (
	"context"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathByIDWithoutCeph(t *testing.T) {
	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-noceph-test")
	defer cleanup()

	// Initialize nceph without ceph configuration
	ctx := context.Background()
	config := map[string]interface{}{
		"root": tempDir,
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
	ctx := context.Background()

	// Test that newCephAdminConn returns NotSupported when ceph is not enabled
	config := &Options{
		CephConfig:   "/etc/ceph/ceph.conf",
		CephClientID: "admin",
		CephKeyring:  "/etc/ceph/ceph.client.admin.keyring",
	}

	_, err := newCephAdminConn(ctx, config)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
	t.Logf("newCephAdminConn correctly returned NotSupported error: %v", err)
}
