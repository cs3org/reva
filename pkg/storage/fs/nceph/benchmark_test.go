// Package nceph benchmarks
//
// This file contains benchmark tests for the nceph (Next CephFS) storage package,
// specifically focusing on metadata operations (GetMD) performance.
//
// Available benchmarks:
// - BenchmarkGetMD_SingleFile: Tests GetMD performance on a single file
// - BenchmarkGetMD_MultipleFiles: Tests GetMD performance across different numbers of files
// - BenchmarkGetMD_NestedDirectories: Tests GetMD performance at different directory depths
// - BenchmarkGetMD_WithMetadataKeys: Tests GetMD performance with different metadata key sets
// - BenchmarkGetMD_DirectoryOperations: Tests GetMD performance on directories with varying content
//
// Usage examples:
//   go test -bench=BenchmarkGetMD_SingleFile ./pkg/storage/fs/nceph
//   go test -bench=BenchmarkGetMD_MultipleFiles ./pkg/storage/fs/nceph
//   go test -bench=BenchmarkGetMD_ ./pkg/storage/fs/nceph  # Run all GetMD benchmarks
//
// Environment variables:
//   NCEPH_TEST_DIR: Base directory for benchmark tests (default: temp directory)
//   NCEPH_TEST_PRESERVE: Set to "true" to preserve test directories after benchmarks

package nceph

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/require"
)

// BenchmarkGetMD_SingleFile benchmarks GetMD operations on a single file
func BenchmarkGetMD_SingleFile(b *testing.B) {
	// Create temporary directory to simulate mount point
	tempDir, cleanup := getBenchmarkTestDir(b, "benchmark-getmd-single")
	defer cleanup()

	// Create test file
	testFile := filepath.Join(tempDir, "benchmark_file.txt")
	err := os.WriteFile(testFile, []byte("benchmark test content"), 0644)
	require.NoError(b, err, "Failed to create test file")

	// Set permissions
	err = os.Chmod(testFile, 0666)
	require.NoError(b, err, "Failed to set file permissions")
	err = os.Chmod(tempDir, 0777)
	require.NoError(b, err, "Failed to set directory permissions")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// File reference
	ref := &provider.Reference{Path: "/benchmark_file.txt"}

	// Warm up - ensure everything works
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed during benchmark:", err)
		}
	}
}

// BenchmarkGetMD_MultipleFiles benchmarks GetMD operations across multiple files
func BenchmarkGetMD_MultipleFiles(b *testing.B) {
	// Test with different file counts
	fileCounts := []int{10, 50, 100, 500}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("Files_%d", fileCount), func(b *testing.B) {
			benchmarkGetMDMultipleFiles(b, fileCount)
		})
	}
}

func benchmarkGetMDMultipleFiles(b *testing.B, fileCount int) {
	// Create temporary directory
	tempDir, cleanup := getBenchmarkTestDir(b, fmt.Sprintf("benchmark-getmd-%d", fileCount))
	defer cleanup()

	// Create multiple test files
	fileRefs := make([]*provider.Reference, fileCount)
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file_%04d.txt", i)
		filePath := filepath.Join(tempDir, fileName)
		content := fmt.Sprintf("Content for file %d", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create test file %d", i)
		
		err = os.Chmod(filePath, 0666)
		require.NoError(b, err, "Failed to set permissions on file %d", i)
		
		fileRefs[i] = &provider.Reference{Path: "/" + fileName}
	}

	// Set directory permissions
	err := os.Chmod(tempDir, 0777)
	require.NoError(b, err, "Failed to set directory permissions")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Warm up - test a few files
	for i := 0; i < min(5, fileCount); i++ {
		_, err := fs.GetMD(ctx, fileRefs[i], nil)
		require.NoError(b, err, "Warmup GetMD failed for file %d", i)
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Get metadata for random file
		fileIndex := i % fileCount
		_, err := fs.GetMD(ctx, fileRefs[fileIndex], nil)
		if err != nil {
			b.Fatalf("GetMD failed for file %d during benchmark: %v", fileIndex, err)
		}
	}
}

// BenchmarkGetMD_NestedDirectories benchmarks GetMD operations on files in nested directories
func BenchmarkGetMD_NestedDirectories(b *testing.B) {
	// Test with different nesting depths
	depths := []int{1, 3, 5, 10}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("Depth_%d", depth), func(b *testing.B) {
			benchmarkGetMDNestedDirectories(b, depth)
		})
	}
}

