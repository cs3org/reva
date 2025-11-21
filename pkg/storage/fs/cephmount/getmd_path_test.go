package cephmount

import (
	"os"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealPathConversionWithGetMD(t *testing.T) {
	// Create temporary directory to simulate /mnt/miniflax
	tempDir, cleanup := GetTestDir(t, "real-getmd-test")
	defer cleanup()

	// Log current process info
	currentUID := os.Getuid()
	currentGID := os.Getgid()
	t.Logf("Process info: running as UID=%d, GID=%d", currentUID, currentGID)
	t.Logf("Test directory: %s", tempDir)

	// Create a test file in the simulated mount point
	testFileName := "myfile.txt"
	testFilePath := filepath.Join(tempDir, testFileName)
	err := os.WriteFile(testFilePath, []byte("hello world"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Make the test file readable by everyone (in case we're running as root but switching to another user)
	err = os.Chmod(testFilePath, 0666)
	require.NoError(t, err, "Failed to set permissions on test file")

	// Make the test directory accessible by everyone
	err = os.Chmod(tempDir, 0777)
	require.NoError(t, err, "Failed to set permissions on test directory")

	// Log file permissions
	if info, err := os.Stat(testFilePath); err == nil {
		t.Logf("Test file permissions: %s (size: %d bytes)", info.Mode(), info.Size())
	}

	// Create test filesystem with your fstab entry concept
	config := map[string]any{
		"testing_allow_local_mode": true,
	}

	fs := CreateCephMountFSForTesting(t, ContextWithTestLogger(t), config, "/volumes/_nogroup/rasmus", tempDir)

	t.Run("GetMD_for_myfile_txt", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Add a user context to avoid permission issues
		// Use root (0) as the test user since we're running as root and created files as root
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("Setting user context: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// User requests GetMD for /myfile.txt
		ref := &provider.Reference{
			Path: "/myfile.txt",
		}

		t.Logf("About to call GetMD with:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", filepath.Join(tempDir, "myfile.txt"))
		t.Logf("   - User context: UID=%d, GID=%d", testUID, testGID)

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			t.Logf("GetMD failed with error: %v", err)
			// Let's check if the file exists and what its permissions are
			if info, statErr := os.Stat(filepath.Join(tempDir, "myfile.txt")); statErr == nil {
				t.Logf("File exists with mode %s, size %d", info.Mode(), info.Size())
			} else {
				t.Logf("File stat failed: %v", statErr)
			}
		}
		require.NoError(t, err, "GetMD should succeed")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/myfile.txt", resourceInfo.Path, "ResourceInfo.Path should be the external path")
		assert.Equal(t, uint64(11), resourceInfo.Size, "File size should be 11 bytes")

		t.Logf("Full GetMD operation test:")
		t.Logf("   User request: GetMD(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s/%s", tempDir, testFileName)
		t.Logf("   ResourceInfo.Path: %s", resourceInfo.Path)
		t.Logf("   File size: %d bytes", resourceInfo.Size)
	})

	t.Run("GetMD_for_nested_path", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Use the same user context as the first test
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("Setting user context for nested test: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// Create nested directory structure
		nestedDir := filepath.Join(tempDir, "documents", "project")
		err := os.MkdirAll(nestedDir, 0755)
		require.NoError(t, err, "Failed to create nested directory")

		// Make the nested directories accessible
		err = os.Chmod(filepath.Join(tempDir, "documents"), 0777)
		require.NoError(t, err, "Failed to set permissions on documents directory")
		err = os.Chmod(nestedDir, 0777)
		require.NoError(t, err, "Failed to set permissions on project directory")

		nestedFile := filepath.Join(nestedDir, "report.pdf")
		err = os.WriteFile(nestedFile, []byte("PDF content here"), 0644)
		require.NoError(t, err, "Failed to create nested test file")

		// Make the nested file readable
		err = os.Chmod(nestedFile, 0666)
		require.NoError(t, err, "Failed to set permissions on nested file")

		t.Logf("Created nested structure:")
		t.Logf("   - %s (mode: %v)", filepath.Join(tempDir, "documents"), "0777")
		t.Logf("   - %s (mode: %v)", nestedDir, "0777")
		t.Logf("   - %s (mode: %v)", nestedFile, "0666")

		// User requests GetMD for /documents/project/report.pdf
		ref := &provider.Reference{
			Path: "/documents/project/report.pdf",
		}

		t.Logf("About to call GetMD for nested path:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", nestedFile)

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			t.Logf("GetMD failed for nested path: %v", err)
		}
		require.NoError(t, err, "GetMD should succeed for nested path")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/documents/project/report.pdf", resourceInfo.Path, "ResourceInfo.Path should be the external path")
		assert.Equal(t, uint64(16), resourceInfo.Size, "File size should be 16 bytes")

		t.Logf("Nested path GetMD operation test:")
		t.Logf("   User request: GetMD(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", nestedFile)
		t.Logf("   ResourceInfo.Path: %s", resourceInfo.Path)
		t.Logf("   File size: %d bytes", resourceInfo.Size)
	})

	t.Run("GetMD_for_root_directory", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Use the same user context
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("Setting user context for root directory test: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// User requests GetMD for / (root directory)
		ref := &provider.Reference{
			Path: "/",
		}

		t.Logf("About to call GetMD for root directory:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", tempDir)

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		if err != nil {
			t.Logf("GetMD failed for root directory: %v", err)
		}
		require.NoError(t, err, "GetMD should succeed for root directory")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/", resourceInfo.Path, "ResourceInfo.Path should be root path")
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, resourceInfo.Type, "Root should be a container")

		t.Logf("Root directory GetMD operation test:")
		t.Logf("   User request: GetMD(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", tempDir)
		t.Logf("   ResourceInfo.Path: %s", resourceInfo.Path)
		t.Logf("   ResourceInfo.Type: %s", resourceInfo.Type)
	})
}

func TestRealPathConversionWithListFolder(t *testing.T) {
	// Create temporary directory to simulate /mnt/miniflax
	tempDir, cleanup := GetTestDir(t, "real-listfolder-test")
	defer cleanup()

	// Log current process info
	currentUID := os.Getuid()
	currentGID := os.Getgid()
	t.Logf("ListFolder test - Process info: running as UID=%d, GID=%d", currentUID, currentGID)
	t.Logf("Test directory: %s", tempDir)

	// Make the test directory accessible by everyone
	err := os.Chmod(tempDir, 0777)
	require.NoError(t, err, "Failed to set permissions on test directory")

	// Create test directory structure:
	// /foo/
	//   ├── bar.txt
	//   ├── baz.pdf
	//   └── subdir/
	//       └── nested.doc
	fooDir := filepath.Join(tempDir, "foo")
	err = os.MkdirAll(fooDir, 0755)
	require.NoError(t, err, "Failed to create foo directory")

	// Make the foo directory accessible
	err = os.Chmod(fooDir, 0777)
	require.NoError(t, err, "Failed to set permissions on foo directory")

	// Create files in /foo/
	barFile := filepath.Join(fooDir, "bar.txt")
	err = os.WriteFile(barFile, []byte("bar content"), 0644)
	require.NoError(t, err, "Failed to create bar.txt")

	// Set permissions on bar file
	err = os.Chmod(barFile, 0666)
	require.NoError(t, err, "Failed to set permissions on bar.txt")

	bazFile := filepath.Join(fooDir, "baz.pdf")
	err = os.WriteFile(bazFile, []byte("PDF content"), 0644)
	require.NoError(t, err, "Failed to create baz.pdf")

	// Set permissions on baz file
	err = os.Chmod(bazFile, 0666)
	require.NoError(t, err, "Failed to set permissions on baz.pdf")

	// Create subdirectory with nested file
	subdirPath := filepath.Join(fooDir, "subdir")
	err = os.MkdirAll(subdirPath, 0755)
	require.NoError(t, err, "Failed to create subdir")

	// Set permissions on subdir
	err = os.Chmod(subdirPath, 0777)
	require.NoError(t, err, "Failed to set permissions on subdir")

	nestedFile := filepath.Join(subdirPath, "nested.doc")
	err = os.WriteFile(nestedFile, []byte("nested document"), 0644)
	require.NoError(t, err, "Failed to create nested.doc")

	// Set permissions on nested file
	err = os.Chmod(nestedFile, 0666)
	require.NoError(t, err, "Failed to set permissions on nested.doc")

	t.Logf("Created test structure with permissions:")
	t.Logf("   - %s (mode: 0777)", tempDir)
	t.Logf("   - %s (mode: 0777)", fooDir)
	t.Logf("   - %s (mode: 0666)", barFile)
	t.Logf("   - %s (mode: 0666)", bazFile)
	t.Logf("   - %s (mode: 0777)", subdirPath)
	t.Logf("   - %s (mode: 0666)", nestedFile)

	// Create test filesystem
	config := map[string]any{
		"testing_allow_local_mode": true,
	}

	fs := CreateCephMountFSForTesting(t, ContextWithTestLogger(t), config, "/volumes/_nogroup/rasmus", tempDir)

	t.Run("ListFolder_for_foo_directory", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Use consistent user context
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("ListFolder test - Setting user context: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// User requests ListFolder for /foo
		ref := &provider.Reference{
			Path: "/foo",
		}

		t.Logf("About to call ListFolder:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", fooDir)

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
		if err != nil {
			t.Logf("ListFolder failed: %v", err)
		}
		require.NoError(t, err, "ListFolder should succeed")
		require.Len(t, entries, 3, "Should find 3 entries (2 files + 1 directory)")

		// Create a map of found paths for easier verification
		foundPaths := make(map[string]*provider.ResourceInfo)
		for _, entry := range entries {
			foundPaths[entry.Path] = entry
		}

		// Verify each expected entry
		expectedEntries := []struct {
			path         string
			expectedType provider.ResourceType
			description  string
		}{
			{"/foo/bar.txt", provider.ResourceType_RESOURCE_TYPE_FILE, "bar.txt file"},
			{"/foo/baz.pdf", provider.ResourceType_RESOURCE_TYPE_FILE, "baz.pdf file"},
			{"/foo/subdir", provider.ResourceType_RESOURCE_TYPE_CONTAINER, "subdir directory"},
		}

		for _, expected := range expectedEntries {
			entry, found := foundPaths[expected.path]
			assert.True(t, found, "Should find entry for %s", expected.path)
			if found {
				assert.Equal(t, expected.expectedType, entry.Type, "%s should have correct type", expected.description)
				t.Logf("Found entry: %s (type: %s)", entry.Path, entry.Type)
			}
		}

		t.Logf("Full ListFolder operation test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", fooDir)
		t.Logf("   Found %d entries with correct external paths", len(entries))
	})

	t.Run("ListFolder_for_nested_subdir", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Use consistent user context
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("ListFolder nested test - Setting user context: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// User requests ListFolder for /foo/subdir
		ref := &provider.Reference{
			Path: "/foo/subdir",
		}

		t.Logf("About to call ListFolder for nested subdir:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", subdirPath)

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
		if err != nil {
			t.Logf("ListFolder failed for nested subdir: %v", err)
		}
		require.NoError(t, err, "ListFolder should succeed for nested directory")
		require.Len(t, entries, 1, "Should find 1 entry in subdir")

		// Verify the nested file
		entry := entries[0]
		assert.Equal(t, "/foo/subdir/nested.doc", entry.Path, "Nested file should have full external path")
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, entry.Type, "Should be a file")
		assert.Equal(t, uint64(15), entry.Size, "File size should be 15 bytes")

		t.Logf("Nested directory ListFolder test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", subdirPath)
		t.Logf("   Found entry: %s", entry.Path)
	})

	t.Run("ListFolder_for_root_directory", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Use consistent user context
		testUID := int64(currentUID)
		testGID := int64(currentGID)
		t.Logf("ListFolder root test - Setting user context: UID=%d, GID=%d", testUID, testGID)

		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: testUID,
			GidNumber: testGID,
		}
		ctx = appctx.ContextSetUser(ctx, user)

		// User requests ListFolder for / (root directory)
		ref := &provider.Reference{
			Path: "/",
		}

		t.Logf("About to call ListFolder for root directory:")
		t.Logf("   - Reference path: %s", ref.Path)
		t.Logf("   - Expected filesystem path: %s", tempDir)

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
		if err != nil {
			t.Logf("ListFolder failed for root directory: %v", err)
		}
		require.NoError(t, err, "ListFolder should succeed for root directory")
		require.Greater(t, len(entries), 0, "Root directory should have at least one entry")

		// Find the foo directory in the results
		var fooEntry *provider.ResourceInfo
		for _, entry := range entries {
			if entry.Path == "/foo" {
				fooEntry = entry
				break
			}
		}

		require.NotNil(t, fooEntry, "Should find /foo directory in root listing")
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, fooEntry.Type, "/foo should be a directory")

		t.Logf("Root directory ListFolder test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", tempDir)
		t.Logf("   Found /foo entry with correct external path")
		t.Logf("   Total entries: %d", len(entries))
	})
}
