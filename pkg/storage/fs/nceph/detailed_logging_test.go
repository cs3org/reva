package nceph

import (
	"os"
	"path/filepath"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/require"
)

// TestDetailedPathLogging shows the enhanced path logging in action
func TestDetailedPathLogging(t *testing.T) {
	// Use our standard test logger
	ctx := ContextWithTestLogger(t)
	
	// Create temporary directory
	tempDir, cleanup := GetTestDir(t, "detailed-logging-test")
	defer cleanup()

	// Create test file
	testFile := filepath.Join(tempDir, "example.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0666)
	require.NoError(t, err)

	// Create filesystem with detailed configuration
	config := map[string]interface{}{
		"allow_local_mode": true,
	}
	fs := CreateNcephFSForTesting(t, ctx, config, "/volumes/_nogroup/testuser", tempDir)

	// Set user context
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "testuser",
			Idp:      "local",
		},
		Username:  "testuser",
		UidNumber: 1000,
		GidNumber: 1000,
	}
	ctx = appctx.ContextSetUser(ctx, user)

	t.Log("🔍 Testing GetMD operation with detailed path logging:")

	// Test GetMD operation
	ref := &provider.Reference{Path: "/example.txt"}
	resourceInfo, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	
	t.Logf("✅ GetMD completed: received path '/example.txt' -> result path '%s'", resourceInfo.Path)

	t.Log("🔍 Testing ListFolder operation with detailed path logging:")

	// Test ListFolder operation
	dirRef := &provider.Reference{Path: "/"}
	entries, err := fs.ListFolder(ctx, dirRef, nil)
	require.NoError(t, err)
	
	t.Logf("✅ ListFolder completed: received path '/' -> found %d entries", len(entries))
	for _, entry := range entries {
		t.Logf("   - %s", entry.Path)
	}
}
