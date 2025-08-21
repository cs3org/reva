//go:build ceph

package nceph

import (
	"os"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCephAdminConnection(t *testing.T) {
	// This test only verifies that we can establish a Ceph admin connection
	RequireCephIntegration(t)

	ctx := ContextWithTestLogger(t)

	// Get Ceph configuration from environment or defaults
	cephConfig := GetCephConfig()

	// Create Options struct
	config := &Options{
		CephConfig:   cephConfig["ceph_config"].(string),
		CephClientID: cephConfig["ceph_client_id"].(string),
		CephKeyring:  cephConfig["ceph_keyring"].(string),
		CephRoot:     cephConfig["ceph_root"].(string),
	}

	// Test creating a real ceph admin connection
	conn, err := newCephAdminConn(ctx, config)

	if err != nil {
		// Should not be configuration-related errors since we validated the config
		assert.NotContains(t, err.Error(), "not enabled", "Should not get 'not enabled' error with proper config")
		assert.NotContains(t, err.Error(), "build with -tags ceph", "Should not get build tag error with proper config")
		assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get incomplete config error with proper config")

		// Any other error means the connection failed - this is a test failure
		t.Fatalf("Failed to create Ceph admin connection with valid configuration: %v", err)
	} else {
		// Successfully created connection
		t.Log("Successfully created Ceph admin connection")
		// Clean up
		if conn != nil && conn.radosConn != nil {
			conn.radosConn.Shutdown()
		}
		if conn != nil && conn.adminMount != nil {
			conn.adminMount.Release()
		}
	}
}

func TestGetPathByIDIntegration(t *testing.T) {
	// This test verifies the complete GetPathByID functionality
	// It should fail for ANY error - connection issues, privilege issues, etc.
	RequireCephIntegration(t)

	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-pathbyid-test")
	defer cleanup()

	// Get Ceph configuration from environment or defaults
	config := GetCephConfig()
	config["root"] = tempDir // Override root with our test directory

	// Initialize nceph with ceph configuration
	ctx := ContextWithTestLogger(t)
	fs, err := New(ctx, config)
	require.NoError(t, err, "Failed to create nceph filesystem with Ceph configuration")
	require.NotNil(t, fs)

	// Test GetPathByID - this should work without any errors
	// If it fails for any reason (connection, privileges, file not found, etc.), the test fails
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "1"})

	if err != nil {
		// ANY error is a test failure - we expect GetPathByID to work properly
		t.Fatalf("GetPathByID failed. This indicates issues with Ceph connection, privileges, or configuration: %v", err)
	} else {
		t.Log("GetPathByID succeeded - Ceph integration is working correctly")
	}
}

func TestCephRootConfiguration(t *testing.T) {
	// This test verifies that the Ceph root parameter is properly configured
	RequireCephIntegration(t)

	ctx := ContextWithTestLogger(t)

	// Test with default root
	t.Run("default_root", func(t *testing.T) {
		config := GetCephConfig()
		assert.Contains(t, config, "ceph_root", "Config should contain ceph_root")
		assert.Equal(t, "/", config["ceph_root"], "Default ceph_root should be /")
	})

	// Test with custom root
	t.Run("custom_root", func(t *testing.T) {
		customRoot := "/volumes/test_data"
		config := GetCephConfigWithRoot(customRoot)
		assert.Equal(t, customRoot, config["ceph_root"], "Custom ceph_root should be set correctly")

		// Verify we can create Options struct with custom root
		options := &Options{
			CephConfig:   config["ceph_config"].(string),
			CephClientID: config["ceph_client_id"].(string),
			CephKeyring:  config["ceph_keyring"].(string),
			CephRoot:     config["ceph_root"].(string),
		}

		assert.Equal(t, customRoot, options.CephRoot, "Options.CephRoot should match custom root")

		// Test that we can create a connection with custom root
		// (This may fail due to connection issues, but shouldn't fail due to config)
		conn, err := newCephAdminConn(ctx, options)
		if err != nil {
			t.Logf("Connection failed with custom root (may be expected): %v", err)
			// Verify it's not a configuration error
			assert.NotContains(t, err.Error(), "incomplete ceph configuration", "Should not get config error")
		} else {
			t.Logf("Successfully created connection with custom Ceph root: %s", customRoot)
			// Clean up
			if conn != nil {
				_ = conn.Close(ctx)
			}
		}
	})

	// Test with environment variable override
	t.Run("environment_override", func(t *testing.T) {
		// This test verifies that NCEPH_CEPH_ROOT environment variable works
		// Note: This test depends on the environment variable being set externally
		envRoot := os.Getenv("NCEPH_CEPH_ROOT")
		if envRoot == "" {
			t.Skip("NCEPH_CEPH_ROOT not set, skipping environment override test")
		}

		config := GetCephConfig()
		assert.Equal(t, envRoot, config["ceph_root"], "Ceph root should match environment variable")
		t.Logf("Using Ceph root from environment: %s", envRoot)
	})
}
