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
	"os"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
)

func TestNobodyUserFallback(t *testing.T) {
	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-nobody-test")
	defer cleanup()

	// Create nceph filesystem with custom nobody UID/GID
	customNobodyUID := 99999
	customNobodyGID := 99999

	config := map[string]interface{}{
		"root":       tempDir,
		"nobody_uid": customNobodyUID,
		"nobody_gid": customNobodyGID,
	}

	fs, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("Failed to create nceph filesystem: %v", err)
	}
	defer fs.Shutdown(context.Background())

	ncephFS := fs.(*ncephfs)

	// Test that configuration was applied correctly
	if ncephFS.conf.NobodyUID != customNobodyUID {
		t.Errorf("Expected nobody UID %d, got %d", customNobodyUID, ncephFS.conf.NobodyUID)
	}
	if ncephFS.conf.NobodyGID != customNobodyGID {
		t.Errorf("Expected nobody GID %d, got %d", customNobodyGID, ncephFS.conf.NobodyGID)
	}

	// Test thread pool configuration
	if ncephFS.threadPool.nobodyUID != customNobodyUID {
		t.Errorf("Expected thread pool nobody UID %d, got %d", customNobodyUID, ncephFS.threadPool.nobodyUID)
	}
	if ncephFS.threadPool.nobodyGID != customNobodyGID {
		t.Errorf("Expected thread pool nobody GID %d, got %d", customNobodyGID, ncephFS.threadPool.nobodyGID)
	}

	t.Logf("✅ Nobody user configuration applied correctly: UID=%d, GID=%d", customNobodyUID, customNobodyGID)
}

func TestNobodyUserMapping(t *testing.T) {
	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-nobody-mapping-test")
	defer cleanup()

	// Create nceph filesystem with default configuration
	config := map[string]interface{}{
		"root": tempDir,
	}

	fs, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("Failed to create nceph filesystem: %v", err)
	}
	defer fs.Shutdown(context.Background())

	ncephFS := fs.(*ncephfs)

	// Test mapping of nobody user
	nobodyUser := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "nceph-nobody",
			OpaqueId: "nobody",
		},
		Username:    "nobody",
		DisplayName: "Nobody User",
	}

	uid, gid := ncephFS.threadPool.mapUserToUIDGID(nobodyUser)

	// Should use the configured nobody UID/GID (default 65534)
	expectedUID := 65534
	expectedGID := 65534

	if uid != expectedUID {
		t.Errorf("Expected nobody user to map to UID %d, got %d", expectedUID, uid)
	}
	if gid != expectedGID {
		t.Errorf("Expected nobody user to map to GID %d, got %d", expectedGID, gid)
	}

	t.Logf("✅ Nobody user mapping works correctly: UID=%d, GID=%d", uid, gid)

	// Test mapping of regular user (should use default)
	regularUser := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "local",
			OpaqueId: "testuser",
		},
		Username:    "testuser",
		DisplayName: "Test User",
	}

	uid, gid = ncephFS.threadPool.mapUserToUIDGID(regularUser)

	// Should use the default fallback UID/GID (1000)
	expectedUID = 1000
	expectedGID = 1000

	if uid != expectedUID {
		t.Errorf("Expected regular user to map to UID %d, got %d", expectedUID, uid)
	}
	if gid != expectedGID {
		t.Errorf("Expected regular user to map to GID %d, got %d", expectedGID, gid)
	}

	t.Logf("✅ Regular user mapping works correctly: UID=%d, GID=%d", uid, gid)
}

func TestNobodyUserOperations(t *testing.T) {
	// Create test directory (configurable via NCEPH_TEST_DIR environment variable)
	tempDir, cleanup := GetTestDir(t, "nceph-nobody-ops-test")
	defer cleanup()

	// Set permissions for the temp directory to be widely accessible
	err := os.Chmod(tempDir, 0755)
	if err != nil {
		t.Fatalf("Failed to set permissions: %v", err)
	}

	// Try to change ownership to nobody user (65534), but don't fail if we can't
	// This is expected when running tests as non-root
	err = os.Chown(tempDir, 65534, 65534)
	if err != nil {
		// If we can't chown to nobody, chown to the current user so tests can continue
		// This is a reasonable fallback for testing purposes
		currentUID := os.Getuid()
		currentGID := os.Getgid()
		err = os.Chown(tempDir, currentUID, currentGID)
		if err != nil {
			t.Fatalf("Failed to change ownership to current user: %v", err)
		}
		t.Logf("Note: Could not chown to nobody user, using current user (%d:%d) instead", currentUID, currentGID)
	} else {
		// When running as root and we can chown to nobody, we need to ensure
		// that regular users can still write to the directory for the mixed test
		// Set more permissive permissions to allow both nobody and regular user operations
		err = os.Chmod(tempDir, 0777)
		if err != nil {
			t.Fatalf("Failed to set permissive permissions: %v", err)
		}
		t.Logf("Successfully set up directory for nobody user (65534:65534) with write access for all users")
	}

	// Create nceph filesystem
	config := map[string]interface{}{
		"root": tempDir,
	}

	fs, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("Failed to create nceph filesystem: %v", err)
	}
	defer fs.Shutdown(context.Background())

	// Create context without user (should trigger nobody user fallback)
	ctx := context.Background()

	// Verify no user is in context
	_, ok := appctx.ContextGetUser(ctx)
	if ok {
		t.Fatal("Expected no user in context, but found one")
	}

	// Test directory creation (should use nobody thread)
	ref := &provider.Reference{Path: "test-nobody-dir"}
	err = fs.CreateDir(ctx, ref)
	if err != nil {
		t.Fatalf("Failed to create directory with nobody user: %v", err)
	}

	// Verify directory was created
	expectedPath := filepath.Join(tempDir, "test-nobody-dir")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("Directory was not created")
	}

	t.Logf("✅ Directory creation with nobody user fallback works correctly")

	// Test file operations with nobody user
	ref = &provider.Reference{Path: "test-nobody-dir/testfile.txt"}

	// This should also use nobody thread since no user is in context
	ri, err := fs.GetMD(ctx, ref, []string{})
	if err == nil {
		t.Fatalf("Expected file not found error, but got resource info: %v", ri)
	}

	t.Logf("✅ File operations with nobody user fallback work correctly")

	// Test with context that has a regular user
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			Idp:      "local",
			OpaqueId: "testuser",
		},
		Username:    "testuser",
		DisplayName: "Test User",
	}

	ctxWithUser := appctx.ContextSetUser(ctx, user)

	// This should use the regular user thread (UID 1000)
	ref = &provider.Reference{Path: "test-user-dir"}
	err = fs.CreateDir(ctxWithUser, ref)
	if err != nil {
		t.Fatalf("Failed to create directory with regular user: %v", err)
	}

	// Verify directory was created
	expectedPath = filepath.Join(tempDir, "test-user-dir")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("Directory was not created")
	}

	t.Logf("✅ Operations with regular user work correctly alongside nobody fallback")
}
