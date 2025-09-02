//go:build ceph

// Package nceph Ceph integration benchmarks
//
// This file contains benchmark tests that run against real CephFS mounts when built with --tags ceph.
// These benchmarks provide more realistic performance measurements compared to local disk benchmarks.
//
// Available benchmarks:
// - BenchmarkGetMD_SingleFile_Ceph: Tests GetMD performance on a single file on real CephFS
// - BenchmarkGetMD_MultipleFiles_Ceph: Tests GetMD performance across different numbers of files on CephFS
// - BenchmarkGetMD_NestedDirectories_Ceph: Tests GetMD performance at different directory depths on CephFS
// - BenchmarkGetMD_WithMetadataKeys_Ceph: Tests GetMD performance with different metadata key sets on CephFS
// - BenchmarkGetMD_DirectoryOperations_Ceph: Tests GetMD performance on CephFS directories with varying content
//
// Prerequisites:
//   - NCEPH_FSTAB_ENTRY environment variable must be set with a valid CephFS fstab entry
//   - Access to a running Ceph cluster
//   - Proper authentication credentials
//
// Usage examples:
//   go test --tags ceph -bench=BenchmarkGetMD_SingleFile_Ceph ./pkg/storage/fs/nceph
//   go test --tags ceph -bench=BenchmarkGetMD_.*_Ceph ./pkg/storage/fs/nceph  # Run all Ceph benchmarks
//
// Environment variables:
//   NCEPH_FSTAB_ENTRY: Required - Complete fstab entry for CephFS mount
//   NCEPH_TEST_DIR: Optional - Base directory for benchmark tests on CephFS
//   NCEPH_TEST_PRESERVE: Set to "true" to preserve test directories after benchmarks

package nceph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/require"
)

// BenchmarkGetMD_SingleFile_Ceph benchmarks GetMD operations on a single file using real CephFS
func BenchmarkGetMD_SingleFile_Ceph(b *testing.B) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, "benchmark-getmd-single-ceph")
	defer cleanup()

	// Create test file on CephFS mount
	testFile := filepath.Join(testDir, "benchmark_file.txt")
	err := os.WriteFile(testFile, []byte("benchmark test content on ceph"), 0644)
	require.NoError(b, err, "Failed to create test file on CephFS")

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// File reference - use the path relative to the benchmark test directory
	// The ncephfs should see this as a file in its configured volume
	relativePath := "/benchmark-tests/" + filepath.Base(testDir) + "/benchmark_file.txt"
	ref := &provider.Reference{Path: relativePath}

	// Warm up - ensure everything works
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed during CephFS benchmark:", err)
		}
	}
}

// BenchmarkGetMD_MultipleFiles_Ceph benchmarks GetMD operations across multiple files on CephFS
func BenchmarkGetMD_MultipleFiles_Ceph(b *testing.B) {
	// Test with different file counts
	fileCounts := []int{10, 50, 100, 500}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("Files_%d", fileCount), func(b *testing.B) {
			benchmarkGetMDMultipleFilesCeph(b, fileCount)
		})
	}
}

func benchmarkGetMDMultipleFilesCeph(b *testing.B, fileCount int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-getmd-%d-ceph", fileCount))
	defer cleanup()

	// Create multiple test files on CephFS
	fileRefs := make([]*provider.Reference, fileCount)
	testDirName := filepath.Base(testDir)
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file_%04d.txt", i)
		filePath := filepath.Join(testDir, fileName)
		content := fmt.Sprintf("Content for file %d on CephFS", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create test file %d on CephFS", i)
		
		// Use the correct path relative to the ncephfs root
		relativePath := "/benchmark-tests/" + testDirName + "/" + fileName
		fileRefs[i] = &provider.Reference{Path: relativePath}
	}

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Warm up - test a few files
	for i := 0; i < min(5, fileCount); i++ {
		_, err := fs.GetMD(ctx, fileRefs[i], nil)
		require.NoError(b, err, "Warmup GetMD failed for file %d on CephFS", i)
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Get metadata for random file
		fileIndex := i % fileCount
		_, err := fs.GetMD(ctx, fileRefs[fileIndex], nil)
		if err != nil {
			b.Fatalf("GetMD failed for file %d during CephFS benchmark: %v", fileIndex, err)
		}
	}
}

// BenchmarkGetMD_NestedDirectories_Ceph benchmarks GetMD operations on files in nested directories on CephFS
func BenchmarkGetMD_NestedDirectories_Ceph(b *testing.B) {
	// Test with different nesting depths
	depths := []int{1, 3, 5, 10}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("Depth_%d", depth), func(b *testing.B) {
			benchmarkGetMDNestedDirectoriesCeph(b, depth)
		})
	}
}

func benchmarkGetMDNestedDirectoriesCeph(b *testing.B, depth int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-nested-%d-ceph", depth))
	defer cleanup()

	// Create nested directory structure on CephFS
	currentDir := testDir
	pathSegments := []string{}
	
	for i := 0; i < depth; i++ {
		dirName := fmt.Sprintf("level_%d", i)
		currentDir = filepath.Join(currentDir, dirName)
		pathSegments = append(pathSegments, dirName)
		
		err := os.MkdirAll(currentDir, 0755)
		require.NoError(b, err, "Failed to create directory at level %d on CephFS", i)
	}

	// Create test file in the deepest directory
	fileName := "deep_file.txt"
	filePath := filepath.Join(currentDir, fileName)
	err := os.WriteFile(filePath, []byte("deep file content on ceph"), 0644)
	require.NoError(b, err, "Failed to create deep test file on CephFS")

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Build reference path - use correct path relative to ncephfs root
	testDirName := filepath.Base(testDir)
	refPath := "/benchmark-tests/" + testDirName + "/" + filepath.Join(append(pathSegments, fileName)...)
	ref := &provider.Reference{Path: refPath}

	// Warm up
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed for nested file on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed for nested file during CephFS benchmark:", err)
		}
	}
}

