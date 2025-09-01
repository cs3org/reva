// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package nceph

import (
	"context"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

// convertCephVolumePathToUserPath converts a Ceph volume path (RADOS canonical form)
// to a user-relative path by mapping it through the local filesystem and trimming chrootDir.
// This uses the auto-discovered mapping from fstab to convert between coordinate systems.
func (fs *ncephfs) convertCephVolumePathToUserPath(ctx context.Context, cephVolumePath string) string {
	log := appctx.GetLogger(ctx)
	
	// Step 1: Convert Ceph volume path to local filesystem path using fstab mapping
	// Map from RADOS coordinates to local filesystem coordinates
	// Example: /volumes/cephfs/app/users/alice/file.txt -> /mnt/cephfs/users/alice/file.txt
	localPath := cephVolumePath
	if fs.cephVolumePath != "" && fs.localMountPoint != "" && fs.cephVolumePath != fs.localMountPoint {
		// Use the auto-discovered mapping from fstab
		// Replace the Ceph volume prefix with the local mount point prefix
		localPath = strings.Replace(cephVolumePath, fs.cephVolumePath, fs.localMountPoint, 1)
		
		log.Debug().
			Str("ceph_volume_path", cephVolumePath).
			Str("ceph_volume_prefix", fs.cephVolumePath).
			Str("local_mount_point", fs.localMountPoint).
			Str("local_filesystem_path", localPath).
			Msg("nceph: Mapped Ceph volume path to local filesystem path using fstab mapping")
	}
	
	// Step 2: Convert local filesystem path to user-relative path
	// Trim the chrootDir prefix to get the user-visible path
	// Example: /mnt/cephfs/users/alice/file.txt -> /users/alice/file.txt (if chrootDir = /mnt/cephfs)
	userPath := localPath
	if fs.chrootDir != "" && fs.chrootDir != "/" {
		originalPath := userPath
		userPath = strings.TrimPrefix(userPath, fs.chrootDir)
		
		if originalPath != userPath {
			log.Debug().
				Str("chroot_dir", fs.chrootDir).
				Str("local_filesystem_path", originalPath).
				Str("user_relative_path", userPath).
				Msg("nceph: Converted local filesystem path to user-relative path")
		} else {
			log.Debug().
				Str("chroot_dir", fs.chrootDir).
				Str("local_filesystem_path", localPath).
				Msg("nceph: Local filesystem path does not contain chrootDir prefix - no trimming needed")
		}
	}
	
	// Ensure the user path starts with / for consistency
	if userPath != "" && !strings.HasPrefix(userPath, "/") {
		userPath = "/" + userPath
		log.Debug().
			Str("user_path", userPath).
			Msg("nceph: Added leading slash to user-relative path")
	}
	
	return userPath
}

// convertUserPathToCephVolumePath converts a user-relative path to a Ceph volume path (RADOS canonical form).
// This uses the auto-discovered mapping from fstab to convert between coordinate systems.
func (fs *ncephfs) convertUserPathToCephVolumePath(ctx context.Context, userPath string) string {
	log := appctx.GetLogger(ctx)
	
	// Step 1: Convert user-relative path to local filesystem path
	// Add the chrootDir prefix to get the full local filesystem path
	// Example: /users/alice/file.txt -> /mnt/cephfs/users/alice/file.txt
	localPath := userPath
	if fs.chrootDir != "" && fs.chrootDir != "/" {
		// Ensure proper path joining
		if strings.HasPrefix(userPath, "/") {
			localPath = fs.chrootDir + userPath
		} else {
			localPath = fs.chrootDir + "/" + userPath
		}
		
		log.Debug().
			Str("user_path", userPath).
			Str("chroot_dir", fs.chrootDir).
			Str("local_filesystem_path", localPath).
			Msg("nceph: Converted user-relative path to local filesystem path")
	}
	
	// Step 2: Convert local filesystem path to Ceph volume path using fstab mapping
	// Map from local filesystem coordinates to RADOS coordinates
	// Example: /mnt/cephfs/users/alice/file.txt -> /volumes/cephfs/app/users/alice/file.txt
	cephVolumePath := localPath
	if fs.localMountPoint != "" && fs.cephVolumePath != "" && fs.localMountPoint != fs.cephVolumePath {
		// Handle special case where either prefix is "/"
		if fs.localMountPoint == "/" {
			// If local mount point is "/", just prepend the Ceph volume prefix
			if fs.cephVolumePath == "/" {
				cephVolumePath = localPath // No change needed
			} else {
				cephVolumePath = fs.cephVolumePath + localPath
			}
		} else if fs.cephVolumePath == "/" {
			// If Ceph volume prefix is "/", just remove the local mount point prefix
			cephVolumePath = strings.TrimPrefix(localPath, fs.localMountPoint)
			if cephVolumePath == "" {
				cephVolumePath = "/"
			}
		} else {
			// Normal case: replace local mount point prefix with Ceph volume prefix
			cephVolumePath = strings.Replace(localPath, fs.localMountPoint, fs.cephVolumePath, 1)
		}
		
		log.Debug().
			Str("local_filesystem_path", localPath).
			Str("local_mount_point", fs.localMountPoint).
			Str("ceph_volume_prefix", fs.cephVolumePath).
			Str("ceph_volume_path", cephVolumePath).
			Msg("nceph: Converted local filesystem path to Ceph volume path using fstab mapping")
	}
	
	return cephVolumePath
}
