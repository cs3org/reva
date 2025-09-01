package nceph

import (
	"context"
	"os"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// TestIntegrationWithRealCeph demonstrates integration testing against a real Ceph mount
func TestIntegrationWithRealCeph(t *testing.T) {
	// Skip if NCEPH_FSTAB_ENTRY is not set or looks like a dummy entry
	fstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY")
	if fstabEntry == "" {
		t.Skip("Skipping integration test: NCEPH_FSTAB_ENTRY not set")
	}
	if fstabEntry == "dummy@cluster:/ /tmp/test ceph defaults" {
		t.Skip("Skipping integration test: NCEPH_FSTAB_ENTRY appears to be a dummy entry for unit tests")
	}

	// Enable info-level logging to see the path details
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(zerolog.InfoLevel)
	ctx := logger.WithContext(context.Background())

	t.Logf("🔍 Integration test using NCEPH_FSTAB_ENTRY: %s", fstabEntry)

	// Create filesystem for integration testing (no overrides)
	config := map[string]interface{}{
		// No allow_local_mode - this should use the real Ceph mount
	}
	fs := CreateNcephFSForIntegration(t, config)

	// Set user context
	user := &userv1beta1.User{
		Id: &userv1beta1.UserId{
			OpaqueId: "integrationuser",
			Idp:      "local",
		},
		Username:  "integrationuser",
		UidNumber: int64(os.Getuid()),
		GidNumber: int64(os.Getgid()),
	}
	ctx = appctx.ContextSetUser(ctx, user)

	t.Log("🔍 Testing GetMD against real Ceph mount:")

	// Test GetMD on root directory (should work with any mount)
	ref := &provider.Reference{Path: "/"}
	resourceInfo, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err, "GetMD should succeed on real mount")
	require.NotNil(t, resourceInfo, "ResourceInfo should not be nil")

	t.Logf("✅ Integration GetMD successful:")
	t.Logf("   - Requested path: %s", ref.Path)
	t.Logf("   - Result path: %s", resourceInfo.Path)
	t.Logf("   - Resource type: %s", resourceInfo.Type)

	t.Log("🔍 Testing ListFolder against real Ceph mount:")

	// Test ListFolder on root directory
	entries, err := fs.ListFolder(ctx, ref, nil)
	require.NoError(t, err, "ListFolder should succeed on real mount")

	t.Logf("✅ Integration ListFolder successful:")
	t.Logf("   - Requested path: %s", ref.Path)
	t.Logf("   - Found %d entries", len(entries))
	for i, entry := range entries {
		if i < 5 { // Show first 5 entries
			t.Logf("   - %s (%s)", entry.Path, entry.Type)
		}
	}
	if len(entries) > 5 {
		t.Logf("   - ... and %d more entries", len(entries)-5)
	}
}
