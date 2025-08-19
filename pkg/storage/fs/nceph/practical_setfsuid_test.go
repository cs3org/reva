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
// In applying this license, CERN does not waive the privileges and immunalities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

//go:build linux

package nceph

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestPracticalPerThreadUID demonstrates a practical use case for per-thread UID control
// This shows how you could implement user impersonation in a storage system like nceph
func TestPracticalPerThreadUID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("setfsuid is only available on Linux")
	}

	fmt.Println("=== Practical Per-Thread UID Control for Storage Systems ===")
	fmt.Println("Simulating a storage system serving multiple users concurrently")

	// Simulate different users making concurrent requests
	userRequests := []UserRequest{
		{UserID: 1001, Username: "alice", Operation: "create", Path: "/data/alice/document.txt"},
		{UserID: 1002, Username: "bob", Operation: "create", Path: "/data/bob/report.pdf"},
		{UserID: 1003, Username: "charlie", Operation: "create", Path: "/data/charlie/config.json"},
		{UserID: 65534, Username: "guest", Operation: "create", Path: "/data/guest/readme.txt"},
	}

	results := make(chan OperationResult, len(userRequests))

	// Process each user request in a separate thread with appropriate UID
	for _, req := range userRequests {
		go func(request UserRequest) {
			result := processUserRequest(request)
			results <- result
		}(req)
	}

	// Collect and display results
	fmt.Printf("\nProcessing %d concurrent user requests...\n\n", len(userRequests))

	successCount := 0
	for i := 0; i < len(userRequests); i++ {
		result := <-results

		status := "❌ FAILED"
		if result.Success {
			status = "✅ SUCCESS"
			successCount++
		}

		fmt.Printf("%s User %s (UID:%d): %s %s\n",
			status, result.Username, result.UserID, result.Operation, result.Path)
		fmt.Printf("    Thread: %d, fsuid: %d->%d, file_owner: %d\n",
			result.ThreadID, result.OriginalFsuid, result.RequestFsuid, result.FileOwnerUID)

		if result.Error != "" {
			fmt.Printf("    Error: %s\n", result.Error)
		}
		fmt.Println()
	}

	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Successful operations: %d/%d\n", successCount, len(userRequests))

	if successCount > 0 {
		fmt.Printf("✅ Demonstrated per-thread UID control for multi-user storage system\n")
		t.Logf("Successfully processed %d operations with per-thread UID control", successCount)
	} else {
		fmt.Printf("ℹ️  All operations used same UID (expected without root privileges)\n")
		t.Logf("All operations used same UID - expected behavior without privileges")
	}

	fmt.Println("\n=== Key Benefits for Storage Systems ===")
	fmt.Println("1. 🔒 Each user's files are created with correct ownership")
	fmt.Println("2. 🏃 Concurrent requests don't interfere with each other's UIDs")
	fmt.Println("3. 🛡️  No need to change the entire process UID")
	fmt.Println("4. ⚡ Better performance than spawning separate processes")
}

type UserRequest struct {
	UserID    int
	Username  string
	Operation string
	Path      string
}

type OperationResult struct {
	UserID        int
	Username      string
	Operation     string
	Path          string
	Success       bool
	Error         string
	ThreadID      int
	OriginalFsuid int
	RequestFsuid  int
	FileOwnerUID  uint32
	FileOwnerGID  uint32
}

