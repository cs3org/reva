package nceph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFstabParserWithRealExample(t *testing.T) {
	ctx := ContextWithTestLogger(t)
	
	// Test with the real fstab entry you provided
	fstabEntry := "cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus /mnt/miniflax ceph defaults,name=mds-admin,secretfile=/etc/ceph/ceph.client.mds-admin.key,conf=/etc/ceph/ceph.conf,_netdev 0 2"
	
	mountInfo, err := ParseFstabEntry(ctx, fstabEntry)
	require.NoError(t, err, "Should parse the real fstab entry successfully")
	
	// Verify all fields are extracted correctly
	assert.Equal(t, "cephminiflax.cern.ch:6789", mountInfo.MonitorHost, "Monitor host should be extracted correctly")
	assert.Equal(t, "/volumes/_nogroup/rasmus", mountInfo.CephVolumePath, "Ceph volume path should be extracted correctly")
	assert.Equal(t, "/mnt/miniflax", mountInfo.LocalMountPoint, "Local mount point should be extracted correctly")
	assert.Equal(t, "mds-admin", mountInfo.ClientName, "Client name should be extracted correctly")
	assert.Equal(t, "/etc/ceph/ceph.client.mds-admin.key", mountInfo.SecretFile, "Secret file should be extracted correctly")
	assert.Equal(t, "/etc/ceph/ceph.conf", mountInfo.ConfigFile, "Config file should be extracted correctly")
	assert.Equal(t, "/etc/ceph/ceph.client.mds-admin.keyring", mountInfo.KeyringFile, "Keyring file should be constructed correctly")
	
	t.Logf("✅ Successfully parsed real fstab entry:")
	t.Logf("   Monitor Host: %s", mountInfo.MonitorHost)
	t.Logf("   Ceph Volume Path: %s", mountInfo.CephVolumePath)
	t.Logf("   Local Mount Point: %s", mountInfo.LocalMountPoint)
	t.Logf("   Client Name: %s", mountInfo.ClientName)
	t.Logf("   Secret File: %s", mountInfo.SecretFile)
	t.Logf("   Config File: %s", mountInfo.ConfigFile)
	t.Logf("   Keyring File: %s", mountInfo.KeyringFile)
}

func TestFstabParserMinimalValidation(t *testing.T) {
	ctx := ContextWithTestLogger(t)
	
	tests := []struct {
		name        string
		fstabEntry  string
		expectError bool
		description string
	}{
		{
			name:        "real_cern_example",
			fstabEntry:  "cephminiflax.cern.ch:6789:/volumes/_nogroup/rasmus /mnt/miniflax ceph defaults,name=mds-admin,secretfile=/etc/ceph/ceph.client.mds-admin.key,conf=/etc/ceph/ceph.conf,_netdev 0 2",
			expectError: false,
			description: "Real CERN fstab example should parse successfully",
		},
		{
			name:        "minimal_valid",
			fstabEntry:  "mon.example.com:6789:/volumes/test /mnt/ceph ceph name=admin,secretfile=/etc/ceph/secret 0 0",
			expectError: false,
			description: "Minimal valid fstab entry should parse",
		},
		{
			name:        "invalid_filesystem",
			fstabEntry:  "server:/path /mnt/nfs nfs defaults 0 0",
			expectError: true,
			description: "Non-ceph filesystem should be rejected",
		},
		{
			name:        "incomplete_entry",
			fstabEntry:  "incomplete entry",
			expectError: true,
			description: "Incomplete fstab entry should be rejected",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFstabEntry(ctx, tt.fstabEntry)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}
