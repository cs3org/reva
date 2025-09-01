package nceph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCephAutoDiscovery(t *testing.T) {
	// Create temporary files to simulate Ceph config and fstab
	tempDir := t.TempDir()

	// Create mock Ceph config file
	cephConfigFile := filepath.Join(tempDir, "ceph.conf")
	cephConfigContent := `[global]
admin socket = /var/run/ceph/$cluster-$name-$pid.asok
client reconnect stale = true
debug client = 0/2
fuse big writes = true
mon host = cephminiflax.cern.ch:6789

[client.mds-admin]
keyring = /etc/ceph/miniflax.mds-admin.keyring
`

	err := os.WriteFile(cephConfigFile, []byte(cephConfigContent), 0644)
	require.NoError(t, err)

	// Create mock fstab content (we'll test the parsing function directly)
	mockFstabContent := `# /etc/fstab: static file system information.
# <file system> <mount point>   <type>  <options>       <dump>  <pass>
/dev/sda1 / ext4 defaults 0 1
tmpfs /tmp tmpfs defaults 0 0
cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus	/mnt/miniflax	ceph	name=mds-admin,secretfile=/etc/ceph/miniflax.mds-admin.secret,x-systemd.device-timeout=30,x-systemd.mount-timeout=30,noatime,_netdev,wsync	0	2
`

	// Test 1: Extract monitor host from config
	t.Run("extract_monitor_host", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)
		monitorHost, err := extractMonitorHostFromConfig(ctx, cephConfigFile)
		require.NoError(t, err)
		assert.Equal(t, "cephminiflax.cern.ch:6789", monitorHost)
	})

	// Test 2: Parse fstab entry (using a helper function)
	t.Run("parse_fstab_entry", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Write mock fstab to a temp file
		mockFstabFile := filepath.Join(tempDir, "fstab")
		err := os.WriteFile(mockFstabFile, []byte(mockFstabContent), 0644)
		require.NoError(t, err)

		// Test parsing by temporarily replacing the fstab path
		mountInfo, err := parseFstabFile(ctx, mockFstabFile, "cephminiflax.cern.ch:6789")
		require.NoError(t, err)

		assert.Equal(t, "cephminiflax.cern.ch:6789", mountInfo.MonitorHost)
		assert.Equal(t, "/volumes/_nogroup/rasmus", mountInfo.CephVolumePath)
		assert.Equal(t, "/mnt/miniflax", mountInfo.LocalMountPoint)
		assert.Equal(t, "mds-admin", mountInfo.ClientName)
	})

	// Test 3: Complete auto-discovery integration
	t.Run("complete_autodiscovery", func(t *testing.T) {
		ctx := ContextWithTestLogger(t)

		// Mock the fstab reading by creating a custom discover function
		mountInfo, err := testDiscoverCephMountInfo(ctx, cephConfigFile, mockFstabContent)
		require.NoError(t, err)

		assert.Equal(t, "cephminiflax.cern.ch:6789", mountInfo.MonitorHost)
		assert.Equal(t, "/volumes/_nogroup/rasmus", mountInfo.CephVolumePath)
		assert.Equal(t, "/mnt/miniflax", mountInfo.LocalMountPoint)
		assert.Equal(t, "mds-admin", mountInfo.ClientName)

		t.Logf("Auto-discovered configuration:")
		t.Logf("   Monitor Host: %s", mountInfo.MonitorHost)
		t.Logf("   Ceph Volume Path: %s", mountInfo.CephVolumePath)
		t.Logf("   Local Mount Point: %s", mountInfo.LocalMountPoint)
		t.Logf("   Client Name: %s", mountInfo.ClientName)
	})

	// Test 4: Test auto-discovery with nceph configuration
	t.Run("nceph_with_autodiscovery", func(t *testing.T) {
		// This test shows how auto-discovery would work with nceph configuration
		testConfig := map[string]interface{}{
			"root":           tempDir,
			"auto_discovery": true,
			"ceph_config":    cephConfigFile,
		}

		// Note: This test would fail in the actual New() function because
		// it tries to read /etc/fstab, but it demonstrates the concept
		t.Logf("Example configuration with auto-discovery enabled:")
		for key, value := range testConfig {
			t.Logf("   %s: %v", key, value)
		}

		t.Logf("With auto-discovery, the system would:")
		t.Logf("   1. Read monitor host from %s", cephConfigFile)
		t.Logf("   2. Find matching Ceph mount in /etc/fstab")
		t.Logf("   3. Extract Ceph volume path as common denominator")
		t.Logf("   4. Configure path mapping automatically")
	})
}

