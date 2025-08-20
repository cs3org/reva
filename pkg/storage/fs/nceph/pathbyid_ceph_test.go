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
	// This test requires real Ceph configuration and the -ceph-integration flag
	RequireCephIntegration(t)

	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-ceph-test")
	defer cleanup()

	// Get Ceph configuration from environment or defaults
	config := GetCephConfig()
	config["root"] = tempDir // Override root with our test directory

	// Initialize nceph with ceph configuration
	ctx := context.Background()
	fs, err := New(ctx, config)
	require.NoError(t, err, "Failed to create nceph filesystem with Ceph configuration")
	require.NotNil(t, fs)

	// Test GetPathByID with a real Ceph connection
	// This should either succeed (if the ID exists) or fail with a legitimate Ceph error
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "1"})

	if err != nil {
		// With valid Ceph configuration, we should get legitimate Ceph errors, not "not supported" errors
		assert.NotContains(t, err.Error(), "build with -tags ceph", "Should not get build tag error with proper config")
		assert.NotContains(t, err.Error(), "ceph support not enabled", "Should not get support disabled error with proper config")
		assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get incomplete config error with proper config")

		// Log the actual error for debugging
		t.Logf("GetPathByID returned Ceph error (expected for non-existent ID): %v", err)
	} else {
		t.Log("GetPathByID succeeded - this means the resource ID '1' actually exists in the Ceph cluster")
	}
}

func TestCephAdminConnWithCeph(t *testing.T) {
	// This test requires real Ceph configuration and the -ceph-integration flag
	RequireCephIntegration(t)

	ctx := context.Background()

	// Get Ceph configuration from environment or defaults
	cephConfig := GetCephConfig()

	// Create Options struct
	config := &Options{
		CephConfig:   cephConfig["ceph_config"].(string),
		CephClientID: cephConfig["ceph_client_id"].(string),
		CephKeyring:  cephConfig["ceph_keyring"].(string),
	}

	// Test creating a real ceph admin connection
	conn, err := newCephAdminConn(ctx, config)

	if err != nil {
		// Should not be configuration-related errors since we validated the config
		assert.NotContains(t, err.Error(), "not enabled", "Should not get 'not enabled' error with proper config")
		assert.NotContains(t, err.Error(), "build with -tags ceph", "Should not get build tag error with proper config")
		assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get incomplete config error with proper config")

		// This could be a legitimate connection error (cluster not accessible, authentication issues, etc.)
		t.Fatalf("Failed to create Ceph admin connection with valid configuration: %v", err)
	} else {
		// Successfully created connection
		t.Log("Successfully created Ceph admin connection")
		// Clean up
		if conn != nil && conn.radosConn != nil {
			conn.radosConn.Shutdown()
		}
		if conn != nil && conn.fsMount != nil {
			conn.fsMount.Release()
		}
	}
}
