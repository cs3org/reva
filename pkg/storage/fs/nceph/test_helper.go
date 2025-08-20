// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package nceph

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

// GetTestDir returns a directory for testing. It checks for the NCEPH_TEST_DIR
// environment variable. If set, it creates a subdirectory within that path.
// If not set, it falls back to creating a temporary directory.
//
// The returned cleanup function should be called to remove the test directory
// unless NCEPH_TEST_DIR is set and NCEPH_TEST_PRESERVE is also set to "true".
//
// Usage:
//
//	testDir, cleanup := GetTestDir(t, "test-prefix")
//	defer cleanup()
//
// Environment variables:
//
//	NCEPH_TEST_DIR: Base directory for tests (e.g., "/mnt/ceph/test")
//	NCEPH_TEST_PRESERVE: If "true", preserves test directories when NCEPH_TEST_DIR is set
func GetTestDir(t *testing.T, prefix string) (string, func()) {
	baseDir := os.Getenv("NCEPH_TEST_DIR")
	preserve := os.Getenv("NCEPH_TEST_PRESERVE") == "true"

	if baseDir == "" {
		// Use temporary directory as fallback
		tmpDir, err := os.MkdirTemp("", prefix)
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		return tmpDir, func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("Warning: failed to remove temp dir %s: %v", tmpDir, err)
			}
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("Failed to create base test dir %s: %v", baseDir, err)
	}

	// Create unique subdirectory within the base directory
	testDir, err := os.MkdirTemp(baseDir, prefix+"-")
	if err != nil {
		t.Fatalf("Failed to create test dir in %s: %v", baseDir, err)
	}

	t.Logf("Using test directory: %s", testDir)

	cleanup := func() {
		if preserve {
			t.Logf("Preserving test directory: %s", testDir)
			return
		}
		if err := os.RemoveAll(testDir); err != nil {
			t.Logf("Warning: failed to remove test dir %s: %v", testDir, err)
		}
	}

	return testDir, cleanup
}

// SetupTestDir is a convenience function that calls GetTestDir and sets up
// appropriate permissions for the returned directory. It also handles the
// common pattern of changing ownership to allow test users to write.
//
// The uid and gid parameters specify the desired ownership. If uid is 0,
// no ownership change is attempted.
func SetupTestDir(t *testing.T, prefix string, uid, gid int) (string, func()) {
	testDir, cleanup := GetTestDir(t, prefix)

	// Set permissions
	if err := os.Chmod(testDir, 0755); err != nil {
		cleanup()
		t.Fatalf("Failed to set permissions on test dir %s: %v", testDir, err)
	}

	// Change ownership if requested and we're running as root
	if uid != 0 {
		if err := os.Chown(testDir, uid, gid); err != nil {
			// Only log warning, don't fail the test
			t.Logf("Warning: failed to change ownership of %s to %d:%d: %v", testDir, uid, gid, err)
		}
	}

	return testDir, cleanup
}

// ContextWithTestLogger creates a context with a configured logger for testing.
// This ensures that debug logs are visible during test runs.
//
// Usage:
//
//	ctx := ContextWithTestLogger(t)
//	fs, err := newCephAdminConn(ctx, config)
func ContextWithTestLogger(t *testing.T) context.Context {
	// Create a logger that outputs to the test log
	logger := zerolog.New(zerolog.NewTestWriter(t)).
		Level(zerolog.DebugLevel).
		With().
		Timestamp().
		Logger()

	// Create context with the logger
	ctx := appctx.WithLogger(context.Background(), &logger)
	return ctx
}

// GetTestSubDir creates a subdirectory within an existing test directory.
// This is useful when you need multiple directories for a single test.
func GetTestSubDir(t *testing.T, baseDir, subDirName string) string {
	subDir := filepath.Join(baseDir, subDirName)
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory %s: %v", subDir, err)
	}
	return subDir
}

var (
	cephIntegration = flag.Bool("ceph-integration", false, "Enable Ceph integration tests (requires valid Ceph configuration)")
)

// RequireCephIntegration checks if Ceph integration tests are enabled and configuration is valid.
// If not, it skips the test. If the integration flag is set but configuration is invalid, it fails the test.
//
// Usage:
//
//	func TestCephFeature(t *testing.T) {
//	    RequireCephIntegration(t)
//	    // ... rest of test
//	}
//
// Run with: go test -tags ceph -ceph-integration -v
func RequireCephIntegration(t *testing.T) {
	if !*cephIntegration {
		t.Skip("Ceph integration tests disabled. Use -ceph-integration flag to enable.")
	}

	// Check for required Ceph configuration
	if !ValidateCephConfig(t) {
		t.Fatal("Ceph integration tests enabled but invalid configuration. Please set NCEPH_CEPH_CONFIG, NCEPH_CEPH_CLIENT_ID, and NCEPH_CEPH_KEYRING environment variables or ensure /etc/ceph/ceph.conf exists.")
	}
}

// ValidateCephConfig checks if the required Ceph configuration is available.
// It returns true if configuration appears to be valid, false otherwise.
func ValidateCephConfig(t *testing.T) bool {
	// Check environment variables first
	cephConfig := os.Getenv("NCEPH_CEPH_CONFIG")
	cephClientID := os.Getenv("NCEPH_CEPH_CLIENT_ID")
	cephKeyring := os.Getenv("NCEPH_CEPH_KEYRING")

	if cephConfig != "" && cephClientID != "" && cephKeyring != "" {
		// Verify files exist
		if _, err := os.Stat(cephConfig); err != nil {
			t.Logf("Ceph config file not found: %s", cephConfig)
			return false
		}
		if _, err := os.Stat(cephKeyring); err != nil {
			t.Logf("Ceph keyring file not found: %s", cephKeyring)
			return false
		}
		return true
	}

	// Fallback to default locations
	defaultConfig := "/etc/ceph/ceph.conf"
	defaultKeyring := "/etc/ceph/ceph.client.admin.keyring"

	configExists := false
	keyringExists := false

	if _, err := os.Stat(defaultConfig); err == nil {
		configExists = true
	}
	if _, err := os.Stat(defaultKeyring); err == nil {
		keyringExists = true
	}

	if !configExists {
		t.Logf("Default Ceph config not found at %s and NCEPH_CEPH_CONFIG not set", defaultConfig)
	}
	if !keyringExists {
		t.Logf("Default Ceph keyring not found at %s and NCEPH_CEPH_KEYRING not set", defaultKeyring)
	}

	return configExists && keyringExists
}

// GetCephConfig returns the Ceph configuration to use for tests.
// It checks environment variables first, then falls back to defaults.
func GetCephConfig() map[string]interface{} {
	config := map[string]interface{}{}

	// Get base test directory
	if baseDir := os.Getenv("NCEPH_TEST_DIR"); baseDir != "" {
		config["root"] = baseDir
	}

	// Get Ceph configuration
	cephConfig := os.Getenv("NCEPH_CEPH_CONFIG")
	if cephConfig == "" {
		cephConfig = "/etc/ceph/ceph.conf"
	}
	config["ceph_config"] = cephConfig

	cephClientID := os.Getenv("NCEPH_CEPH_CLIENT_ID")
	if cephClientID == "" {
		cephClientID = "admin"
	}
	config["ceph_client_id"] = cephClientID

	cephKeyring := os.Getenv("NCEPH_CEPH_KEYRING")
	if cephKeyring == "" {
		cephKeyring = "/etc/ceph/ceph.client.admin.keyring"
	}
	config["ceph_keyring"] = cephKeyring

	return config
}
