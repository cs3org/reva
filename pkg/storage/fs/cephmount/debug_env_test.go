//go:build ceph

package cephmount

import (
	"os"
	"testing"
)

// DebugEnvironment shows what environment variables are available for Ceph benchmarks
func TestDebugEnvironment(t *testing.T) {
	fstabEntry := os.Getenv("CEPHMOUNT_FSTAB_ENTRY")
	if fstabEntry == "" {
		t.Logf("CEPHMOUNT_FSTAB_ENTRY is not set")
	} else {
		t.Logf("CEPHMOUNT_FSTAB_ENTRY is set to: %s", fstabEntry)
	}

	// Also check if we can read /etc/fstab
	if data, err := os.ReadFile("/etc/fstab"); err != nil {
		t.Logf("Cannot read /etc/fstab: %v", err)
	} else {
		t.Logf("/etc/fstab contains %d bytes", len(data))
	}
}