// BenchmarkGetMD_WithMetadataKeys_Ceph benchmarks GetMD operations with different metadata keys on CephFS
func BenchmarkGetMD_WithMetadataKeys_Ceph(b *testing.B) {
	// Test with different metadata key sets
	metadataTests := []struct {
		name string
		keys []string
	}{
		{"NoKeys", nil},
		{"EmptyKeys", []string{}},
		{"BasicKeys", []string{"size", "mtime"}},
		{"AllCommonKeys", []string{"size", "mtime", "etag", "permissions", "checksum"}},
	}

	for _, test := range metadataTests {
		b.Run(test.name, func(b *testing.B) {
			benchmarkGetMDWithMetadataKeysCeph(b, test.keys)
		})
	}
}

func benchmarkGetMDWithMetadataKeysCeph(b *testing.B, mdKeys []string) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, "benchmark-metadata-keys-ceph")
	defer cleanup()

	// Create test file on CephFS
	testFile := filepath.Join(testDir, "metadata_test.txt")
	err := os.WriteFile(testFile, []byte("metadata benchmark content with some data on ceph"), 0644)
	require.NoError(b, err, "Failed to create test file on CephFS")

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// File reference - use correct path relative to ncephfs root
	testDirName := filepath.Base(testDir)
	relativePath := "/benchmark-tests/" + testDirName + "/metadata_test.txt"
	ref := &provider.Reference{Path: relativePath}

	// Warm up
	_, err = fs.GetMD(ctx, ref, mdKeys)
	require.NoError(b, err, "Warmup GetMD failed on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, mdKeys)
		if err != nil {
			b.Fatal("GetMD failed during CephFS benchmark:", err)
		}
	}
}

// BenchmarkGetMD_DirectoryOperations_Ceph benchmarks GetMD operations on directories on CephFS
func BenchmarkGetMD_DirectoryOperations_Ceph(b *testing.B) {
	// Test with directories containing different numbers of files
	fileCounts := []int{0, 10, 100, 1000}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("DirWith_%d_Files", fileCount), func(b *testing.B) {
			benchmarkGetMDDirectoryOperationsCeph(b, fileCount)
		})
	}
}

func benchmarkGetMDDirectoryOperationsCeph(b *testing.B, fileCount int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-dir-%d-ceph", fileCount))
	defer cleanup()

	// Create subdirectory with files on CephFS
	subDir := filepath.Join(testDir, "test_directory")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(b, err, "Failed to create subdirectory on CephFS")

	// Create files in subdirectory
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file_%04d.txt", i)
		filePath := filepath.Join(subDir, fileName)
		content := fmt.Sprintf("File %d content on CephFS", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create file %d on CephFS", i)
	}

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Directory reference - use correct path relative to ncephfs root
	testDirName := filepath.Base(testDir)
	relativePath := "/benchmark-tests/" + testDirName + "/test_directory"
	ref := &provider.Reference{Path: relativePath}

	// Warm up
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed for directory on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed for directory during CephFS benchmark:", err)
		}
	}
}

// Helper functions for Ceph benchmarks

// requireCephIntegrationForBenchmark checks if Ceph integration is available for benchmarks
func requireCephIntegrationForBenchmark(b *testing.B) {
	if os.Getenv("NCEPH_FSTAB_ENTRY") == "" {
		b.Skip("NCEPH_FSTAB_ENTRY not set - skipping Ceph integration benchmark")
	}
}

// setupCephBenchmark creates a CephFS-based filesystem and test directory for benchmarks
func setupCephBenchmark(b *testing.B, prefix string) (*ncephfs, string, func()) {
	// Get fstab entry from environment
	fstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY")
	if fstabEntry == "" {
		b.Skip("NCEPH_FSTAB_ENTRY environment variable not set")
	}

	// Parse the mount point from fstab entry
	// Format: "server:port:/path /mnt/point ceph options"
	parts := strings.Fields(fstabEntry)
	if len(parts) < 3 {
		b.Fatalf("Invalid fstab entry format: %s", fstabEntry)
	}
	mountPoint := parts[1] // /mnt/miniflax

	// Create test directory on the mounted CephFS
	testDir := filepath.Join(mountPoint, "benchmark-tests", prefix)
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		b.Fatalf("Failed to create test directory on CephFS mount %s: %v", testDir, err)
	}

	// Create filesystem instance using real CephFS integration
	config := map[string]interface{}{
		"fstabentry": fstabEntry,
		// Don't set allow_local_mode - use real CephFS
	}

	// Create the filesystem using integration helper
	ctx := contextWithBenchmarkLogger(b)
	fs := createNcephFSForCephBenchmark(b, ctx, config)

	// Cleanup function
	cleanup := func() {
		if os.Getenv("NCEPH_TEST_PRESERVE") != "true" {
			if err := os.RemoveAll(testDir); err != nil {
				b.Logf("Warning: failed to remove test dir %s: %v", testDir, err)
			}
		}
	}

	return fs, testDir, cleanup
}

// createNcephFSForCephBenchmark creates an ncephfs instance for Ceph benchmarks
func createNcephFSForCephBenchmark(b *testing.B, ctx context.Context, config map[string]interface{}) *ncephfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}

	// Do NOT set allow_local_mode for Ceph benchmarks - we want real CephFS

	// Create the filesystem using the standard New function with real fstab entry
	fs, err := New(ctx, testConfig)
	if err != nil {
		b.Fatalf("failed to create ncephfs for Ceph benchmark: %v", err)
	}

	return fs.(*ncephfs)
}
