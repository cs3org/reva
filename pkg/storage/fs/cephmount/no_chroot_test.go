package cephmount

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoChrootDirectoryError verifies proper error handling when no chroot directory is available
func TestNoChrootDirectoryError(t *testing.T) {
	// Clear any existing environment variable
	originalValue := os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR")
	os.Unsetenv("CEPHMOUNT_TEST_CHROOT_DIR")
	defer func() {
		if originalValue != "" {
			os.Setenv("CEPHMOUNT_TEST_CHROOT_DIR", originalValue)
		}
	}()

	// Try to create filesystem without fstabentry and without environment variable
	config := map[string]interface{}{
		"testing_allow_local_mode": true, // Local mode but no chroot source
	}

	ctx := ContextWithTestLogger(t)
	fs, err := New(ctx, config)

	// Should fail with appropriate error
	require.Error(t, err, "Should fail when no chroot directory is available")
	require.Nil(t, fs, "Filesystem should be nil on error")

	// Check error message
	assert.Contains(t, err.Error(), "no chroot directory available", "Error should mention missing chroot directory")
	assert.Contains(t, err.Error(), "CEPHMOUNT_TEST_CHROOT_DIR", "Error should mention the environment variable option")

	t.Logf("Properly handles case with no chroot directory")
	t.Logf("   Error: %s", err.Error())
}
