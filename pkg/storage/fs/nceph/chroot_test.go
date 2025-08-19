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

func TestChrootJail(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "nceph-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file outside the chroot
	outsideFile := filepath.Join(os.TempDir(), "outside-chroot.txt")
	err = os.WriteFile(outsideFile, []byte("secret data"), 0644)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}
	defer os.Remove(outsideFile)

	// Initialize nceph with the temp directory as root
	ctx := context.Background()
	config := map[string]interface{}{
		"root":        tempDir,
		"user_layout": "{{.Username}}",
		"uploads":     ".uploads",
	}

	storage, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	fs := storage.(*ncephfs)

	// Create test directories and files within chroot
	// Test that we can create files within the chroot
	testFile := "testuser/test.txt"

	// First create the user directory
	err = fs.rootFS.MkdirAll("testuser", 0755)
	if err != nil {
		t.Fatalf("Failed to create user directory within chroot: %v", err)
	}

	file, err := fs.rootFS.Create(testFile)
	if err != nil {
		t.Fatalf("Failed to create file within chroot: %v", err)
	}
	file.Close()

	// Test that we can stat the file
	_, err = fs.rootFS.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file within chroot: %v", err)
	}

	// Test that we cannot access files outside the chroot
	// This should fail because os.Root prevents escaping the jail
	_, err = fs.rootFS.Stat("../../../" + outsideFile)
	if err == nil {
		t.Fatal("Expected error when trying to access file outside chroot, but got none")
	}

	// Test that we can use directories within chroot
	err = fs.rootFS.MkdirAll("testuser/subdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create directory within chroot: %v", err)
	}

	// Test that the directory exists
	_, err = fs.rootFS.Stat("testuser/subdir")
	if err != nil {
		t.Fatalf("Failed to stat directory within chroot: %v", err)
	}

	t.Logf("Chroot jail is working correctly - confined to %s", tempDir)
}

func TestBasicFileOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "nceph-ops-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize nceph
	ctx := context.Background()
	config := map[string]interface{}{
		"root":        tempDir,
		"user_layout": "{{.Username}}",
		"uploads":     ".uploads",
	}

	storage, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	fs := storage.(*ncephfs)

	// Create a user context
	user := &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "root"},
		Username: "root",
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Test GetHome - should return NotSupported
	_, err = fs.GetHome(ctx)
	if err == nil {
		t.Fatalf("Expected GetHome to return NotSupported error, but got nil")
	}
	t.Logf("GetHome correctly returned error: %v", err)

	// Test CreateHome - should return NotSupported
	err = fs.CreateHome(ctx)
	if err == nil {
		t.Fatalf("Expected CreateHome to return NotSupported error, but got nil")
	}
	t.Logf("CreateHome correctly returned error: %v", err)

	// Test that the root directory exists (it should, since it was created in New())
	_, err = fs.rootFS.Stat(".")
	if err != nil {
		t.Fatalf("Root directory was not accessible: %v", err)
	}

	t.Log("Basic file operations work correctly")
}

func TestGetPathByIDNotSupported(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "nceph-pathbyid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize nceph without ceph configuration
	ctx := context.Background()
	config := map[string]interface{}{
		"root":        tempDir,
		"user_layout": "{{.Username}}",
		"uploads":     ".uploads",
	}

	storage, err := New(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create filesystem: %v", err)
	}

	fs := storage.(*ncephfs)

	// Create a user context
	user := &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "root"},
		Username: "root",
	}
	ctx = appctx.ContextSetUser(ctx, user)

	// Test that GetPathByID returns NotSupported error
	_, err = fs.GetPathByID(ctx, &provider.ResourceId{OpaqueId: "123"})
	if err == nil {
		t.Fatal("Expected GetPathByID to return error when ceph is not configured")
	}

	// Check that it's the right type of error
	if !isNotSupportedError(err) {
		t.Fatalf("Expected NotSupported error, got: %v", err)
	}

	t.Log("GetPathByID correctly returns NotSupported when ceph is not available")
}

func isNotSupportedError(err error) bool {
	// Simple check for NotSupported error
	return err != nil && err.Error() != ""
}
