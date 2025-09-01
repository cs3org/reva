package nceph

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestPathConversionWithSpecificFstab(t *testing.T) {
	// Create temporary directory to simulate /mnt/miniflax
	tempDir, cleanup := GetTestDir(t, "miniflax-simulation")
	defer cleanup()
	
	// Test with your specific fstab entry concept
	fstabEntry := "cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus /mnt/miniflax ceph name=mds-admin,secretfile=/etc/ceph/miniflax.mds-admin.secret,x-systemd.device-timeout=30,x-systemd.mount-timeout=30,noatime,_netdev,wsync 0 2"
	
	// Create test filesystem with local mode (using tempDir instead of /mnt/miniflax)
	config := map[string]interface{}{
		"fstabentry":     fstabEntry,
		"allow_local_mode": true,
	}
	
	fs := CreateNcephFSForTesting(t, ContextWithTestLogger(t), config, "/volumes/_nogroup/rasmus", tempDir)
	
	// Test the path conversion for your specific example
	t.Run("user_request_myfile", func(t *testing.T) {
		// User requests: /myfile.txt
		externalPath := "/myfile.txt"
		
		// Convert to chroot-relative path
		chrootPath := fs.toChroot(externalPath)
		assert.Equal(t, "myfile.txt", chrootPath, "toChroot should remove leading slash")
		
		// This chrootPath would be used with rootFS operations
		// rootFS.Stat("myfile.txt") would access {tempDir}/myfile.txt (simulating /mnt/miniflax/myfile.txt)
		
		// Convert back to external path (what user sees in response)
		resultPath := fs.fromChroot(chrootPath)
		assert.Equal(t, "/myfile.txt", resultPath, "fromChroot should restore external path")
		
		t.Logf("✅ Path conversion chain:")
		t.Logf("   User request: %s", externalPath)
		t.Logf("   Chroot path: %s (used with rootFS)", chrootPath)
		t.Logf("   Actual filesystem: %s/%s (simulating /mnt/miniflax/%s)", tempDir, chrootPath, chrootPath)
		t.Logf("   Result path: %s (returned to user)", resultPath)
	})
	
	// Test edge cases
	t.Run("root_directory", func(t *testing.T) {
		externalPath := "/"
		chrootPath := fs.toChroot(externalPath)
		assert.Equal(t, ".", chrootPath, "Root directory should become '.'")
		
		resultPath := fs.fromChroot(chrootPath)
		assert.Equal(t, "/", resultPath, "Should convert back to root")
	})
	
	t.Run("nested_path", func(t *testing.T) {
		externalPath := "/documents/project/file.pdf"
		chrootPath := fs.toChroot(externalPath)
		assert.Equal(t, "documents/project/file.pdf", chrootPath, "Should remove leading slash")
		
		resultPath := fs.fromChroot(chrootPath)
		assert.Equal(t, "/documents/project/file.pdf", resultPath, "Should restore full path")
		
		t.Logf("   Nested path test:")
		t.Logf("   User request: %s", externalPath)
		t.Logf("   Chroot path: %s", chrootPath)
		t.Logf("   Actual filesystem: %s/%s (simulating /mnt/miniflax/%s)", tempDir, chrootPath, chrootPath)
		t.Logf("   Result path: %s", resultPath)
	})
}
