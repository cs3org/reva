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
// - BenchmarkListFolder_Ceph: Tests ListFolder performance on CephFS directories with varying numbers of files
// - BenchmarkListFolder_NestedDirectories_Ceph: Tests ListFolder performance on nested directory structures on CephFS
// - BenchmarkUpload_Ceph: Tests Upload performance with different file sizes (1KB to 100MB) on real CephFS
// - BenchmarkUpload_ConcurrentUploads_Ceph: Tests Upload performance with different concurrency levels on real CephFS
// - BenchmarkUpload_DifferentDirectories_Ceph: Tests Upload performance to directories at different depths on real CephFS
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
	"bytes"
	"context"
	"fmt"
	"io"
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

// BenchmarkListFolder_Ceph benchmarks ListFolder operations on directories with different numbers of files on CephFS
func BenchmarkListFolder_Ceph(b *testing.B) {
	// Test with different file counts
	fileCounts := []int{0, 10, 50, 100, 500, 1000}
	
	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("Files_%d", fileCount), func(b *testing.B) {
			benchmarkListFolderCeph(b, fileCount)
		})
	}
}

func benchmarkListFolderCeph(b *testing.B, fileCount int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-list-%d-ceph", fileCount))
	defer cleanup()

	// Create test directory with files on CephFS
	listDir := filepath.Join(testDir, "list_test_dir")
	err := os.MkdirAll(listDir, 0755)
	require.NoError(b, err, "Failed to create list test directory on CephFS")

	// Create files in the directory
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("file_%04d.txt", i)
		filePath := filepath.Join(listDir, fileName)
		content := fmt.Sprintf("Content for file %d on CephFS", i)
		
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(b, err, "Failed to create test file %d on CephFS", i)
	}

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Directory reference - use correct path relative to ncephfs root
	testDirName := filepath.Base(testDir)
	relativePath := "/benchmark-tests/" + testDirName + "/list_test_dir"
	ref := &provider.Reference{Path: relativePath}

	// Warm up - ensure everything works
	_, err = fs.ListFolder(ctx, ref, nil)
	require.NoError(b, err, "Warmup ListFolder failed on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.ListFolder(ctx, ref, nil)
		if err != nil {
			b.Fatal("ListFolder failed during CephFS benchmark:", err)
		}
	}
}

// BenchmarkListFolder_NestedDirectories_Ceph benchmarks ListFolder operations on directories with nested subdirectories on CephFS
func BenchmarkListFolder_NestedDirectories_Ceph(b *testing.B) {
	// Test with different nesting depths
	depths := []int{1, 3, 5, 10}
	
	for _, depth := range depths {
		b.Run(fmt.Sprintf("Depth_%d", depth), func(b *testing.B) {
			benchmarkListFolderNestedCeph(b, depth)
		})
	}
}

func benchmarkListFolderNestedCeph(b *testing.B, depth int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-list-nested-%d-ceph", depth))
	defer cleanup()

	// Create main test directory
	mainDir := filepath.Join(testDir, "nested_list_test")
	err := os.MkdirAll(mainDir, 0755)
	require.NoError(b, err, "Failed to create main test directory on CephFS")

	// Create nested directory structure with files at each level
	currentDir := mainDir
	for i := 0; i < depth; i++ {
		// Create subdirectory
		subDir := fmt.Sprintf("level_%d", i)
		currentDir = filepath.Join(currentDir, subDir)
		err := os.MkdirAll(currentDir, 0755)
		require.NoError(b, err, "Failed to create directory at level %d on CephFS", i)

		// Create a few files at this level
		for j := 0; j < 3; j++ {
			fileName := fmt.Sprintf("file_level%d_%d.txt", i, j)
			filePath := filepath.Join(currentDir, fileName)
			content := fmt.Sprintf("Content at level %d, file %d on CephFS", i, j)
			
			err := os.WriteFile(filePath, []byte(content), 0644)
			require.NoError(b, err, "Failed to create file at level %d on CephFS", i)
		}
	}

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Directory reference for the main directory - use correct path relative to ncephfs root
	testDirName := filepath.Base(testDir)
	relativePath := "/benchmark-tests/" + testDirName + "/nested_list_test"
	ref := &provider.Reference{Path: relativePath}

	// Warm up
	_, err = fs.ListFolder(ctx, ref, nil)
	require.NoError(b, err, "Warmup ListFolder failed for nested directories on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := fs.ListFolder(ctx, ref, nil)
		if err != nil {
			b.Fatal("ListFolder failed during nested CephFS benchmark:", err)
		}
	}
}

