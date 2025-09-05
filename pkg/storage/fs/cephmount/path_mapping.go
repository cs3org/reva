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

package cephmount

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// validatePathWithinBounds ensures that the given path is within the allowed mount bounds
// and rejects any path traversal attempts that could escape the chroot jail.
func (fs *cephmountfs) validatePathWithinBounds(ctx context.Context, path string, operation string) error {
	log := appctx.GetLogger(ctx)

	// Clean the path to resolve any .. or . components
	cleanPath := filepath.Clean(path)

	log.Debug().
		Str("original_path", path).
		Str("clean_path", cleanPath).
		Str("operation", operation).
		Msg("cephmount: Validating path bounds")

	// Check if the path escapes the bounds through .. traversal
	if strings.Contains(cleanPath, "..") {
		log.Error().
			Str("path", path).
			Str("clean_path", cleanPath).
			Str("operation", operation).
			Msg("cephmount: SECURITY: Path contains .. traversal after cleaning - potential escape attempt")
		return errtypes.PermissionDenied("cephmount: path traversal not allowed")
	}

	// Ensure the path is within the configured Ceph volume bounds
	if fs.cephVolumePath != "" && fs.cephVolumePath != "/" {
		if !strings.HasPrefix(cleanPath, fs.cephVolumePath) {
			log.Error().
				Str("path", cleanPath).
				Str("ceph_volume_prefix", fs.cephVolumePath).
				Str("operation", operation).
				Msg("cephmount: SECURITY: Path is outside configured Ceph volume bounds")
			return errtypes.PermissionDenied("cephmount: path outside configured volume bounds")
		}
	}

	// Additional check: ensure the path when converted to local filesystem stays within chroot
	localPath := cleanPath
	if fs.cephVolumePath != "" && fs.localMountPoint != "" && fs.cephVolumePath != fs.localMountPoint {
		localPath = strings.Replace(cleanPath, fs.cephVolumePath, fs.localMountPoint, 1)
	}

	if fs.chrootDir != "" && fs.chrootDir != "/" {
		if !strings.HasPrefix(localPath, fs.chrootDir) {
			log.Error().
				Str("path", cleanPath).
				Str("local_path", localPath).
				Str("chroot_dir", fs.chrootDir).
				Str("operation", operation).
				Msg("cephmount: SECURITY: Path would escape chroot bounds")
			return errtypes.PermissionDenied("cephmount: path outside chroot bounds")
		}
	}

	log.Debug().
		Str("path", cleanPath).
		Str("operation", operation).
		Msg("cephmount: Path validation passed - within bounds")

	return nil
}

// convertCephVolumePathToUserPath converts a Ceph volume path (RADOS canonical form)
// to a user-relative path by mapping it through the local filesystem and trimming chrootDir.
// This uses the auto-discovered mapping from fstab to convert between coordinate systems.
func (fs *cephmountfs) convertCephVolumePathToUserPath(ctx context.Context, cephVolumePath string) string {
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
			Msg("cephmount: Mapped Ceph volume path to local filesystem path using fstab mapping")
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
				Msg("cephmount: Converted local filesystem path to user-relative path")
		} else {
			log.Debug().
				Str("chroot_dir", fs.chrootDir).
				Str("local_filesystem_path", localPath).
				Msg("cephmount: Local filesystem path does not contain chrootDir prefix - no trimming needed")
		}
	}

	// Ensure the user path starts with / for consistency
	if userPath != "" && !strings.HasPrefix(userPath, "/") {
		userPath = "/" + userPath
		log.Debug().
			Str("user_path", userPath).
			Msg("cephmount: Added leading slash to user-relative path")
	}

	return userPath
}

// convertUserPathToCephVolumePath converts a user-relative path to a Ceph volume path (RADOS canonical form).
// This uses the auto-discovered mapping from fstab to convert between coordinate systems.
func (fs *cephmountfs) convertUserPathToCephVolumePath(ctx context.Context, userPath string) string {
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
			Msg("cephmount: Converted user-relative path to local filesystem path")
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
			Msg("cephmount: Converted local filesystem path to Ceph volume path using fstab mapping")
	}

	return cephVolumePath
}