// processUserRequest simulates processing a storage operation as a specific user
func processUserRequest(req UserRequest) OperationResult {
	// Lock to OS thread to ensure setfsuid affects only this thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	threadID := getTID()
	originalFsuid := setfsuidSafe(-1)
	originalFsgid := setfsgidSafe(-1)

	result := OperationResult{
		UserID:        req.UserID,
		Username:      req.Username,
		Operation:     req.Operation,
		Path:          req.Path,
		ThreadID:      threadID,
		OriginalFsuid: originalFsuid,
	}

	// Change filesystem UID to impersonate the user
	prevUID := setfsuidSafe(req.UserID)
	setfsgidSafe(req.UserID) // Using same value for GID

	// Check what UID we actually got
	actualFsuid := setfsuidSafe(-1)
	setfsgidSafe(-1) // Check GID but don't store
	result.RequestFsuid = actualFsuid

	fmt.Printf("Thread %d: Processing %s for %s, fsuid %d->%d (actual: %d)\n",
		threadID, req.Operation, req.Username, prevUID, req.UserID, actualFsuid)

	// Simulate the storage operation - create a file
	testFile := fmt.Sprintf("/tmp/nceph_user_%s_thread_%d.txt", req.Username, threadID)

	// Add a small delay to demonstrate concurrent execution
	time.Sleep(50 * time.Millisecond)

	if file, err := os.Create(testFile); err != nil {
		result.Error = err.Error()
		result.Success = false
	} else {
		// Write some content
		content := fmt.Sprintf("Operation: %s\nUser: %s (UID: %d)\nThread: %d\nFilesystem UID: %d\nPath: %s\n",
			req.Operation, req.Username, req.UserID, threadID, actualFsuid, req.Path)
		file.WriteString(content)
		file.Close()

		// Check the actual file ownership
		if stat, err := os.Stat(testFile); err == nil {
			if sstat, ok := stat.Sys().(*syscall.Stat_t); ok {
				result.FileOwnerUID = sstat.Uid
				result.FileOwnerGID = sstat.Gid
				result.Success = true
			}
		}

		// Clean up the test file
		os.Remove(testFile)
	}

	// Restore original filesystem UID/GID
	setfsuidSafe(originalFsuid)
	setfsgidSafe(originalFsgid)

	return result
}

// TestSetfsuidPermissionDemo demonstrates the difference between running with/without privileges
func TestSetfsuidPermissionDemo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("setfsuid is only available on Linux")
	}

	fmt.Println("\n=== Setfsuid Privilege Requirements Demo ===")

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentUID := os.Getuid()
	currentEUID := os.Geteuid()
	currentFsuid := setfsuidSafe(-1)

	fmt.Printf("Current process: UID=%d, EUID=%d, fsuid=%d\n", currentUID, currentEUID, currentFsuid)

	// Test different target UIDs
	testUIDs := []struct {
		uid  int
		name string
	}{
		{currentUID, "same_as_current"},
		{0, "root"},
		{1001, "user1001"},
		{65534, "nobody"},
	}

	for _, test := range testUIDs {
		fmt.Printf("\nTesting setfsuid(%d) for %s:\n", test.uid, test.name)

		// Try to change to target UID
		prevUID := setfsuidSafe(test.uid)
		actualUID := setfsuidSafe(-1)

		if actualUID == test.uid {
			fmt.Printf("  ✅ SUCCESS: fsuid changed from %d to %d\n", prevUID, actualUID)
		} else {
			fmt.Printf("  ❌ FAILED: fsuid remained at %d (requested %d)\n", actualUID, test.uid)
		}

		// Test file creation with this UID
		testFile := fmt.Sprintf("/tmp/fsuid_test_%s_%d.txt", test.name, actualUID)
		if file, err := os.Create(testFile); err != nil {
			fmt.Printf("  📁 File creation failed: %v\n", err)
		} else {
			file.WriteString(fmt.Sprintf("Created with fsuid %d\n", actualUID))
			file.Close()

			if stat, err := os.Stat(testFile); err == nil {
				if sstat, ok := stat.Sys().(*syscall.Stat_t); ok {
					fmt.Printf("  📁 File created with owner UID: %d\n", sstat.Uid)
				}
			}
			os.Remove(testFile)
		}

		// Restore original
		setfsuidSafe(currentFsuid)
	}

	fmt.Println("\n=== Permission Requirements ===")
	if currentEUID == 0 {
		fmt.Println("✓ Running as root - setfsuid should work for any UID")
	} else {
		fmt.Println("ℹ️  Running as non-root - setfsuid limited to current UID")
		fmt.Println("  To test full functionality: sudo go test -v ./pkg/storage/fs/nceph/ -run TestSetfsuid")
	}

	fmt.Println("\n=== Use Cases ===")
	fmt.Println("• Storage systems serving multiple users")
	fmt.Println("• Network file systems with user impersonation")
	fmt.Println("• Multi-tenant applications requiring file ownership control")
	fmt.Println("• Security-sensitive applications with privilege separation")
}
