package nceph

import (
	"context"
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

	// Create a test file in the simulated mount point
	testFileName := "myfile.txt"
	testFilePath := filepath.Join(tempDir, testFileName)
	err := os.WriteFile(testFilePath, []byte("hello world"), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create test filesystem with your fstab entry concept
	config := map[string]interface{}{
		"allow_local_mode": true,
	}

	fs := CreateNcephFSForTesting(t, "/volumes/_nogroup/rasmus", tempDir, config)

	t.Run("GetMD_for_myfile_txt", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// User requests GetMD for /myfile.txt
		ref := &provider.Reference{
			Path: "/myfile.txt",
		}

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		require.NoError(t, err, "GetMD should succeed")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/myfile.txt", resourceInfo.Path, "ResourceInfo.Path should be the external path")
		assert.Equal(t, uint64(11), resourceInfo.Size, "File size should be 11 bytes")
		
		t.Logf("✅ Full GetMD operation test:")
		t.Logf("   User request: GetMD(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s/%s", tempDir, testFileName)
		t.Logf("   ResourceInfo.Path: %s", resourceInfo.Path)
		t.Logf("   File size: %d bytes", resourceInfo.Size)
	})

	t.Run("GetMD_for_nested_path", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// Create nested directory structure
		nestedDir := filepath.Join(tempDir, "documents", "project")
		err := os.MkdirAll(nestedDir, 0755)
		require.NoError(t, err, "Failed to create nested directory")
		
		nestedFile := filepath.Join(nestedDir, "report.pdf")
		err = os.WriteFile(nestedFile, []byte("PDF content here"), 0644)
		require.NoError(t, err, "Failed to create nested test file")

		// User requests GetMD for /documents/project/report.pdf
		ref := &provider.Reference{
			Path: "/documents/project/report.pdf",
		}

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		require.NoError(t, err, "GetMD should succeed for nested path")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/documents/project/report.pdf", resourceInfo.Path, "ResourceInfo.Path should be the external path")
		assert.Equal(t, uint64(16), resourceInfo.Size, "File size should be 16 bytes")
		
		t.Logf("✅ Nested path GetMD operation test:")
		t.Logf("   User request: GetMD(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", nestedFile)
		t.Logf("   ResourceInfo.Path: %s", resourceInfo.Path)
		t.Logf("   File size: %d bytes", resourceInfo.Size)
	})

	t.Run("GetMD_for_root_directory", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// User requests GetMD for / (root directory)
		ref := &provider.Reference{
			Path: "/",
		}

		// Call GetMD
		resourceInfo, err := fs.GetMD(ctx, ref, nil)
		require.NoError(t, err, "GetMD should succeed for root directory")
		require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

		// Verify the path in the result is the external path
		assert.Equal(t, "/", resourceInfo.Path, "ResourceInfo.Path should be root path")
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, resourceInfo.Type, "Root should be a container")
		
		t.Logf("✅ Root directory GetMD operation test:")
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

	// Create test directory structure:
	// /foo/
	//   ├── bar.txt
	//   ├── baz.pdf
	//   └── subdir/
	//       └── nested.doc
	fooDir := filepath.Join(tempDir, "foo")
	err := os.MkdirAll(fooDir, 0755)
	require.NoError(t, err, "Failed to create foo directory")

	// Create files in /foo/
	barFile := filepath.Join(fooDir, "bar.txt")
	err = os.WriteFile(barFile, []byte("bar content"), 0644)
	require.NoError(t, err, "Failed to create bar.txt")

	bazFile := filepath.Join(fooDir, "baz.pdf")
	err = os.WriteFile(bazFile, []byte("PDF content"), 0644)
	require.NoError(t, err, "Failed to create baz.pdf")

	// Create subdirectory with nested file
	subdirPath := filepath.Join(fooDir, "subdir")
	err = os.MkdirAll(subdirPath, 0755)
	require.NoError(t, err, "Failed to create subdir")

	nestedFile := filepath.Join(subdirPath, "nested.doc")
	err = os.WriteFile(nestedFile, []byte("nested document"), 0644)
	require.NoError(t, err, "Failed to create nested.doc")

	// Create test filesystem
	config := map[string]interface{}{
		"allow_local_mode": true,
	}

	fs := CreateNcephFSForTesting(t, "/volumes/_nogroup/rasmus", tempDir, config)

	t.Run("ListFolder_for_foo_directory", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// User requests ListFolder for /foo
		ref := &provider.Reference{
			Path: "/foo",
		}

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
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
				t.Logf("✅ Found entry: %s (type: %s)", entry.Path, entry.Type)
			}
		}

		t.Logf("✅ Full ListFolder operation test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", fooDir)
		t.Logf("   Found %d entries with correct external paths", len(entries))
	})

	t.Run("ListFolder_for_nested_subdir", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// User requests ListFolder for /foo/subdir
		ref := &provider.Reference{
			Path: "/foo/subdir",
		}

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
		require.NoError(t, err, "ListFolder should succeed for nested directory")
		require.Len(t, entries, 1, "Should find 1 entry in subdir")

		// Verify the nested file
		entry := entries[0]
		assert.Equal(t, "/foo/subdir/nested.doc", entry.Path, "Nested file should have full external path")
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_FILE, entry.Type, "Should be a file")
		assert.Equal(t, uint64(15), entry.Size, "File size should be 15 bytes")

		t.Logf("✅ Nested directory ListFolder test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", subdirPath)
		t.Logf("   Found entry: %s", entry.Path)
	})

	t.Run("ListFolder_for_root_directory", func(t *testing.T) {
		ctx := context.Background()
		
		// Add a user context to avoid permission issues
		user := &userv1beta1.User{
			Id: &userv1beta1.UserId{
				OpaqueId: "testuser",
				Idp:      "local",
			},
			Username:  "testuser",
			UidNumber: int64(os.Getuid()), // Use current user's UID to avoid permission issues
			GidNumber: int64(os.Getgid()), // Use current user's GID to avoid permission issues
		}
		ctx = appctx.ContextSetUser(ctx, user)
		
		// User requests ListFolder for / (root directory)
		ref := &provider.Reference{
			Path: "/",
		}

		// Call ListFolder
		entries, err := fs.ListFolder(ctx, ref, []string{})
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

		t.Logf("✅ Root directory ListFolder test:")
		t.Logf("   User request: ListFolder(%s)", ref.Path)
		t.Logf("   Filesystem accesses: %s", tempDir)
		t.Logf("   Found /foo entry with correct external path")
		t.Logf("   Total entries: %d", len(entries))
	})
}