func benchmarkGetMDNestedDirectories(b *testing.B, depth int) {
	// Create temporary directory
	tempDir, cleanup := getBenchmarkTestDir(b, fmt.Sprintf("benchmark-nested-%d", depth))
	defer cleanup()

	// Create nested directory structure
	currentDir := tempDir
	pathSegments := []string{}
	
	for i := 0; i < depth; i++ {
		dirName := fmt.Sprintf("level_%d", i)
		currentDir = filepath.Join(currentDir, dirName)
		pathSegments = append(pathSegments, dirName)
		
		err := os.MkdirAll(currentDir, 0777)
		require.NoError(b, err, "Failed to create directory at level %d", i)
	}

	// Create test file in the deepest directory
	fileName := "deep_file.txt"
	filePath := filepath.Join(currentDir, fileName)
	err := os.WriteFile(filePath, []byte("deep file content"), 0644)
	require.NoError(b, err, "Failed to create deep test file")
	
	err = os.Chmod(filePath, 0666)
	require.NoError(b, err, "Failed to set permissions on deep file")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Build reference path
	refPath := "/" + filepath.Join(append(pathSegments, fileName)...)
	ref := &provider.Reference{Path: refPath}

	// Warm up
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed for nested file")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed for nested file during benchmark:", err)
		}
	}
}

// BenchmarkGetMD_WithMetadataKeys benchmarks GetMD operations with different metadata keys
func BenchmarkGetMD_WithMetadataKeys(b *testing.B) {
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
			benchmarkGetMDWithMetadataKeys(b, test.keys)
		})
	}
}

func benchmarkGetMDWithMetadataKeys(b *testing.B, mdKeys []string) {
	// Create temporary directory
	tempDir, cleanup := getBenchmarkTestDir(b, "benchmark-metadata-keys")
	defer cleanup()

	// Create test file
	testFile := filepath.Join(tempDir, "metadata_test.txt")
	err := os.WriteFile(testFile, []byte("metadata benchmark content with some data"), 0644)
	require.NoError(b, err, "Failed to create test file")

	err = os.Chmod(testFile, 0666)
	require.NoError(b, err, "Failed to set file permissions")
	err = os.Chmod(tempDir, 0777)
	require.NoError(b, err, "Failed to set directory permissions")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// File reference
	ref := &provider.Reference{Path: "/metadata_test.txt"}

	// Warm up
	_, err = fs.GetMD(ctx, ref, mdKeys)
	require.NoError(b, err, "Warmup GetMD failed")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, mdKeys)
		if err != nil {
			b.Fatal("GetMD failed during benchmark:", err)
		}
	}
}

// BenchmarkGetMD_DirectoryOperations benchmarks GetMD operations on directories
func BenchmarkGetMD_DirectoryOperations(b *testing.B) {
	// Test with directories containing different numbers of files
	fileCounts := []int{0, 10, 100, 1000}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("DirWith_%d_Files", fileCount), func(b *testing.B) {
			benchmarkGetMDDirectoryOperations(b, fileCount)
		})
	}
}

func benchmarkGetMDDirectoryOperations(b *testing.B, fileCount int) {
	// Create temporary directory
	tempDir, cleanup := getBenchmarkTestDir(b, fmt.Sprintf("benchmark-dir-%d", fileCount))
	defer cleanup()

	// Create subdirectory with files
	subDir := filepath.Join(tempDir, "test_directory")
	err := os.MkdirAll(subDir, 0777)
	require.NoError(b, err, "Failed to create subdirectory")

	// Create files in subdirectory
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file_%04d.txt", i)
		filePath := filepath.Join(subDir, fileName)
		content := fmt.Sprintf("File %d content", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create file %d", i)
	}

	// Set permissions
	err = os.Chmod(tempDir, 0777)
	require.NoError(b, err, "Failed to set root directory permissions")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Directory reference
	ref := &provider.Reference{Path: "/test_directory"}

	// Warm up
	_, err = fs.GetMD(ctx, ref, nil)
	require.NoError(b, err, "Warmup GetMD failed for directory")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			b.Fatal("GetMD failed for directory during benchmark:", err)
		}
	}
}

// BenchmarkGetMD_Concurrent benchmarks concurrent GetMD operations
// Note: This benchmark is disabled due to context/thread safety issues
// TODO: Fix concurrent access patterns for benchmarking
/*
func BenchmarkGetMD_Concurrent(b *testing.B) {
	// Test with different levels of concurrency
	concurrencyLevels := []int{1, 2, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			benchmarkGetMDConcurrent(b, concurrency)
		})
	}
}

func benchmarkGetMDConcurrent(b *testing.B, concurrency int) {
	// Create temporary directory
	tempDir, cleanup := getBenchmarkTestDir(b, fmt.Sprintf("benchmark-concurrent-%d", concurrency))
	defer cleanup()

	// Create test files for concurrent access
	fileCount := concurrency * 10 // Multiple files per goroutine
	fileRefs := make([]*provider.Reference, fileCount)
	
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("concurrent_file_%04d.txt", i)
		filePath := filepath.Join(tempDir, fileName)
		content := fmt.Sprintf("Concurrent test content %d", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create concurrent test file %d", i)
		
		err = os.Chmod(filePath, 0666)
		require.NoError(b, err, "Failed to set permissions on file %d", i)
		
		fileRefs[i] = &provider.Reference{Path: "/" + fileName}
	}

	err := os.Chmod(tempDir, 0777)
	require.NoError(b, err, "Failed to set directory permissions")

	// Create filesystem instance
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := createNcephFSForBenchmark(b, contextWithBenchmarkLogger(b), config, "/volumes/_nogroup/benchmark", tempDir)

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Warm up
	_, err = fs.GetMD(ctx, fileRefs[0], nil)
	require.NoError(b, err, "Warmup GetMD failed")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		fileIndex := 0
		for pb.Next() {
			// Cycle through available files
			ref := fileRefs[fileIndex%fileCount]
			fileIndex++
			
			_, err := fs.GetMD(ctx, ref, nil)
			if err != nil {
				b.Errorf("GetMD failed during concurrent benchmark: %v", err)
				return
			}
		}
	})
}
*/