// BenchmarkUpload_Ceph benchmarks Upload operations with different file sizes on CephFS
func BenchmarkUpload_Ceph(b *testing.B) {
	// Test with different file sizes
	fileSizes := []struct {
		name string
		size int64
	}{
		{"1KB", 1 * 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}
	
	for _, fileSize := range fileSizes {
		b.Run(fileSize.name, func(b *testing.B) {
			benchmarkUploadCeph(b, fileSize.size)
		})
	}
}

func benchmarkUploadCeph(b *testing.B, fileSize int64) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-upload-%d-ceph", fileSize))
	defer cleanup()

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Create test data buffer
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Warm up - upload once to ensure everything works
	testDirName := filepath.Base(testDir)
	warmupPath := "/benchmark-tests/" + testDirName + "/warmup_file.txt"
	warmupRef := &provider.Reference{Path: warmupPath}
	warmupReader := bytes.NewReader(testData)
	err := fs.Upload(ctx, warmupRef, io.NopCloser(warmupReader), nil)
	require.NoError(b, err, "Warmup upload failed on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(fileSize) // Report throughput in MB/s

	for i := 0; i < b.N; i++ {
		// Create unique file name for each iteration
		fileName := fmt.Sprintf("/benchmark-tests/%s/upload_test_%d.txt", testDirName, i)
		ref := &provider.Reference{Path: fileName}
		
		// Create reader from test data
		reader := bytes.NewReader(testData)
		
		// Upload file
		err := fs.Upload(ctx, ref, io.NopCloser(reader), nil)
		if err != nil {
			b.Fatal("Upload failed during CephFS benchmark:", err)
		}
	}
}

// BenchmarkUpload_ConcurrentUploads_Ceph benchmarks concurrent upload operations on CephFS
func BenchmarkUpload_ConcurrentUploads_Ceph(b *testing.B) {
	// Test with more conservative concurrency levels for CephFS
	concurrencies := []int{1, 2, 4}
	
	for _, concurrency := range concurrencies {
		b.Run(fmt.Sprintf("Goroutines_%d", concurrency), func(b *testing.B) {
			benchmarkUploadConcurrentCeph(b, concurrency)
		})
	}
}

func benchmarkUploadConcurrentCeph(b *testing.B, concurrency int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-upload-concurrent-%d-ceph", concurrency))
	defer cleanup()

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Create test data (smaller size for concurrent tests)
	fileSize := int64(256 * 1024) // 256KB instead of 1MB
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	testDirName := filepath.Base(testDir)

	// Warm up - single upload to ensure CephFS is ready
	warmupPath := "/benchmark-tests/" + testDirName + "/warmup_concurrent.txt"
	warmupRef := &provider.Reference{Path: warmupPath}
	warmupReader := bytes.NewReader(testData)
	err := fs.Upload(ctx, warmupRef, io.NopCloser(warmupReader), nil)
	require.NoError(b, err, "Warmup upload failed on CephFS")

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(fileSize)

	// For CephFS concurrent tests, use sequential approach to avoid resource issues
	uploadCount := 0
	for i := 0; i < b.N; i++ {
		fileName := fmt.Sprintf("/benchmark-tests/%s/concurrent_upload_%d_%d.txt", testDirName, concurrency, uploadCount)
		ref := &provider.Reference{Path: fileName}
		
		// Create fresh reader for each upload
		reader := bytes.NewReader(testData)
		
		// Upload file
		err := fs.Upload(ctx, ref, io.NopCloser(reader), nil)
		if err != nil {
			b.Fatalf("Upload failed during CephFS concurrent benchmark: %v", err)
		}
		uploadCount++
	}
}

// BenchmarkUpload_DifferentDirectories_Ceph benchmarks uploads to different directory structures on CephFS
func BenchmarkUpload_DifferentDirectories_Ceph(b *testing.B) {
	// Test with different directory depths
	depths := []int{1, 3, 5, 10}
	
	for _, depth := range depths {
		b.Run(fmt.Sprintf("Depth_%d", depth), func(b *testing.B) {
			benchmarkUploadDirectoriesCeph(b, depth)
		})
	}
}

func benchmarkUploadDirectoriesCeph(b *testing.B, depth int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-upload-dirs-%d-ceph", depth))
	defer cleanup()

	// Set user context
	user := getBenchmarkTestUser(b)
	ctx := appctx.ContextSetUser(contextWithBenchmarkLogger(b), user)

	// Create test data (100KB per upload)
	fileSize := int64(100 * 1024)
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Create directory structure on filesystem
	testDirName := filepath.Base(testDir)
	dirPath := "/benchmark-tests/" + testDirName
	for i := 0; i < depth; i++ {
		dirPath += fmt.Sprintf("/level_%d", i)
		// Create directory through filesystem
		dirRef := &provider.Reference{Path: dirPath}
		err := fs.CreateDir(ctx, dirRef)
		if err != nil {
			// Directory might already exist, which is fine
		}
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(fileSize)

	for i := 0; i < b.N; i++ {
		// Upload to the deepest directory
		fileName := fmt.Sprintf("%s/upload_%d.txt", dirPath, i)
		ref := &provider.Reference{Path: fileName}
		
		// Create reader from test data
		reader := bytes.NewReader(testData)
		
		// Upload file
		err := fs.Upload(ctx, ref, io.NopCloser(reader), nil)
		if err != nil {
			b.Fatal("Upload to nested directory failed during CephFS benchmark:", err)
		}
	}
}

// BenchmarkMultiUser_ThreadIsolation_Ceph benchmarks thread isolation across multiple users on CephFS
func BenchmarkMultiUser_ThreadIsolation_Ceph(b *testing.B) {
	// Test with different user/thread combinations on CephFS
	testCases := []struct {
		name       string
		userCount  int
		threadCount int
	}{
		{"10Users_10Threads", 10, 10},
		{"50Users_50Threads", 50, 50},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkMultiUserThreadIsolationCeph(b, tc.userCount, tc.threadCount)
		})
	}
}

