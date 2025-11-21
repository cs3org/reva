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

//go:build linux

package cephmount

import (
	"os"
	"slices"
	"testing"
)

func TestPrivilegeVerification(t *testing.T) {
	// Test privilege verification with standard nobody UID/GID
	result := VerifyPrivileges(65534, 65534)

	if result == nil {
		t.Fatal("PrivilegeVerificationResult should not be nil")
	}

	// Basic checks
	if result.CurrentUID != os.Getuid() {
		t.Errorf("Expected CurrentUID to be %d, got %d", os.Getuid(), result.CurrentUID)
	}

	if result.CurrentGID != os.Getgid() {
		t.Errorf("Expected CurrentGID to be %d, got %d", os.Getgid(), result.CurrentGID)
	}

	// Test that we at least tested some UIDs and GIDs
	if len(result.TestedUIDs) == 0 {
		t.Error("Expected at least one UID to be tested")
	}

	if len(result.TestedGIDs) == 0 {
		t.Error("Expected at least one GID to be tested")
	}

	t.Logf("Privilege Verification Results:")
	t.Logf("Current UID/GID: %d/%d", result.CurrentUID, result.CurrentGID)
	t.Logf("Current fsuid/fsgid: %d/%d", result.CurrentFsUID, result.CurrentFsGID)
	t.Logf("Can change UID: %t", result.CanChangeUID)
	t.Logf("Can change GID: %t", result.CanChangeGID)
	t.Logf("Tested UIDs: %v", result.TestedUIDs)
	t.Logf("Tested GIDs: %v", result.TestedGIDs)

	if len(result.ErrorMessages) > 0 {
		t.Logf("Error Messages:")
		for _, msg := range result.ErrorMessages {
			t.Logf("  - %s", msg)
		}
	}

	if len(result.Recommendations) > 0 {
		t.Logf("Recommendations:")
		for _, rec := range result.Recommendations {
			t.Logf("  - %s", rec)
		}
	}

	// Test string representation
	summary := result.String()
	if summary == "" {
		t.Error("String() should return a non-empty summary")
	}
	t.Logf("\nSummary:\n%s", summary)

	// For non-root users, we expect insufficient privileges
	if result.CurrentUID != 0 {
		if result.HasSufficientPrivileges() {
			t.Logf("Unexpected: Non-root user has sufficient privileges (may be running with capabilities)")
		} else {
			t.Logf("Expected: Non-root user has insufficient privileges")
		}
	} else {
		if result.HasSufficientPrivileges() {
			t.Logf("Expected: Root user has sufficient privileges")
		} else {
			t.Errorf("Unexpected: Root user should have sufficient privileges")
		}
	}
}

func TestThreadPoolPrivilegeVerification(t *testing.T) {
	// Create thread pool and verify privileges are checked during initialization
	config := UserThreadPoolConfig{
		NobodyUID: 99999, // Use a custom nobody UID for testing
		NobodyGID: 99999, // Use a custom nobody GID for testing
	}

	threadPool, privResult, err := NewUserThreadPool(config)
	if err != nil {
		t.Fatalf("Failed to create thread pool: %v", err)
	}
	defer threadPool.Shutdown()

	if privResult == nil {
		t.Fatal("PrivilegeVerificationResult should not be nil")
	}

	// Verify the result includes our custom nobody UID/GID in tests
	foundNobodyUID := slices.Contains(privResult.TestedUIDs, 99999)

	foundNobodyGID := slices.Contains(privResult.TestedGIDs, 99999)

	if !foundNobodyUID {
		t.Errorf("Expected custom nobody UID %d to be tested, tested UIDs: %v", 99999, privResult.TestedUIDs)
	}

	if !foundNobodyGID {
		t.Errorf("Expected custom nobody GID %d to be tested, tested GIDs: %v", 99999, privResult.TestedGIDs)
	}

	t.Logf("Thread pool initialization includes privilege verification")
	t.Logf("Privilege status: %s", func() string {
		if privResult.HasSufficientPrivileges() {
			return "SUFFICIENT"
		} else if privResult.HasPartialPrivileges() {
			return "PARTIAL"
		}
		return "INSUFFICIENT"
	}())
}

func TestCephMountPrivilegeVerificationIntegration(t *testing.T) {
	// Create test directory (configurable via CEPHMOUNT_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "cephmount-privilege-test")
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

	// Create cephmount filesystem - this should trigger privilege verification
	config := map[string]any{
		"nobody_uid":               65534,
		"nobody_gid":               65534,
		"testing_allow_local_mode": true, // Allow local mode for tests (bypasses auto-discovery)
	}

	// Capture log output during initialization (logs will show privilege status)
	ctx := ContextWithTestLogger(t)
	fs, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create cephmount filesystem: %v", err)
	}
	defer fs.Shutdown(ctx)

	t.Logf("CephMount filesystem created successfully with privilege verification")

	// The New() function should have logged the privilege verification results
	// In a real scenario, you'd check the logs, but for this test we just verify
	// that initialization succeeded even with potentially insufficient privileges
}

func TestPrivilegeVerificationEdgeCases(t *testing.T) {
	// Test with same UID/GID as current user
	currentUID := os.Getuid()
	currentGID := os.Getgid()

	result := VerifyPrivileges(currentUID, currentGID)

	// Should be able to "change" to the same UID/GID (no actual change)
	if !result.CanChangeUID {
		t.Logf("Cannot change to same UID %d (this may be expected)", currentUID)
	}

	if !result.CanChangeGID {
		t.Logf("Cannot change to same GID %d (this may be expected)", currentGID)
	}

	// Test with root UID/GID (if we're not root, this should fail)
	if currentUID != 0 {
		rootResult := VerifyPrivileges(0, 0)
		if rootResult.CanChangeUID {
			t.Logf("Unexpected: Non-root user can change to root UID")
		}
		if rootResult.CanChangeGID {
			t.Logf("Unexpected: Non-root user can change to root GID")
		}
	}

	t.Logf("Edge case testing completed")
}