// Helper function for min (Go 1.21+)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Benchmark-specific helper functions

// getBenchmarkTestDir creates a test directory for benchmarks
func getBenchmarkTestDir(b *testing.B, prefix string) (string, func()) {
	baseDir := os.Getenv("NCEPH_TEST_DIR")
	preserve := os.Getenv("NCEPH_TEST_PRESERVE") == "true"

	if baseDir == "" {
		// Use temporary directory as fallback
		tmpDir, err := os.MkdirTemp("", prefix)
		if err != nil {
			b.Fatalf("Failed to create temp dir: %v", err)
		}

		return tmpDir, func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				b.Logf("Warning: failed to remove temp dir %s: %v", tmpDir, err)
			}
		}
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		b.Fatalf("Failed to create base test dir %s: %v", baseDir, err)
	}

	// Create unique subdirectory within the base directory
	testDir, err := os.MkdirTemp(baseDir, prefix+"-")
	if err != nil {
		b.Fatalf("Failed to create test dir in %s: %v", baseDir, err)
	}

	b.Logf("Using benchmark test directory: %s", testDir)

	cleanup := func() {
		if preserve {
			b.Logf("Preserving benchmark test directory: %s", testDir)
			return
		}
		if err := os.RemoveAll(testDir); err != nil {
			b.Logf("Warning: failed to remove test dir %s: %v", testDir, err)
		}
	}

	return testDir, cleanup
}

// createNcephFSForBenchmark creates an ncephfs instance for benchmarks
func createNcephFSForBenchmark(b *testing.B, ctx context.Context, config map[string]interface{}, cephVolumePath string, localMountPoint string) *ncephfs {
	// Create a copy of the config to avoid modifying the original
	testConfig := make(map[string]interface{})
	for k, v := range config {
		testConfig[k] = v
	}
	// Benchmarks always use local mode and ignore real fstab entries
	testConfig["allow_local_mode"] = true
	// Don't set fstabentry for benchmarks - they should be isolated
	delete(testConfig, "fstabentry")

	// Set the test chroot directory environment variable for benchmarks
	originalChrootDir := os.Getenv("NCEPH_TEST_CHROOT_DIR")
	os.Setenv("NCEPH_TEST_CHROOT_DIR", localMountPoint)
	defer func() {
		if originalChrootDir == "" {
			os.Unsetenv("NCEPH_TEST_CHROOT_DIR")
		} else {
			os.Setenv("NCEPH_TEST_CHROOT_DIR", originalChrootDir)
		}
	}()

	// Create the filesystem using the standard New function
	fs, err := New(ctx, testConfig)
	if err != nil {
		b.Fatalf("failed to create ncephfs for benchmark: %v", err)
	}

	ncephFS := fs.(*ncephfs)

	// Override the discovered paths for benchmarks
	ncephFS.cephVolumePath = cephVolumePath
	ncephFS.localMountPoint = localMountPoint

	return ncephFS
}

// getBenchmarkTestUser returns the current user information for use in benchmarks
func getBenchmarkTestUser(b *testing.B) *userv1beta1.User {
	currentUser, err := user.Current()
	if err != nil {
		b.Fatalf("failed to get current user: %v", err)
	}
	
	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		b.Fatalf("failed to parse current user UID: %v", err)
	}
	
	gid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		b.Fatalf("failed to parse current user GID: %v", err)
	}

	return &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: currentUser.Username,
			Idp:      "local",
		},
		Username:  currentUser.Username,
		UidNumber: int64(uid),
		GidNumber: int64(gid),
	}
}

// contextWithBenchmarkLogger creates a context with test logger for benchmarks
func contextWithBenchmarkLogger(b *testing.B) context.Context {
	// Create a simple context for benchmarks - don't need extensive logging during benchmarks
	ctx := context.Background()
	// For benchmarks, we typically want minimal logging to avoid affecting performance
	// Use a null logger or minimal logger
	logger := appctx.GetLogger(ctx) // Will use the default logger from appctx
	return appctx.WithLogger(ctx, logger)
}