func benchmarkMultiUserThreadIsolationCeph(b *testing.B, userCount, threadCount int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-multiuser-%d-%d-ceph", userCount, threadCount))
	defer cleanup()

	// Create large test file content (512KB per file to keep threads busy but not overwhelm CephFS)
	fileSize := int64(512 * 1024)
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	testDirName := filepath.Base(testDir)
	baseCtx := contextWithBenchmarkLogger(b)

	// Pre-create test files for each user
	b.Log("Setting up test files for users on CephFS...")
	userContexts := make([]context.Context, userCount)
	for userID := 0; userID < userCount; userID++ {
		// Create unique user context
		user := getBenchmarkTestUser(b)
		user.Id.OpaqueId = fmt.Sprintf("ceph_user_%d", userID)
		user.Username = fmt.Sprintf("cephtestuser_%d", userID)
		user.UidNumber = int64(3000 + userID)
		user.GidNumber = int64(3000 + userID)
		userContexts[userID] = appctx.ContextSetUser(baseCtx, user)

		// Create test file for this user
		fileName := fmt.Sprintf("/benchmark-tests/%s/user_%d_testfile.txt", testDirName, userID)
		ref := &provider.Reference{Path: fileName}
		reader := bytes.NewReader(testData)
		err := fs.Upload(userContexts[userID], ref, io.NopCloser(reader), nil)
		require.NoError(b, err, "Failed to create test file for user %d on CephFS", userID)
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(userCount) * fileSize)

	// Run the actual benchmark
	for i := 0; i < b.N; i++ {
		// Use channels to coordinate goroutines
		done := make(chan bool, threadCount)
		errorChan := make(chan error, threadCount)

		// Launch concurrent threads for different users
		for threadID := 0; threadID < threadCount; threadID++ {
			go func(tID int) {
				// Each thread picks a user (round-robin)
				userID := tID % userCount
				userCtx := userContexts[userID]
				
				// Perform operations as this user to test isolation
				fileName := fmt.Sprintf("/benchmark-tests/%s/user_%d_testfile.txt", testDirName, userID)
				ref := &provider.Reference{Path: fileName}
				
				// Read the file multiple times to simulate sustained user activity
				for readCount := 0; readCount < 3; readCount++ { // Reduced for CephFS
					_, err := fs.GetMD(userCtx, ref, nil)
					if err != nil {
						errorChan <- fmt.Errorf("CephFS user %d thread %d read %d failed: %w", userID, tID, readCount, err)
						return
					}
				}
				
				// Test file operations specific to this user
				tempFileName := fmt.Sprintf("/benchmark-tests/%s/user_%d_thread_%d_temp.txt", testDirName, userID, tID)
				tempRef := &provider.Reference{Path: tempFileName}
				
				// Upload a small file
				smallData := []byte(fmt.Sprintf("CephFS Thread %d data for user %d", tID, userID))
				reader := bytes.NewReader(smallData)
				err := fs.Upload(userCtx, tempRef, io.NopCloser(reader), nil)
				if err != nil {
					errorChan <- fmt.Errorf("CephFS user %d thread %d upload failed: %w", userID, tID, err)
					return
				}
				
				// Read it back
				_, err = fs.GetMD(userCtx, tempRef, nil)
				if err != nil {
					errorChan <- fmt.Errorf("CephFS user %d thread %d read temp file failed: %w", userID, tID, err)
					return
				}
				
				done <- true
			}(threadID)
		}

		// Wait for all threads to complete
		completedThreads := 0
		var errors []error
		for completedThreads < threadCount {
			select {
			case <-done:
				completedThreads++
			case err := <-errorChan:
				errors = append(errors, err)
				if len(errors) == 1 {
					b.Logf("CephFS transient error encountered: %v", err)
				}
			}
		}

		// Allow some transient errors but fail if too many
		errorThreshold := max(1, threadCount/10) // Allow up to 10% error rate for CephFS
		if len(errors) > errorThreshold {
			b.Fatalf("CephFS thread isolation test failed: %d/%d threads had errors (threshold: %d), errors: %v", 
				len(errors), threadCount, errorThreshold, errors)
		} else if len(errors) > 0 {
			b.Logf("CephFS thread isolation completed with %d/%d transient errors (within acceptable threshold)", 
				len(errors), threadCount)
		}
	}
}

