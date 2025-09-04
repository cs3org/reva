package cephmount

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathSecurityValidation(t *testing.T) {
	// Test the validatePathWithinBounds function with various scenarios
	// This test doesn't require Ceph integration as it tests the validation logic

	ctx := ContextWithTestLogger(t)

	// Create a test filesystem instance with known bounds
	tempDir, cleanup := GetTestDir(t, "security-validation-test")
	defer cleanup()

	config := map[string]interface{}{
		"testing_allow_local_mode": true,
	}

	fs := CreateCephMountFSForTesting(t, ctx, config, "/volumes/testuser", tempDir)

	t.Run("legitimate_paths_should_pass", func(t *testing.T) {
		legitimatePaths := []string{
			"/volumes/testuser/file.txt",
			"/volumes/testuser/subdir/file.txt",
			"/volumes/testuser/",
			"/volumes/testuser",
		}

		for _, path := range legitimatePaths {
			t.Run("allow_"+sanitizeTestName(path), func(t *testing.T) {
				err := fs.validatePathWithinBounds(ctx, path, "test")
				assert.NoError(t, err, "Should allow legitimate path: %s", path)
			})
		}
	})

	t.Run("traversal_paths_should_fail", func(t *testing.T) {
		maliciousPaths := []string{
			"../../etc/passwd",
			"/volumes/../../../etc/shadow",
			"/volumes/testuser/../../../root/.ssh/id_rsa",
			"/volumes/testuser/../../etc/hosts",
			"../malicious/file.txt",
			"/volumes/../admin/secret.txt",
		}

		for _, path := range maliciousPaths {
			t.Run("reject_"+sanitizeTestName(path), func(t *testing.T) {
				err := fs.validatePathWithinBounds(ctx, path, "test")
				assert.Error(t, err, "Should reject malicious path: %s", path)
				t.Logf("Correctly rejected: %s", path)
			})
		}
	})

	t.Run("paths_outside_volume_should_fail", func(t *testing.T) {
		outsidePaths := []string{
			"/volumes/otheruser/file.txt",
			"/different/volume/file.txt",
			"/root/system/file.txt",
			"/etc/passwd",
			"/var/log/system.log",
		}

		for _, path := range outsidePaths {
			t.Run("reject_outside_"+sanitizeTestName(path), func(t *testing.T) {
				err := fs.validatePathWithinBounds(ctx, path, "test")
				assert.Error(t, err, "Should reject path outside volume bounds: %s", path)
				t.Logf("Correctly rejected path outside bounds: %s", path)
			})
		}
	})

	t.Run("edge_cases", func(t *testing.T) {
		edgeCases := []struct {
			path      string
			shouldErr bool
			desc      string
		}{
			{"/volumes/testuser", false, "exact_volume_match"},
			{"/volumes/testuser/", false, "volume_with_trailing_slash"},
			{"/volumes/testuser/../testuser/file.txt", false, "indirect_traversal_to_same_location"}, // This resolves to legitimate path
			{"/volumes/testuser/./file.txt", false, "current_dir_reference"},
		}

		for _, tc := range edgeCases {
			t.Run(tc.desc, func(t *testing.T) {
				err := fs.validatePathWithinBounds(ctx, tc.path, "test")
				if tc.shouldErr {
					assert.Error(t, err, "Should reject edge case: %s", tc.path)
				} else {
					assert.NoError(t, err, "Should allow edge case: %s", tc.path)
				}
			})
		}
	})

	t.Log("Path security validation tests completed")
	t.Log("The validatePathWithinBounds function correctly:")
	t.Log("  1. Allows legitimate paths within volume bounds")
	t.Log("  2. Rejects path traversal attempts")
	t.Log("  3. Prevents access outside configured volume")
	t.Log("  4. Handles edge cases appropriately")
}

// sanitizeTestName converts a path to a valid test name
func sanitizeTestName(path string) string {
	// Replace problematic characters for test names
	result := path
	result = strings.ReplaceAll(result, "/", "_")
	result = strings.ReplaceAll(result, ".", "_")
	result = strings.ReplaceAll(result, "..", "dotdot")
	if result == "" {
		result = "empty"
	}
	if result[0] == '_' {
		result = "root" + result
	}
	return result
}
