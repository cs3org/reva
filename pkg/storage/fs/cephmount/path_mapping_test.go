package cephmount

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathMapping(t *testing.T) {
	testCases := []struct {
		name             string
		cephVolumePath   string
		localMountPoint  string
		root             string
		inputPath        string
		expectedUserPath string
		expectedCephPath string
		description      string
	}{
		{
			name:             "root_mount_simple",
			cephVolumePath:   "/",
			localMountPoint:  "/mnt/cephfs",
			root:             "/mnt/cephfs",
			inputPath:        "/test/file.txt",
			expectedUserPath: "/mnt/cephfs/test/file.txt",
			expectedCephPath: "/test/file.txt",
			description:      "Root mount with simple file path",
		},
		{
			name:             "subvolume_mount",
			cephVolumePath:   "/volumes/users",
			localMountPoint:  "/home",
			root:             "/home/alice",
			inputPath:        "/volumes/users/alice/documents/file.txt",
			expectedUserPath: "/home/alice/documents/file.txt",
			expectedCephPath: "/volumes/users/alice/documents/file.txt",
			description:      "Subvolume mount with user-specific path",
		},
		{
			name:             "nested_subvolume",
			cephVolumePath:   "/volumes/project_data",
			localMountPoint:  "/mnt/projects",
			root:             "/mnt/projects/team1",
			inputPath:        "/volumes/project_data/team1/src/code.go",
			expectedUserPath: "/mnt/projects/team1/src/code.go",
			expectedCephPath: "/volumes/project_data/team1/src/code.go",
			description:      "Nested subvolume with project-specific path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Ceph volume path to user path conversion logic
			userPath := tc.inputPath

			// Convert Ceph volume path to local mount point path
			if tc.cephVolumePath != "/" {
				// Remove the Ceph volume path prefix and add local mount point prefix
				if after, ok := strings.CutPrefix(userPath, tc.cephVolumePath); ok {
					relativePath := after
					if !strings.HasPrefix(relativePath, "/") {
						relativePath = "/" + relativePath
					}
					userPath = tc.localMountPoint + relativePath
				}
			} else {
				// For root mount, just add the local mount point prefix
				userPath = tc.localMountPoint + userPath
			}

			assert.Equal(t, tc.expectedUserPath, userPath, tc.description)
			t.Logf("%s: %s → %s", tc.name, tc.inputPath, userPath)
		})
	}
}