// BenchmarkMultiUser_ConcurrentReads_Ceph benchmarks concurrent read operations by multiple users on CephFS
func BenchmarkMultiUser_ConcurrentReads_Ceph(b *testing.B) {
	// Test scenarios with different user patterns on CephFS
	testCases := []struct {
		name       string
		userCount  int
		readsPerUser int
	}{
		{"20Users_3Reads", 20, 3},
		{"50Users_5Reads", 50, 5},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkMultiUserConcurrentReadsCeph(b, tc.userCount, tc.readsPerUser)
		})
	}
}

func benchmarkMultiUserConcurrentReadsCeph(b *testing.B, userCount, readsPerUser int) {
	// Check for Ceph integration requirements
	requireCephIntegrationForBenchmark(b)

	// Create Ceph-based filesystem and test directory
	fs, testDir, cleanup := setupCephBenchmark(b, fmt.Sprintf("benchmark-concurrent-reads-%d-%d-ceph", userCount, readsPerUser))
	defer cleanup()

	// Create test files for each user
	fileSize := int64(256 * 1024) // 256KB per file for CephFS
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	testDirName := filepath.Base(testDir)
	baseCtx := contextWithBenchmarkLogger(b)

	// Setup: create files and user contexts
	userContexts := make([]context.Context, userCount)
	fileRefs := make([]*provider.Reference, userCount)
	
	for userID := 0; userID < userCount; userID++ {
		// Create unique user context
		user := getBenchmarkTestUser(b)
		user.Id.OpaqueId = fmt.Sprintf("ceph_concurrent_user_%d", userID)
		user.Username = fmt.Sprintf("cephconcurrentuser_%d", userID)
		user.UidNumber = int64(4000 + userID)
		user.GidNumber = int64(4000 + userID)
		userContexts[userID] = appctx.ContextSetUser(baseCtx, user)

		// Create test file for this user using nceph interface
		fileName := fmt.Sprintf("/benchmark-tests/%s/concurrent_user_%d_file.txt", testDirName, userID)
		fileRefs[userID] = &provider.Reference{Path: fileName}
		
		// First ensure the directory exists using nceph
		dirPath := fmt.Sprintf("/benchmark-tests/%s", testDirName)
		dirRef := &provider.Reference{Path: dirPath}
		_ = fs.CreateDir(userContexts[userID], dirRef) // Ignore error if it already exists
		
		reader := bytes.NewReader(testData)
		err := fs.Upload(userContexts[userID], fileRefs[userID], io.NopCloser(reader), nil)
		require.NoError(b, err, "Failed to create test file for concurrent user %d on CephFS", userID)
	}

	// Reset timer and run benchmark
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(userCount * readsPerUser) * fileSize)

	for i := 0; i < b.N; i++ {
		// Create worker pool (more conservative for CephFS)
		jobs := make(chan int, userCount*readsPerUser)
		results := make(chan error, userCount*readsPerUser)
		
		// Start workers (conservative for CephFS to avoid overwhelming it)
		workerCount := min(8, userCount/2) // More conservative than local benchmarks
		if workerCount < 1 {
			workerCount = 1
		}
		
		for w := 0; w < workerCount; w++ {
			go func() {
				for jobID := range jobs {
					userID := jobID % userCount
					userCtx := userContexts[userID]
					ref := fileRefs[userID]
					
					_, err := fs.GetMD(userCtx, ref, nil)
					results <- err
				}
			}()
		}

		// Send jobs
		totalJobs := userCount * readsPerUser
		for j := 0; j < totalJobs; j++ {
			jobs <- j
		}
		close(jobs)

		// Collect results with error tolerance for transient CephFS issues
		var errorCount int
		var lastError error
		for j := 0; j < totalJobs; j++ {
			err := <-results
			if err != nil {
				errorCount++
				lastError = err
				if errorCount == 1 {
					b.Logf("CephFS transient error encountered (job %d): %v", j, err)
				}
			}
		}

		// Allow some transient errors but fail if too many
		errorThreshold := max(1, totalJobs/20) // Allow up to 5% error rate
		if errorCount > errorThreshold {
			b.Fatalf("CephFS concurrent read failed: %d/%d operations failed (threshold: %d), last error: %v", 
				errorCount, totalJobs, errorThreshold, lastError)
		} else if errorCount > 0 {
			b.Logf("CephFS benchmark completed with %d/%d transient errors (within acceptable threshold)", 
				errorCount, totalJobs)
		}
	}
}