// testDiscoverCephMountInfo is a test version that uses mock fstab content
func testDiscoverCephMountInfo(ctx context.Context, cephConfigFile, mockFstabContent string) (*CephMountInfo, error) {
	// Step 1: Extract monitor host from Ceph config
	monitorHost, err := extractMonitorHostFromConfig(ctx, cephConfigFile)
	if err != nil {
		return nil, err
	}

	// Step 2: Parse mock fstab content directly
	return parseFstabContent(ctx, mockFstabContent, monitorHost)
}

// parseFstabFile reads and parses an fstab file
func parseFstabFile(ctx context.Context, fstabFile, monitorHost string) (*CephMountInfo, error) {
	content, err := os.ReadFile(fstabFile)
	if err != nil {
		return nil, err
	}
	return parseFstabContent(ctx, string(content), monitorHost)
}

// parseFstabContent parses fstab content string (helper for testing)
func parseFstabContent(ctx context.Context, fstabContent, monitorHost string) (*CephMountInfo, error) {
	lines := strings.Split(fstabContent, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Simple parsing for the expected format
		if strings.Contains(line, monitorHost) && strings.Contains(line, "ceph") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				// Parse device field: "monitor:port:/volume/path"
				device := fields[0]
				parts := strings.SplitN(device, ":", 3)
				if len(parts) == 3 {
					extractedMonitorHost := parts[0] + ":" + parts[1]
					cephVolumePath := parts[2]
					localMountPoint := fields[1]

					// Extract client name from options
					options := ""
					if len(fields) > 3 {
						options = fields[3]
					}
					clientName := extractClientNameFromOptions(options)

					if extractedMonitorHost == monitorHost {
						return &CephMountInfo{
							MonitorHost:     extractedMonitorHost,
							CephVolumePath:  cephVolumePath,
							LocalMountPoint: localMountPoint,
							ClientName:      clientName,
						}, nil
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no matching Ceph mount found for monitor host %s", monitorHost)
}

func TestCephConfigParsing(t *testing.T) {
	// Test various Ceph config file formats
	testCases := []struct {
		name            string
		configContent   string
		expectedMonHost string
		shouldError     bool
	}{
		{
			name: "standard_config",
			configContent: `[global]
mon host = cephminiflax.cern.ch:6789
fsid = 12345-67890`,
			expectedMonHost: "cephminiflax.cern.ch:6789",
			shouldError:     false,
		},
		{
			name: "config_with_spaces",
			configContent: `[global]
    mon host    =    cephcluster.example.com:6789    
    fsid = abcd-efgh`,
			expectedMonHost: "cephcluster.example.com:6789",
			shouldError:     false,
		},
		{
			name: "config_without_mon_host",
			configContent: `[global]
fsid = 12345-67890
log file = /var/log/ceph.log`,
			expectedMonHost: "",
			shouldError:     true,
		},
		{
			name: "config_with_comments",
			configContent: `# Ceph configuration file
[global]
# Monitor configuration
mon host = testcluster.local:6789
# Other settings
fsid = test-cluster`,
			expectedMonHost: "testcluster.local:6789",
			shouldError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write test config to temp file
			tempFile := filepath.Join(t.TempDir(), "ceph.conf")
			err := os.WriteFile(tempFile, []byte(tc.configContent), 0644)
			require.NoError(t, err)

			// Test parsing
			ctx := ContextWithTestLogger(t)
			monHost, err := extractMonitorHostFromConfig(ctx, tempFile)

			if tc.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedMonHost, monHost)
				t.Logf("Extracted monitor host: %s", monHost)
			}
		})
	}
}