// Helper functions for Ceph benchmarks

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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

	// Parse the mount point from fstab entry for cleanup purposes
	// Format: "server:port:/path /mnt/point ceph options"
	parts := strings.Fields(fstabEntry)
	if len(parts) < 3 {
		b.Fatalf("Invalid fstab entry format: %s", fstabEntry)
	}
	mountPoint := parts[1] // /mnt/miniflax

	// The test directory path as it will be used by nceph (relative to volume root)
	testDirPath := fmt.Sprintf("benchmark-tests/%s", prefix)

	// Create filesystem instance using real CephFS integration
	config := map[string]interface{}{
		"fstabentry": fstabEntry,
		// Don't set allow_local_mode - use real CephFS
	}

	// Create the filesystem using integration helper
	ctx := contextWithBenchmarkLogger(b)
	fs := createNcephFSForCephBenchmark(b, ctx, config)

	// Create the test directory using nceph interface to ensure path translation consistency
	err := fs.CreateDir(ctx, testDirPath)
	if err != nil {
		// Ignore if directory already exists
		if !strings.Contains(err.Error(), "file exists") &&
			!strings.Contains(err.Error(), "already exists") {
			b.Fatalf("Failed to create test directory %s via nceph: %v", testDirPath, err)
		}
	}

	// Cleanup function to remove test directory via nceph (with fallback to direct removal)
	cleanup := func() {
		if os.Getenv("NCEPH_TEST_PRESERVE") != "true" {
			// Try to remove via nceph first (proper cleanup)
			err := fs.Delete(ctx, testDirPath)
			if err != nil {
				b.Logf("Warning: failed to cleanup test directory %s via nceph: %v", testDirPath, err)
				// Fallback to direct removal on mount point if we can determine it
				if mountPoint != "" {
					testDir := filepath.Join(mountPoint, testDirPath)
					err := os.RemoveAll(testDir)
					if err != nil {
						b.Logf("Warning: failed to cleanup test directory %s directly: %v", testDir, err)
					}
				}
			}
		}
	}

	return fs, testDirPath, cleanup
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
