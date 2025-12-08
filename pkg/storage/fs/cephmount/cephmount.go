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

// Package cephmount provides a local filesystem implementation that mimics ceph operations
package cephmount

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/maxymania/go-system/posix_acl"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	xattrUserNs      = "user."
	xattrLock        = xattrUserNs + "reva.lockpayload"
	aclXattrAccess   = "system.posix_acl_access"
	aclXattrDefault  = "system.posix_acl_default"
)

// cephmountfs is a local filesystem implementation that provides a ceph-like interface
type cephmountfs struct {
	conf            *Options
	cephAdminConn   *CephAdminConn  // Only used for GetPathByID (defined in build-tag files)
	rootFS          *os.Root        // Chrooted filesystem root using os.Root
	threadPool      *UserThreadPool // Pool of per-user threads with dedicated UIDs
	cephVolumePath  string          // Auto-discovered Ceph volume path (RADOS canonical form)
	localMountPoint string          // Auto-discovered local mount point (where Ceph is mounted locally), see fstab
	chrootDir       string          // The local mount point (see fstab), but configurable for unit tests
}

func init() {
	registry.Register("cephmount", New)
}

func parseOptions(m map[string]any) (Options, error) {
	var o Options
	if err := mapstructure.Decode(m, &o); err != nil {
		return o, errors.Wrap(err, "error decoding conf")
	}
	o.ApplyDefaults()
	return o, nil
}

// New returns an implementation of the storage.FS interface that talks to
// the local filesystem using os.Root operations instead of libcephfs.
func New(ctx context.Context, m map[string]any) (storage.FS, error) {
	o, err := parseOptions(m)
	if err != nil {
		return nil, err
	}

	// FstabEntry is now required since manual configuration has been removed
	// However, for testing purposes, TestingAllowLocalMode can bypass this requirement
	if o.FstabEntry == "" && !o.TestingAllowLocalMode {
		return nil, errors.New("cephmount: fstabentry must be provided (manual configuration has been removed)")
	}

	log := appctx.GetLogger(ctx)
	var discoveredCephVolumePath string
	var discoveredLocalMountPoint string

	if o.FstabEntry != "" {
		// Parse Ceph configuration from fstab entry
		log.Info().Str("fstab_entry", o.FstabEntry).Msg("cephmount: Parsing Ceph configuration from fstab entry")

		mountInfo, err := ParseFstabEntry(ctx, o.FstabEntry)
		if err != nil {
			log.Error().Err(err).Msg("cephmount: Failed to parse fstab entry")
			return nil, errors.Wrap(err, "cephmount: failed to parse fstab entry")
		}

		// Store discovered values
		discoveredCephVolumePath = mountInfo.CephVolumePath
		discoveredLocalMountPoint = mountInfo.LocalMountPoint

		log.Info().
			Str("ceph_volume_path", mountInfo.CephVolumePath).
			Str("local_mount_point", mountInfo.LocalMountPoint).
			Str("monitor_host", mountInfo.MonitorHost).
			Str("client_name", mountInfo.ClientName).
			Msg("cephmount: Successfully parsed fstab entry")

	} else if o.TestingAllowLocalMode {
		// Local mode for testing - no Ceph configuration
		log.Info().Msg("cephmount: Running in local mode (Ceph features disabled)")
		discoveredCephVolumePath = ""
		discoveredLocalMountPoint = ""
	}

	// Use discovered local mount point as chroot directory
	chrootDir := discoveredLocalMountPoint

	// Override chroot directory from environment variable for testing (does not pollute Options)
	if testChrootDir := os.Getenv("CEPHMOUNT_TEST_CHROOT_DIR"); testChrootDir != "" {
		log.Info().
			Str("original_chroot_dir", chrootDir).
			Str("override_chroot_dir", testChrootDir).
			Msg("cephmount: Overriding chroot directory from CEPHMOUNT_TEST_CHROOT_DIR environment variable")
		chrootDir = testChrootDir
	}

	// Validate that we have a chroot directory
	if chrootDir == "" {
		return nil, errors.New("cephmount: no chroot directory available (either provide fstabentry or set CEPHMOUNT_TEST_CHROOT_DIR for testing)")
	}

	log.Info().
		Str("ceph_volume_path", discoveredCephVolumePath).
		Str("chroot_dir", chrootDir).
		Msg("cephmount: Configuration applied")

	// Ensure chroot directory exists and get absolute path
	absChrootDir, err := filepath.Abs(chrootDir)
	if err != nil {
		return nil, errors.Wrap(err, "cephmount: failed to get absolute path for chroot directory")
	}

	// Create a chrooted filesystem using os.OpenRoot to jail all operations to the local mount point
	rootFS, err := os.OpenRoot(absChrootDir)
	if err != nil {
		return nil, errors.Wrap(err, "cephmount: failed to create chroot jail with os.OpenRoot")
	}

	// Initialize ceph admin connection if fstab entry was parsed successfully
	var cephAdminConn *CephAdminConn
	if o.FstabEntry != "" && discoveredCephVolumePath != "" {
		// Use the updated newCephAdminConn which will parse the fstab entry internally
		cephAdminConn, err = newCephAdminConn(ctx, &o)
		if err != nil {
			// Log warning but continue - GetPathByID will not work but other operations will
			log.Warn().Err(err).Msg("cephmount: failed to create ceph admin connection, GetPathByID will not work")
		}
	}

	// Initialize user thread pool for per-user filesystem operations
	threadPool, privResult, err := NewUserThreadPool(UserThreadPoolConfig{
		ThreadTTL:     5 * time.Minute, // Keep threads alive for 5 minutes after last use
		CleanupPeriod: 1 * time.Minute, // Check for expired threads every minute
		NobodyUID:     o.NobodyUID,     // Use configured nobody UID
		NobodyGID:     o.NobodyGID,     // Use configured nobody GID
	})
	if err != nil {
		return nil, errors.Wrap(err, "cephmount: failed to initialize user thread pool")
	}

	// Log privilege verification results
	// Reuse the logger from auto-discovery above

	// Always log basic privilege status first
	log.Info().
		Int("current_uid", privResult.CurrentUID).
		Int("current_gid", privResult.CurrentGID).
		Int("current_fsuid", privResult.CurrentFsUID).
		Int("current_fsgid", privResult.CurrentFsGID).
		Bool("can_change_uid", privResult.CanChangeUID).
		Bool("can_change_gid", privResult.CanChangeGID).
		Msg("cephmount: privilege verification status")

	// Log detailed test information
	log.Info().
		Interface("tested_uids", privResult.TestedUIDs).
		Interface("tested_gids", privResult.TestedGIDs).
		Int("target_nobody_uid", o.NobodyUID).
		Int("target_nobody_gid", o.NobodyGID).
		Msg("cephmount: privilege verification details")

	// Verify that privilege testing properly restored original fsuid/fsgid
	finalFsUID := setfsuidSafe(-1)
	finalFsGID := setfsgidSafe(-1)

	log.Info().
		Int("original_fsuid", privResult.CurrentFsUID).
		Int("final_fsuid", finalFsUID).
		Int("original_fsgid", privResult.CurrentFsGID).
		Int("final_fsgid", finalFsGID).
		Bool("fsuid_restored", finalFsUID == privResult.CurrentFsUID).
		Bool("fsgid_restored", finalFsGID == privResult.CurrentFsGID).
		Msg("cephmount: privilege verification restoration status")

	if finalFsUID != privResult.CurrentFsUID {
		log.Error().
			Int("expected_fsuid", privResult.CurrentFsUID).
			Int("actual_fsuid", finalFsUID).
			Msg("cephmount: CRITICAL - privilege verification failed to restore original fsuid - this will cause permission issues")
	}

	if finalFsGID != privResult.CurrentFsGID {
		log.Error().
			Int("expected_fsgid", privResult.CurrentFsGID).
			Int("actual_fsgid", finalFsGID).
			Msg("cephmount: CRITICAL - privilege verification failed to restore original fsgid - this will cause permission issues")
	}

	if !privResult.HasSufficientPrivileges() {
		if privResult.HasPartialPrivileges() {
			log.Warn().
				Bool("can_change_uid", privResult.CanChangeUID).
				Bool("can_change_gid", privResult.CanChangeGID).
				Interface("error_messages", privResult.ErrorMessages).
				Interface("recommendations", privResult.Recommendations).
				Str("impact", "some per-user operations may not work correctly").
				Msg("cephmount: partial privileges detected")
		} else {
			log.Error().
				Int("current_uid", privResult.CurrentUID).
				Int("current_gid", privResult.CurrentGID).
				Int("current_fsuid", privResult.CurrentFsUID).
				Int("current_fsgid", privResult.CurrentFsGID).
				Interface("tested_uids", privResult.TestedUIDs).
				Interface("tested_gids", privResult.TestedGIDs).
				Interface("error_messages", privResult.ErrorMessages).
				Interface("recommendations", privResult.Recommendations).
				Str("impact", "per-user thread isolation will not work - all operations will run as current user").
				Msg("cephmount: insufficient privileges for setfsuid/setfsgid")
		}
	} else {
		log.Info().
			Bool("can_change_uid", privResult.CanChangeUID).
			Bool("can_change_gid", privResult.CanChangeGID).
			Int("current_uid", privResult.CurrentUID).
			Int("current_gid", privResult.CurrentGID).
			Int("current_fsuid", privResult.CurrentFsUID).
			Int("current_fsgid", privResult.CurrentFsGID).
			Int("nobody_uid", o.NobodyUID).
			Int("nobody_gid", o.NobodyGID).
			Interface("tested_uids", privResult.TestedUIDs).
			Interface("tested_gids", privResult.TestedGIDs).
			Str("capability", "full per-user thread isolation available").
			Msg("cephmount: sufficient privileges verified for per-user thread isolation")
	}

	return &cephmountfs{
		conf:            &o,
		cephAdminConn:   cephAdminConn,
		rootFS:          rootFS,
		threadPool:      threadPool,
		cephVolumePath:  discoveredCephVolumePath,
		localMountPoint: discoveredLocalMountPoint,
		chrootDir:       chrootDir,
	}, nil
}

// resolveRef converts a provider.Reference to a chroot-relative path
func (fs *cephmountfs) resolveRef(ctx context.Context, ref *provider.Reference) (string, error) {
	switch {
	case ref.Path != "":
		// Convert external path to chroot-relative path
		return fs.toChroot(ref.Path), nil
	case ref.ResourceId != nil:
		// For resource IDs, use GetPathByID
		if ref.ResourceId.StorageId != "" && ref.ResourceId.OpaqueId != "" {
			externalPath, err := fs.GetPathByID(ctx, ref.ResourceId)
			if err != nil {
				return "", err
			}
			// Convert the external path to chroot-relative
			return fs.toChroot(externalPath), nil
		}
		return "", errors.New("cephmount: invalid resource id")
	default:
		return "", errors.New("cephmount: invalid reference")
	}
}

// fileAsResourceInfo converts file info to ResourceInfo without user context
func (fs *cephmountfs) fileAsResourceInfo(path string, info os.FileInfo, mdKeys []string) (*provider.ResourceInfo, error) {
	var (
		resourceType provider.ResourceType
		target       string
		size         uint64
	)

	// Determine resource type
	if info.IsDir() {
		resourceType = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	} else if info.Mode()&os.ModeSymlink != 0 {
		resourceType = provider.ResourceType_RESOURCE_TYPE_SYMLINK
		// For symlinks, we need to get the absolute filesystem path to read the link
		var absPath string
		if path == "." {
			absPath = fs.chrootDir
		} else {
			absPath = filepath.Join(fs.chrootDir, path)
		}
		if linkTarget, err := os.Readlink(absPath); err == nil {
			target = linkTarget
		}
	} else {
		resourceType = provider.ResourceType_RESOURCE_TYPE_FILE
		size = uint64(info.Size())
	}

	// Get file system stat for additional info
	stat := info.Sys().(*syscall.Stat_t)

	// Create resource ID using inode number
	resourceId := &provider.ResourceId{
		//StorageId: "cephmount",
		OpaqueId:  strconv.FormatUint(stat.Ino, 10),
	}

	owner, _ := user.LookupId(fmt.Sprint(stat.Uid))

	ri := &provider.ResourceInfo{
		Type:     resourceType,
		Id:       resourceId,
		Checksum: &provider.ResourceChecksum{},
		Size:     size,
		Mtime:    &typepb.Timestamp{Seconds: uint64(info.ModTime().Unix())},
		Path:     fs.fromChroot(path),                   // Convert chroot path back to external path
		Owner:    &userv1beta1.UserId{OpaqueId: owner.Username}, 
		PermissionSet: &provider.ResourcePermissions{
			GetPath:              true,
                        GetQuota:             true,
                        InitiateFileDownload: true,
                        ListGrants:           true,
                        ListContainer:        true,
                        ListFileVersions:     true,
                        ListRecycle:          true,
                        Stat:                 true,
                        InitiateFileUpload:   true,
                        RestoreFileVersion:   true,
                        RestoreRecycleItem:   true,
                        Move:                 true,
                        CreateContainer:      true,
                        Delete:               true,
                        PurgeRecycle:         true,

                        // these permissions only make sense to enforce them in the root of the storage space.
                        AddGrant:    true, // managers can add users to the space
                        RemoveGrant: true, // managers can remove users from the space
                        UpdateGrant: true,
                        DenyGrant:   true,
		},
		ArbitraryMetadata: &provider.ArbitraryMetadata{Metadata: map[string]string{}},
	}

	// Set target for symlinks
	if target != "" {
		ri.Target = target
	}

	// Set MIME type for files
	if resourceType == provider.ResourceType_RESOURCE_TYPE_FILE {
		if mimeType := mime.Detect(false, info.Name()); mimeType != "" {
			ri.MimeType = mimeType
		}
	}

	// Set inode and device info
	ri.ArbitraryMetadata.Metadata["inode"] = strconv.FormatUint(stat.Ino, 10)
	ri.ArbitraryMetadata.Metadata["device"] = strconv.FormatUint(uint64(stat.Dev), 10)

	return ri, nil
}

// toChroot converts an external path to a chroot-relative path
// External paths like "/some/file.txt" become "some/file.txt"
func (fs *cephmountfs) toChroot(externalPath string) string {
	// Clean the path and remove leading slash to make it relative
	cleanPath := filepath.Clean(externalPath)
	if cleanPath == "/" || cleanPath == "." {
		return "."
	}
	// Remove leading slash to make it relative to chroot
	return strings.TrimPrefix(cleanPath, "/")
}

// fromChroot converts a chroot-relative path back to external logical path
// Chroot paths like "some/file.txt" become "/some/file.txt"
func (fs *cephmountfs) fromChroot(chrootPath string) string {
	if chrootPath == "." {
		return "/"
	}
	// Ensure the returned path starts with /
	if strings.HasPrefix(chrootPath, "/") {
		return chrootPath
	}
	return "/" + chrootPath
}

func (fs *cephmountfs) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("cephmount: GetHome not implemented")
}

func (fs *cephmountfs) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("cephmount: CreateHome not implemented")
}

func (fs *cephmountfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	// Capture the original received path for logging
	var receivedPath string
	if ref != nil && ref.Path != "" {
		receivedPath = ref.Path
	} else if ref != nil && ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperationWithPaths(ctx, "CreateDir", receivedPath, path)

	// Execute directory creation on user's thread with correct UID
	err = fs.createDirectoryAsUser(ctx, path, os.FileMode(fs.conf.DirPerms))
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to create directory")
		fs.logOperationError(ctx, "CreateDir", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	// Capture the original received path for logging
	var receivedPath string
	if ref != nil && ref.Path != "" {
		receivedPath = ref.Path
	} else if ref != nil && ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperationWithPaths(ctx, "Delete", receivedPath, path)

	// Execute stat and delete operations on user's thread with correct UID
	info, err := fs.statAsUser(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		wrappedErr := errors.Wrap(err, "cephmount: failed to stat file for deletion")
		fs.logOperationError(ctx, "Delete", path, wrappedErr)
		return wrappedErr
	}

	if info.IsDir() {
		err = fs.removeAllAsUser(ctx, path)
	} else {
		err = fs.removeAsUser(ctx, path)
	}

	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to delete")
		fs.logOperationError(ctx, "Delete", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	// Capture the original received paths for logging
	var oldReceivedPath, newReceivedPath string
	if oldRef != nil && oldRef.Path != "" {
		oldReceivedPath = oldRef.Path
	} else if oldRef != nil && oldRef.ResourceId != nil {
		oldReceivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", oldRef.ResourceId.StorageId, oldRef.ResourceId.OpaqueId)
	}
	if newRef != nil && newRef.Path != "" {
		newReceivedPath = newRef.Path
	} else if newRef != nil && newRef.ResourceId != nil {
		newReceivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", newRef.ResourceId.StorageId, newRef.ResourceId.OpaqueId)
	}

	oldPath, err := fs.resolveRef(ctx, oldRef)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve old path")
		fs.logOperationError(ctx, "Move", "", wrappedErr)
		return wrappedErr
	}
	newPath, err := fs.resolveRef(ctx, newRef)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve new path")
		fs.logOperationError(ctx, "Move", oldPath, wrappedErr)
		return wrappedErr
	}

	fs.logOperationWithPaths(ctx, "Move", fmt.Sprintf("%s -> %s", oldReceivedPath, newReceivedPath), fmt.Sprintf("%s -> %s", oldPath, newPath))

	// oldPath and newPath are already chroot-relative from resolveRef
	// Create parent directory if needed and execute move on user's thread with correct UID
	parentPath := path.Dir(newPath)
	if parentPath != "." {
		if err := fs.createDirectoryAsUser(ctx, parentPath, os.FileMode(fs.conf.DirPerms)); err != nil {
			wrappedErr := errors.Wrap(err, "cephmount: failed to create parent directory for move")
			fs.logOperationError(ctx, "Move", fmt.Sprintf("%s -> %s", oldPath, newPath), wrappedErr)
			return wrappedErr
		}
	}

	err = fs.renameAsUser(ctx, oldPath, newPath)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to move file")
		fs.logOperationError(ctx, "Move", fmt.Sprintf("%s -> %s", oldPath, newPath), wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	if ref == nil {
		wrappedErr := errors.New("error: ref is nil")
		fs.logOperationError(ctx, "GetMD", "", wrappedErr)
		return nil, wrappedErr
	}

	log := appctx.GetLogger(ctx)

	// Capture the original received path for logging
	var receivedPath string
	if ref.Path != "" {
		receivedPath = ref.Path
	} else if ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "GetMD", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperationWithPaths(ctx, "GetMD", receivedPath, path)

	// path is already chroot-relative from resolveRef
	// Execute stat operation on user's thread with correct UID
	info, err := fs.statAsUser(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			wrappedErr := errtypes.NotFound("file not found")
			fs.logOperationError(ctx, "GetMD", path, wrappedErr)
			return nil, wrappedErr
		}
		wrappedErr := errors.Wrap(err, "cephmount: failed to stat file")
		fs.logOperationError(ctx, "GetMD", path, wrappedErr)
		return nil, wrappedErr
	}

	ri, err = fs.fileAsResourceInfo(path, info, mdKeys)
	if err != nil {
		log.Debug().Any("resourceInfo", ri).Err(err).Msg("fileAsResourceInfo returned error")
		wrappedErr := errors.Wrap(err, "cephmount: failed to convert file to resource info")
		fs.logOperationError(ctx, "GetMD", path, wrappedErr)
		return nil, wrappedErr
	}

	return ri, nil
}

func (fs *cephmountfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (files []*provider.ResourceInfo, err error) {
	if ref == nil {
		wrappedErr := errors.New("error: ref is nil")
		fs.logOperationError(ctx, "ListFolder", "", wrappedErr)
		return nil, wrappedErr
	}

	log := appctx.GetLogger(ctx)

	// Capture the original received path for logging
	var receivedPath string
	if ref.Path != "" {
		receivedPath = ref.Path
	} else if ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "ListFolder", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperationWithPaths(ctx, "ListFolder", receivedPath, path)

	// INFO: About to call readDirectoryAsUser
	log.Debug().
		Str("operation", "ListFolder").
		Str("chroot_path", path).
		Str("full_filesystem_path", filepath.Join(fs.chrootDir, path)).
		Msg("cephmount ListFolder about to call readDirectoryAsUser")

	// Execute directory listing on user's thread with correct UID
	entries, err := fs.readDirectoryAsUser(ctx, path)
	if err != nil {
		// INFO: readDirectoryAsUser failed
		log.Debug().
			Str("operation", "ListFolder").
			Str("chroot_path", path).
			Str("full_filesystem_path", filepath.Join(fs.chrootDir, path)).
			Err(err).
			Msg("cephmount ListFolder readDirectoryAsUser failed")

		wrappedErr := errors.Wrap(err, "cephmount: failed to read directory")
		fs.logOperationError(ctx, "ListFolder", path, wrappedErr)
		return nil, wrappedErr
	}

	// INFO: readDirectoryAsUser succeeded
	log.Debug().
		Str("operation", "ListFolder").
		Str("chroot_path", path).
		Str("full_filesystem_path", filepath.Join(fs.chrootDir, path)).
		Int("entries_returned", len(entries)).
		Msg("cephmount ListFolder readDirectoryAsUser succeeded")

	// Debug log what entries were found from filesystem
	log.Debug().
		Str("operation", "ListFolder").
		Str("filesystem_path", path).
		Int("raw_entries_found", len(entries)).
		Msg("cephmount ListFolder raw directory read completed")

	// Log individual raw entries if there are any
	for i, entry := range entries {
		log.Trace().
			Str("operation", "ListFolder").
			Int("entry_index", i).
			Str("entry_name", entry.Name()).
			Bool("is_dir", entry.IsDir()).
			Str("filesystem_path", path).
			Msg("cephmount ListFolder found raw directory entry")
	}

	for _, entry := range entries {
		if fs.conf.HiddenDirs[entry.Name()] {
			log.Debug().
				Str("operation", "ListFolder").
				Str("entry_name", entry.Name()).
				Str("reason", "hidden_directory").
				Msg("cephmount ListFolder skipping entry")
			continue
		}

		ri, err := fs.fileAsResourceInfo(filepath.Join(path, entry.Name()), entry, mdKeys)
		if ri == nil || err != nil {
			if err != nil {
				log.Debug().
					Str("operation", "ListFolder").
					Str("entry_name", entry.Name()).
					Str("reason", "fileAsResourceInfo_error").
					Err(err).
					Any("resourceInfo", ri).
					Msg("cephmount ListFolder skipping entry")
			} else {
				log.Debug().
					Str("operation", "ListFolder").
					Str("entry_name", entry.Name()).
					Str("reason", "fileAsResourceInfo_returned_nil").
					Msg("cephmount ListFolder skipping entry")
			}
			continue
		}

		files = append(files, ri)

		// Debug log each entry being returned
		log.Trace().
			Str("operation", "ListFolder").
			Str("entry_path", ri.Path).
			Str("entry_name", entry.Name()).
			Str("entry_type", ri.Type.String()).
			Uint64("entry_size", ri.Size).
			Str("entry_id", ri.Id.OpaqueId).
			Str("storage_id", ri.Id.StorageId).
			Str("filesystem_path", filepath.Join(path, entry.Name())).
			Str("chroot_path_input", filepath.Join(path, entry.Name())).
			Str("fromChroot_output", fs.fromChroot(filepath.Join(path, entry.Name()))).
			Msg("cephmount ListFolder returning entry - PATH DEBUG")
	}

	// Debug log summary of all entries returned
	log.Debug().
		Str("operation", "ListFolder").
		Str("requested_path", receivedPath).
		Str("filesystem_path", path).
		Int("total_entries", len(files)).
		Msg("cephmount ListFolder operation completed")

	return files, nil
}

func (fs *cephmountfs) Download(ctx context.Context, ref *provider.Reference, ranges []storage.Range) (rc io.ReadCloser, err error) {
	if len(ranges) > 0 {
		return nil, errtypes.NotSupported("Download with ranges is not supported with this storage driver")
	}

	// Capture the original received path for logging
	var receivedPath string
	if ref != nil && ref.Path != "" {
		receivedPath = ref.Path
	} else if ref != nil && ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: error resolving ref")
		fs.logOperationError(ctx, "Download", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperationWithPaths(ctx, "Download", receivedPath, path)

	// Execute file open on user's thread with correct UID
	file, err := fs.openFileAsUser(ctx, path)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to open file for download")
		fs.logOperationError(ctx, "Download", path, wrappedErr)
		return nil, wrappedErr
	}

	return file, nil
}

// Upload handles file uploads to the local filesystem
func (fs *cephmountfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	// Capture the original received path for logging
	var receivedPath string
	if ref != nil && ref.Path != "" {
		receivedPath = ref.Path
	} else if ref != nil && ref.ResourceId != nil {
		receivedPath = fmt.Sprintf("ResourceId{StorageId:%s, OpaqueId:%s}", ref.ResourceId.StorageId, ref.ResourceId.OpaqueId)
	}

	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: error resolving reference")
		fs.logOperationError(ctx, "Upload", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperationWithPaths(ctx, "Upload", receivedPath, path)

	// Create parent directory if needed and execute upload on user's thread with correct UID
	parentDir := filepath.Dir(path)
	if parentDir != "." {
		if err := fs.createDirectoryAsUser(ctx, parentDir, os.FileMode(fs.conf.DirPerms)); err != nil {
			wrappedErr := errors.Wrap(err, "cephmount: failed to create parent directory for upload")
			fs.logOperationError(ctx, "Upload", path, wrappedErr)
			return wrappedErr
		}
	}

	// Create and upload the file on user's thread
	err = fs.uploadFileAsUser(ctx, path, r, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: error uploading file")
		fs.logOperationError(ctx, "Upload", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: error resolving reference")
		fs.logOperationError(ctx, "InitiateUpload", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "InitiateUpload", fmt.Sprintf("%s (length: %d)", path, uploadLength))

	return map[string]string{
		"simple": path,
	}, nil
}

func (fs *cephmountfs) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	wrappedErr := errtypes.NotSupported("cephmount: ListRevisions not supported")
	fs.logOperationError(ctx, "ListRevisions", "", wrappedErr)
	return nil, wrappedErr
}

func (fs *cephmountfs) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	wrappedErr := errtypes.NotSupported("cephmount: DownloadRevision not supported")
	fs.logOperationError(ctx, "DownloadRevision", "", wrappedErr)
	return nil, wrappedErr
}

func (fs *cephmountfs) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	wrappedErr := errtypes.NotSupported("cephmount: RestoreRevision not supported")
	fs.logOperationError(ctx, "RestoreRevision", "", wrappedErr)
	return wrappedErr
}

func (fs *cephmountfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "AddGrant", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "AddGrant", path)

	// Use setfacl system command to set permissions
	err = fs.addGrantViaSetfacl(ctx, path, g)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to add grant via setfacl")
		fs.logOperationError(ctx, "AddGrant", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "RemoveGrant", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "RemoveGrant", path)

	// Use setfacl system command to remove permissions
	err = fs.removeGrantViaSetfacl(ctx, path, g)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to remove grant via setfacl")
		fs.logOperationError(ctx, "RemoveGrant", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *cephmountfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	// Update is the same as add for setfacl
	return fs.AddGrant(ctx, ref, g)
}

func (fs *cephmountfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) (err error) {
	// Deny is the same as remove
	grant := &provider.Grant{
		Grantee:     g,
		Permissions: &provider.ResourcePermissions{},
	}
	return fs.RemoveGrant(ctx, ref, grant)
}

func (fs *cephmountfs) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "ListGrants", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "ListGrants", path)

	fullPath := filepath.Join(fs.chrootDir, path)
	
	// Use getfacl to read ACLs
	cmd := exec.CommandContext(ctx, "getfacl", "--omit-header", "--numeric", fullPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// No ACLs or error - return empty list
		return []*provider.Grant{}, nil
	}
	
	log := appctx.GetLogger(ctx)
	
	// Parse getfacl output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Parse ACL entry format: user:uid:rwx or group:gid:rwx (numeric due to --numeric flag)
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		
		entryType := parts[0]
		identifier := parts[1] // This is numeric UID or GID
		permsStr := parts[2]
		
		// Skip base entries (owner, group, other, mask)
		if identifier == "" || entryType == "mask" || entryType == "other" {
			continue
		}
		
		// Convert rwx string to ResourcePermissions
		perms := fs.aclStringToPermissions(permsStr)
		
		var grant *provider.Grant
		switch entryType {
		case "user":
			// Resolve numeric UID back to username
			userInfo, err := user.LookupId(identifier)
			if err != nil {
				// Cannot resolve UID to username - skip this entry
				log.Debug().
					Str("uid", identifier).
					Err(err).
					Msg("cephmount: skipping user ACL entry - cannot resolve UID to username")
				continue
			}
			
			grant = &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userv1beta1.UserId{OpaqueId: userInfo.Username}},
				},
				Permissions: perms,
			}
			
		case "group":
			// Resolve numeric GID back to groupname
			groupInfo, err := user.LookupGroupId(identifier)
			if err != nil {
				// Cannot resolve GID to groupname - skip this entry
				log.Debug().
					Str("gid", identifier).
					Err(err).
					Msg("cephmount: skipping group ACL entry - cannot resolve GID to groupname")
				continue
			}
			
			grant = &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
					Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: groupInfo.Name}},
				},
				Permissions: perms,
			}
			
		default:
			continue
		}
		
		if grant != nil {
			glist = append(glist, grant)
		}
	}

	return glist, nil
}

// updatePerms updates ResourcePermissions based on rwx string
func updatePerms(rp *provider.ResourcePermissions, rwx string) {
	if rp == nil {
		return
	}
	if strings.ContainsRune(rwx, 'r') {
		rp.Stat = true
		rp.GetPath = true
		rp.GetQuota = true
		rp.InitiateFileDownload = true
		rp.ListGrants = true
	}
	if strings.ContainsRune(rwx, 'w') {
		rp.CreateContainer = true
		rp.Delete = true
		rp.InitiateFileUpload = true
		rp.Move = true
		rp.PurgeRecycle = true
		rp.RestoreFileVersion = true
		rp.RestoreRecycleItem = true
	}
	if strings.ContainsRune(rwx, 'x') {
		rp.ListRecycle = true
		rp.ListContainer = true
		rp.ListFileVersions = true
	}
}

func (fs *cephmountfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, used uint64, err error) {
	log := appctx.GetLogger(ctx)

	// Get user home path for quota check
	homePath, err := fs.resolveRef(ctx, &provider.Reference{Path: "."})
	if err != nil {
		return 0, 0, errors.Wrap(err, "cephmount: error resolving home path")
	}

	// log homepath
	log.Debug().Str("operation", "GetQuota").
		Str("home_path", homePath).
		Str("full_filesystem_path", filepath.Join(fs.chrootDir, homePath)).
		Msg("cephmount GetQuota resolved home path")

	// Get max quota from extended attributes or use default
	fullHomePath := filepath.Join(fs.chrootDir, homePath)
	maxQuotaData, err := xattr.Get(fullHomePath, "user.quota.max_bytes")
	if err != nil {
		log.Debug().Msg("cephmount: user.quota.max_bytes xattr not set, using default")
		total = fs.conf.UserQuotaBytes
	} else {
		total, _ = strconv.ParseUint(string(maxQuotaData), 10, 64)
	}

	// Get used quota from extended attributes or use default
	usedQuotaData, err := xattr.Get(fullHomePath, "ceph.dir.rbytes")
	if err != nil {
		log.Debug().Msg("cephmount: ceph.dir.rbytes xattr not set, using 0")
	} else {
		used, _ = strconv.ParseUint(string(usedQuotaData), 10, 64)
	}


	return total, used, nil
}

func (fs *cephmountfs) calculateDirectorySize(root string) (uint64, error) {
	var size uint64

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += uint64(info.Size())
		}
		return nil
	})

	return size, err
}

func (fs *cephmountfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	return errors.New("error: CreateReference not implemented")
}

func (fs *cephmountfs) Shutdown(ctx context.Context) (err error) {
	// Clean up ceph connection if it exists
	if fs.cephAdminConn != nil {
		// Currently disabled to avoid ceph dependencies
		// if fs.cephAdminConn.adminMount != nil {
		//     _ = fs.cephAdminConn.adminMount.Unmount()
		//     _ = fs.cephAdminConn.adminMount.Release()
		// }
		// if fs.cephAdminConn.radosConn != nil {
		//     fs.cephAdminConn.radosConn.Shutdown()
		// }
	}
	return nil
}

func (fs *cephmountfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "SetArbitraryMetadata", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "SetArbitraryMetadata", path)

	fullPath := filepath.Join(fs.chrootDir, path)
	for k, v := range md.Metadata {
		if !strings.HasPrefix(k, xattrUserNs) {
			k = xattrUserNs + k
		}
		if err := xattr.Set(fullPath, k, []byte(v)); err != nil {
			wrappedErr := errors.Wrap(err, "cephmount: failed to set xattr")
			fs.logOperationError(ctx, "SetArbitraryMetadata", path, wrappedErr)
			return wrappedErr
		}
	}

	return nil
}

func (fs *cephmountfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "cephmount: failed to resolve reference")
		fs.logOperationError(ctx, "UnsetArbitraryMetadata", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "UnsetArbitraryMetadata", path)

	fullPath := filepath.Join(fs.chrootDir, path)
	for _, key := range keys {
		if !strings.HasPrefix(key, xattrUserNs) {
			key = xattrUserNs + key
		}
		if err := xattr.Remove(fullPath, key); err != nil {
			// Ignore if the attribute doesn't exist
			if !strings.Contains(err.Error(), "no such attribute") {
				wrappedErr := errors.Wrap(err, "cephmount: failed to remove xattr")
				fs.logOperationError(ctx, "UnsetArbitraryMetadata", path, wrappedErr)
				return wrappedErr
			}
		}
	}

	return nil
}

func (fs *cephmountfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "TouchFile", path)

	// Create parent directory if needed using chrooted operations
	parentDir := filepath.Dir(path)
	if parentDir != "." {
		if err := fs.rootFS.MkdirAll(parentDir, os.FileMode(fs.conf.DirPerms)); err != nil {
			return errors.Wrap(err, "cephmount: failed to create parent directory")
		}
	}

	// Use chrooted file operations
	file, err := fs.rootFS.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to touch file")
	}
	defer file.Close()

	return nil
}

func (fs *cephmountfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (r *provider.CreateStorageSpaceResponse, err error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephmountfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

// Lock implementation using file locks

func encodeLock(l *provider.Lock) string {
	data, _ := json.Marshal(l)
	return b64.StdEncoding.EncodeToString(data)
}

func decodeLock(content string) (*provider.Lock, error) {
	d, err := b64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, err
	}

	l := &provider.Lock{}
	if err = json.Unmarshal(d, l); err != nil {
		return nil, err
	}

	return l, nil
}

func (fs *cephmountfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "SetLock", path)

	// Open the file for locking
	file, err := os.OpenFile(path, os.O_RDWR, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to open file for locking")
	}
	defer file.Close()

	// Try to acquire a file lock
	lockType := syscall.LOCK_EX
	if lock.Type == provider.LockType_LOCK_TYPE_SHARED {
		lockType = syscall.LOCK_SH
	}

	if err := syscall.Flock(int(file.Fd()), lockType|syscall.LOCK_NB); err != nil {
		return errors.Wrap(err, "cephmount: failed to acquire file lock")
	}

	// Store lock metadata as extended attribute
	md := &provider.ArbitraryMetadata{
		Metadata: map[string]string{
			xattrLock: encodeLock(lock),
		},
	}
	return fs.SetArbitraryMetadata(ctx, ref, md)
}

func (fs *cephmountfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	fs.logOperation(ctx, "GetLock", path)

	// Try to read lock metadata
	fullPath := filepath.Join(fs.chrootDir, path)
	buf, err := xattr.Get(fullPath, xattrLock)
	if err != nil {
		if strings.Contains(err.Error(), "no such attribute") {
			return nil, errtypes.NotFound("file was not locked")
		}
		return nil, errors.Wrap(err, "cephmount: failed to get lock xattr")
	}

	lock, err := decodeLock(string(buf))
	if err != nil {
		return nil, errors.Wrap(err, "malformed lock payload")
	}

	// Check if lock has expired
	if time.Unix(int64(lock.Expiration.Seconds), 0).Before(time.Now()) {
		// Lock expired, remove it
		fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
		return nil, errtypes.NotFound("lock has expired")
	}

	return lock, nil
}

func (fs *cephmountfs) RefreshLock(ctx context.Context, ref *provider.Reference, newLock *provider.Lock, existingLockID string) error {
	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	// Check if the holder is the same
	if !sameHolder(oldLock, newLock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	if existingLockID != "" && oldLock.LockId != existingLockID {
		return errtypes.BadRequest("lock id does not match")
	}

	return fs.SetLock(ctx, ref, newLock)
}

func (fs *cephmountfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "Unlock", path)

	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			return errtypes.BadRequest("file not found or not locked")
		default:
			return err
		}
	}

	// Check if the lock id matches
	if oldLock.LockId != lock.LockId {
		return errtypes.BadRequest("lock id does not match")
	}

	if !sameHolder(oldLock, lock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	// Open the file and unlock it
	file, err := os.OpenFile(path, os.O_RDWR, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to open file for unlocking")
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return errors.Wrap(err, "cephmount: failed to release file lock")
	}

	// Remove lock metadata
	return fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
}

// addGrantViaSetfacl adds a grant using the setfacl system command
func (fs *cephmountfs) addGrantViaSetfacl(ctx context.Context, path string, grant *provider.Grant) error {
	log := appctx.GetLogger(ctx)
	fullPath := filepath.Join(fs.chrootDir, path)
	
	// Check if it's a directory for recursive flag
	info, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat path")
	}
	
	// Build ACL entry string based on grantee type
	var aclEntry string
	var identifier string
	
	switch grant.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userId := grant.Grantee.GetUserId()
		if userId == nil {
			return errors.New("cephmount: user grantee without user ID")
		}
		identifier = userId.OpaqueId
		
		// Resolve username to UID for setfacl (OpaqueId is always a username)
		userInfo, err := user.Lookup(identifier)
		if err != nil {
			return errors.Errorf("cephmount: user '%s' does not exist in /etc/passwd. "+
				"All users must be available on the local system. Original error: %v", identifier, err)
		}
		identifier = userInfo.Uid
		log.Debug().
			Str("username", userId.OpaqueId).
			Str("uid", identifier).
			Msg("cephmount: resolved username to UID")
		
		aclEntry = fmt.Sprintf("u:%s:%s", identifier, fs.permissionsToACLString(grant.Permissions))
		
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupId := grant.Grantee.GetGroupId()
		if groupId == nil {
			return errors.New("cephmount: group grantee without group ID")
		}
		identifier = groupId.OpaqueId
		
		// Resolve group name to GID for setfacl (OpaqueId is always a groupname)
		groupInfo, err := user.LookupGroup(identifier)
		if err != nil {
			return errors.Errorf("cephmount: group '%s' does not exist in /etc/group. "+
				"All groups must be available on the local system. Original error: %v", identifier, err)
		}
		identifier = groupInfo.Gid
		log.Debug().
			Str("groupname", groupId.OpaqueId).
			Str("gid", identifier).
			Msg("cephmount: resolved groupname to GID")
		
		aclEntry = fmt.Sprintf("g:%s:%s", identifier, fs.permissionsToACLString(grant.Permissions))
		
	default:
		return errors.New("cephmount: invalid grantee type")
	}
	
	// Build setfacl command
	args := []string{"-m", aclEntry}
	if info.IsDir() {
		// Also set default ACLs for directories
		args = append(args, "-m", "d:"+aclEntry)
		// Recursive flag must come last before the path
		args = append(args, "-R")
	}
	args = append(args, fullPath)
	
	log.Debug().
		Str("path", fullPath).
		Str("acl_entry", aclEntry).
		Strs("args", args).
		Msg("cephmount: executing setfacl")
	
	// Execute setfacl
	cmd := exec.CommandContext(ctx, "setfacl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Err(err).
			Str("path", fullPath).
			Str("output", string(output)).
			Msg("cephmount: setfacl failed")
		return errors.Wrapf(err, "cephmount: setfacl failed: %s", string(output))
	}
	
	log.Debug().
		Str("path", fullPath).
		Msg("cephmount: setfacl succeeded")
	
	return nil
}

// removeGrantViaSetfacl removes a grant using the setfacl system command
func (fs *cephmountfs) removeGrantViaSetfacl(ctx context.Context, path string, grant *provider.Grant) error {
	log := appctx.GetLogger(ctx)
	fullPath := filepath.Join(fs.chrootDir, path)
	
	// Check if it's a directory for recursive flag
	info, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat path")
	}
	
	// Build ACL removal entry string based on grantee type
	var aclEntry string
	var identifier string
	
	switch grant.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userId := grant.Grantee.GetUserId()
		if userId == nil {
			return errors.New("cephmount: user grantee without user ID")
		}
		identifier = userId.OpaqueId
		
		// Resolve username to UID for setfacl (OpaqueId is always a username)
		userInfo, err := user.Lookup(identifier)
		if err != nil {
			return errors.Errorf("cephmount: user '%s' does not exist in /etc/passwd. "+
				"All users must be available on the local system. Original error: %v", identifier, err)
		}
		identifier = userInfo.Uid
		log.Debug().
			Str("username", userId.OpaqueId).
			Str("uid", identifier).
			Msg("cephmount: resolved username to UID for removal")
		
		aclEntry = fmt.Sprintf("u:%s", identifier)
		
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupId := grant.Grantee.GetGroupId()
		if groupId == nil {
			return errors.New("cephmount: group grantee without group ID")
		}
		identifier = groupId.OpaqueId
		
		// Resolve group name to GID for setfacl (OpaqueId is always a groupname)
		groupInfo, err := user.LookupGroup(identifier)
		if err != nil {
			return errors.Errorf("cephmount: group '%s' does not exist in /etc/group. "+
				"All groups must be available on the local system. Original error: %v", identifier, err)
		}
		identifier = groupInfo.Gid
		log.Debug().
			Str("groupname", groupId.OpaqueId).
			Str("gid", identifier).
			Msg("cephmount: resolved groupname to GID for removal")
		
		aclEntry = fmt.Sprintf("g:%s", identifier)
		
	default:
		return errors.New("cephmount: invalid grantee type")
	}
	
	// Build setfacl command with -x to remove
	args := []string{"-x", aclEntry}
	if info.IsDir() {
		// Also remove from default ACLs for directories
		args = append(args, "-x", "d:"+aclEntry)
		// Recursive flag must come last before the path
		args = append(args, "-R")
	}
	args = append(args, fullPath)
	
	log.Debug().
		Str("path", fullPath).
		Str("acl_entry", aclEntry).
		Strs("args", args).
		Msg("cephmount: executing setfacl for removal")
	
	// Execute setfacl
	cmd := exec.CommandContext(ctx, "setfacl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if entry doesn't exist
		if !strings.Contains(string(output), "No such file or directory") {
			log.Error().
				Err(err).
				Str("path", fullPath).
				Str("output", string(output)).
				Msg("cephmount: setfacl removal failed")
			return errors.Wrapf(err, "cephmount: setfacl removal failed: %s", string(output))
		}
	}
	
	log.Debug().
		Str("path", fullPath).
		Msg("cephmount: setfacl removal succeeded")
	
	return nil
}

// aclStringToPermissions converts ACL rwx string to CS3 ResourcePermissions
func (fs *cephmountfs) aclStringToPermissions(aclStr string) *provider.ResourcePermissions {
	perms := &provider.ResourcePermissions{}
	
	if strings.Contains(aclStr, "r") {
		perms.Stat = true
		perms.GetPath = true
		perms.GetQuota = true
		perms.InitiateFileDownload = true
		perms.ListGrants = true
	}
	
	if strings.Contains(aclStr, "w") {
		perms.CreateContainer = true
		perms.Delete = true
		perms.InitiateFileUpload = true
		perms.Move = true
		perms.PurgeRecycle = true
		perms.RestoreFileVersion = true
		perms.RestoreRecycleItem = true
		perms.AddGrant = true
		perms.UpdateGrant = true
		perms.RemoveGrant = true
		perms.DenyGrant = true
	}
	
	if strings.Contains(aclStr, "x") {
		perms.ListRecycle = true
		perms.ListContainer = true
		perms.ListFileVersions = true
	}
	
	return perms
}

// permissionsToACLString converts CS3 ResourcePermissions to ACL rwx string
func (fs *cephmountfs) permissionsToACLString(perms *provider.ResourcePermissions) string {
	var result string
	
	// Read permission
	if perms.Stat || perms.GetPath || perms.GetQuota || perms.ListGrants || perms.InitiateFileDownload {
		result += "r"
	} else {
		result += "-"
	}
	
	// Write permission
	if perms.CreateContainer || perms.Move || perms.Delete || perms.InitiateFileUpload || 
	   perms.AddGrant || perms.UpdateGrant || perms.RemoveGrant || perms.DenyGrant ||
	   perms.RestoreFileVersion || perms.PurgeRecycle || perms.RestoreRecycleItem {
		result += "w"
	} else {
		result += "-"
	}
	
	// Execute permission
	if perms.ListRecycle || perms.ListContainer || perms.ListFileVersions {
		result += "x"
	} else {
		result += "-"
	}
	
	return result
}

// permToInt converts ResourcePermissions to rwx bits
func permToInt(rp *provider.ResourcePermissions) (result uint16) {
	if rp == nil {
		return 0b111 // rwx
	}
	if rp.Stat || rp.GetPath || rp.GetQuota || rp.ListGrants || rp.InitiateFileDownload {
		result |= 4
	}
	if rp.CreateContainer || rp.Move || rp.Delete || rp.InitiateFileUpload || rp.AddGrant || rp.UpdateGrant ||
		rp.RemoveGrant || rp.DenyGrant || rp.RestoreFileVersion || rp.PurgeRecycle || rp.RestoreRecycleItem {
		result |= 2
	}
	if rp.ListRecycle || rp.ListContainer || rp.ListFileVersions {
		result |= 1
	}
	return
}

const (
	updateGrantMode = iota
	removeGrantMode = iota
)

// changePerms modifies POSIX ACLs for a file or directory
func (fs *cephmountfs) changePerms(ctx context.Context, path string, grant *provider.Grant, method int) error {
	log := appctx.GetLogger(ctx)
	
	// Get the full filesystem path
	fullPath := filepath.Join(fs.chrootDir, path)
	
	// Check if it's a directory
	info, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat path")
	}
	isDir := info.IsDir()
	
	// Process the current path
	if err := fs.changePermsForPath(ctx, fullPath, grant, method, isDir); err != nil {
		return err
	}
	
	// If it's a directory, recursively apply to all children
	if isDir {
		err := filepath.Walk(fullPath, func(childPath string, childInfo os.FileInfo, walkErr error) error {
			if walkErr != nil {
				log.Warn().Err(walkErr).Str("path", childPath).Msg("cephmount: skipping path due to walk error")
				return nil // Continue walking
			}
			
			// Skip the root path (already processed)
			if childPath == fullPath {
				return nil
			}
			
			// Apply ACLs to child
			if err := fs.changePermsForPath(ctx, childPath, grant, method, childInfo.IsDir()); err != nil {
				log.Warn().Err(err).Str("path", childPath).Msg("cephmount: failed to set ACL on child path")
				// Continue processing other children
			}
			
			return nil
		})
		
		if err != nil {
			return errors.Wrap(err, "cephmount: failed to walk directory tree")
		}
	}
	
	return nil
}

// changePermsForPath modifies POSIX ACLs for a single file or directory
func (fs *cephmountfs) changePermsForPath(ctx context.Context, fullPath string, grant *provider.Grant, method int, isDir bool) error {
	log := appctx.GetLogger(ctx)
	
	log.Debug().
		Str("path", fullPath).
		Bool("is_dir", isDir).
		Str("grantee_type", grant.Grantee.Type.String()).
		Msg("cephmount: changePermsForPath starting")
	
	// Get file stat to extract owner/group
	stat, err := os.Stat(fullPath)
	if err != nil {
		return errors.Wrap(err, "cephmount: failed to stat file for ACL")
	}
	sysstat := stat.Sys().(*syscall.Stat_t)
	
	log.Debug().
		Str("path", fullPath).
		Uint32("file_uid", sysstat.Uid).
		Uint32("file_gid", sysstat.Gid).
		Str("file_mode", stat.Mode().String()).
		Msg("cephmount: file stat info")
	
	// Read existing access ACL
	buf, err := xattr.Get(fullPath, aclXattrAccess)
	var acls *posix_acl.Acl
	if err != nil {
		log.Debug().Err(err).Str("path", fullPath).Msg("cephmount: failed to get ACL, creating minimal ACL with base entries")
		// If no ACL exists, create one with required base entries
		acls = fs.createMinimalACL(sysstat.Uid, sysstat.Gid, stat.Mode())
		log.Debug().
			Str("path", fullPath).
			Int("acl_entries", len(acls.List)).
			Uint32("acl_version", acls.Version).
			Msg("cephmount: created minimal ACL")
	} else {
		log.Debug().
			Str("path", fullPath).
			Int("existing_acl_size", len(buf)).
			Msg("cephmount: found existing ACL")
		acls = &posix_acl.Acl{}
		acls.Decode(buf)
		log.Debug().
			Str("path", fullPath).
			Int("decoded_entries", len(acls.List)).
			Uint32("acl_version", acls.Version).
			Msg("cephmount: decoded existing ACL")
		// Ensure base entries exist
		acls = fs.ensureBaseACLEntries(acls, sysstat.Uid, sysstat.Gid, stat.Mode())
		log.Debug().
			Str("path", fullPath).
			Int("entries_after_ensure", len(acls.List)).
			Msg("cephmount: ensured base ACL entries")
	}
	
	var sid posix_acl.AclSID
	
	switch grant.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		userId := grant.Grantee.GetUserId()
		if userId == nil {
			return errors.New("cephmount: user grantee without user ID")
		}
		// Try to get user by opaque ID
		userInfo, err := user.Lookup(userId.OpaqueId)
		if err != nil {
			// If lookup fails, try to parse as UID number
			uid, parseErr := strconv.ParseUint(userId.OpaqueId, 10, 32)
			if parseErr != nil {
				return errors.Wrapf(err, "cephmount: failed to lookup user %s", userId.OpaqueId)
			}
			sid.SetUid(uint32(uid))
		} else {
			uid, _ := strconv.ParseUint(userInfo.Uid, 10, 32)
			sid.SetUid(uint32(uid))
		}
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		groupId := grant.Grantee.GetGroupId()
		if groupId == nil {
			return errors.New("cephmount: group grantee without group ID")
		}
		// Try to get group by opaque ID
		groupInfo, err := user.LookupGroup(groupId.OpaqueId)
		if err != nil {
			// If lookup fails, try to parse as GID number
			gid, parseErr := strconv.ParseUint(groupId.OpaqueId, 10, 32)
			if parseErr != nil {
				return errors.Wrapf(err, "cephmount: failed to lookup group %s", groupId.OpaqueId)
			}
			sid.SetGid(uint32(gid))
		} else {
			gid, _ := strconv.ParseUint(groupInfo.Gid, 10, 32)
			sid.SetGid(uint32(gid))
		}
	default:
		return errors.New("cephmount: invalid grantee type")
	}
	
	// Update ACL list
	log.Debug().
		Str("path", fullPath).
		Str("sid", fmt.Sprintf("%064b", sid)).
		Int("method", method).
		Msg("cephmount: about to update ACL list")
	
	aclsList := fs.updateAclList(acls.List, sid, grant.Permissions, method)
	acls.List = aclsList
	
	// Sort ACL entries in the required order for kernel validation
	acls.List = fs.sortACLEntries(acls.List)
	
	log.Debug().
		Str("path", fullPath).
		Int("final_entries", len(acls.List)).
		Msg("cephmount: ACL list updated")
	
	// Log all ACL entries before encoding
	for i, entry := range acls.List {
		log.Debug().
			Str("path", fullPath).
			Int("entry_index", i).
			Str("entry_type", fmt.Sprintf("%d", entry.GetType())).
			Uint32("entry_id", entry.GetID()).
			Uint16("entry_perm", entry.Perm).
			Str("entry_perm_rwx", fmt.Sprintf("%c%c%c",
				func() rune { if entry.Perm&4 != 0 { return 'r' }; return '-' }(),
				func() rune { if entry.Perm&2 != 0 { return 'w' }; return '-' }(),
				func() rune { if entry.Perm&1 != 0 { return 'x' }; return '-' }())).
			Msg("cephmount: ACL entry details")
	}
	
	// Encode and log the raw bytes
	encoded := acls.Encode()
	log.Debug().
		Str("path", fullPath).
		Int("encoded_size", len(encoded)).
		Str("encoded_hex", fmt.Sprintf("%x", encoded)).
		Msg("cephmount: encoded ACL")
	
	// Write back the access ACL
	err = xattr.Set(fullPath, aclXattrAccess, encoded)
	if err != nil {
		log.Error().
			Err(err).
			Str("path", fullPath).
			Int("encoded_size", len(encoded)).
			Str("encoded_hex", fmt.Sprintf("%x", encoded)).
			Msg("cephmount: FAILED to set access ACL xattr")
		return errors.Wrap(err, "cephmount: failed to set access ACL xattr")
	}
	
	log.Debug().
		Str("path", fullPath).
		Msg("cephmount: successfully set access ACL xattr")
	
	// For directories, also set default ACL
	if isDir {
		// Read existing default ACL or create new one
		defaultBuf, err := xattr.Get(fullPath, aclXattrDefault)
		if err != nil {
			// No default ACL exists, create one based on access ACL
			defaultAcls := &posix_acl.Acl{Version: 2, List: []posix_acl.AclElement{}}
			defaultBuf = defaultAcls.Encode()
		}
		
		defaultAcls := &posix_acl.Acl{}
		defaultAcls.Decode(defaultBuf)
		
		// Update default ACL with same changes
		defaultAcls.List = fs.updateAclList(defaultAcls.List, sid, grant.Permissions, method)
		
		// Write back the default ACL
		err = xattr.Set(fullPath, aclXattrDefault, defaultAcls.Encode())
		if err != nil {
			return errors.Wrap(err, "cephmount: failed to set default ACL xattr")
		}
	}
	
	return nil
}

// createMinimalACL creates a minimal valid ACL with required base entries
func (fs *cephmountfs) createMinimalACL(uid, gid uint32, mode os.FileMode) *posix_acl.Acl {
	perm := uint16(mode.Perm())
	
	var ownerSID, groupSID, otherSID posix_acl.AclSID
	
	// For base entries (USER_OWNER, GROUP_OWNER, OTHERS), only SetType should be called
	// SetUid/SetGid set the type to ACL_USER/ACL_GROUP which are extended entries
	ownerSID.SetType(posix_acl.ACL_USER_OWNER)
	groupSID.SetType(posix_acl.ACL_GROUP_OWNER)
	otherSID.SetType(posix_acl.ACL_OTHERS)
	
	acl := &posix_acl.Acl{
		Version: 2,
		List: []posix_acl.AclElement{
			{AclSID: ownerSID, Perm: (perm >> 6) & 0b111},  // owner rwx
			{AclSID: groupSID, Perm: (perm >> 3) & 0b111},  // group rwx
			{AclSID: otherSID, Perm: perm & 0b111},         // other rwx
		},
	}
	
	// Verify the types are correct
	for i, entry := range acl.List {
		t := entry.GetType()
		appctx.GetLogger(context.Background()).Debug().
			Int("index", i).
			Str("type", fmt.Sprintf("%d", t)).
			Uint32("id", entry.GetID()).
			Uint16("perm", entry.Perm).
			Str("sid_hex", fmt.Sprintf("%064b", entry.AclSID)).
			Msg("cephmount: createMinimalACL entry")
	}
	
	return acl
}

// ensureBaseACLEntries ensures an ACL has the required base entries (owner, group, others)
func (fs *cephmountfs) ensureBaseACLEntries(acls *posix_acl.Acl, uid, gid uint32, mode os.FileMode) *posix_acl.Acl {
	if acls == nil {
		return fs.createMinimalACL(uid, gid, mode)
	}
	
	// First, fix any corrupted base entries (those with invalid IDs like 0xFFFFFFFF)
	// Base entries should have ID=0 after proper encoding
	for i := range acls.List {
		t := acls.List[i].GetType()
		id := acls.List[i].GetID()
		
		// Base entry types should not have IDs set
		if (t == posix_acl.ACL_USER_OWNER || t == posix_acl.ACL_GROUP_OWNER || 
		    t == posix_acl.ACL_OTHERS || t == posix_acl.ACL_MASK) && id != 0 {
			// Recreate the SID with correct type only
			var newSID posix_acl.AclSID
			newSID.SetType(t)
			acls.List[i].AclSID = newSID
		}
	}
	
	// Check if we have all required base entries
	hasOwner := false
	hasGroup := false
	hasOther := false
	hasMask := false
	
	for _, entry := range acls.List {
		switch entry.GetType() {
		case posix_acl.ACL_USER_OWNER:
			hasOwner = true
		case posix_acl.ACL_GROUP_OWNER:
			hasGroup = true
		case posix_acl.ACL_OTHERS:
			hasOther = true
		case posix_acl.ACL_MASK:
			hasMask = true
		}
	}
	
	perm := uint16(mode.Perm())
	
	// Add missing base entries
	// For base entries, only SetType should be called (no SetUid/SetGid)
	if !hasOwner {
		var ownerSID posix_acl.AclSID
		ownerSID.SetType(posix_acl.ACL_USER_OWNER)
		acls.List = append([]posix_acl.AclElement{
			{AclSID: ownerSID, Perm: (perm >> 6) & 0b111},
		}, acls.List...)
	}
	
	if !hasGroup {
		var groupSID posix_acl.AclSID
		groupSID.SetType(posix_acl.ACL_GROUP_OWNER)
		acls.List = append(acls.List, posix_acl.AclElement{
			AclSID: groupSID, Perm: (perm >> 3) & 0b111,
		})
	}
	
	if !hasOther {
		var otherSID posix_acl.AclSID
		otherSID.SetType(posix_acl.ACL_OTHERS)
		acls.List = append(acls.List, posix_acl.AclElement{
			AclSID: otherSID, Perm: perm & 0b111,
		})
	}
	
	// If we have extended ACL entries (user or group), ensure we have a mask
	hasExtended := false
	for _, entry := range acls.List {
		t := entry.GetType()
		if t == posix_acl.ACL_USER || t == posix_acl.ACL_GROUP {
			hasExtended = true
			break
		}
	}
	
	if hasExtended && !hasMask {
		var maskSID posix_acl.AclSID
		maskSID.SetType(posix_acl.ACL_MASK)
		acls.List = append(acls.List, posix_acl.AclElement{
			AclSID: maskSID, Perm: 0b111, // rwx by default
		})
	}
	
	return acls
}

// updateAclList updates an ACL list with the given SID and permissions
func (fs *cephmountfs) updateAclList(list []posix_acl.AclElement, sid posix_acl.AclSID, perms *provider.ResourcePermissions, method int) []posix_acl.AclElement {
	// Find existing ACL entry
	var found = false
	var i int
	for i = range list {
		if list[i].AclSID == sid {
			found = true
			break
		}
	}
	
	if method == updateGrantMode {
		if found {
			list[i].Perm |= permToInt(perms)
			if list[i].Perm == 0 { // remove empty grant
				list = append(list[:i], list[i+1:]...)
			}
		} else {
			list = append(list, posix_acl.AclElement{
				AclSID: sid,
				Perm:   permToInt(perms),
			})
		}
	} else { // removeGrantMode
		if found {
			list[i].Perm &^= permToInt(perms) // bitwise and-not, to clear bits on Perm
			if list[i].Perm == 0 {             // remove empty grant
				list = append(list[:i], list[i+1:]...)
			}
		}
	}
	
	// Ensure mask entry exists if we have extended ACL entries
	list = fs.ensureMaskEntry(list)
	
	return list
}

// sortACLEntries sorts ACL entries in the kernel-required order
func (fs *cephmountfs) sortACLEntries(list []posix_acl.AclElement) []posix_acl.AclElement {
	// POSIX ACL entries must be in this specific order:
	// 1. ACL_USER_OWNER (type 1)
	// 2. ACL_USER entries (type 2) - sorted by UID ascending
	// 3. ACL_GROUP_OWNER (type 4)
	// 4. ACL_GROUP entries (type 8) - sorted by GID ascending
	// 5. ACL_MASK (type 16)
	// 6. ACL_OTHERS (type 32)
	
	var sorted []posix_acl.AclElement
	
	// 1. Add USER_OWNER
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_USER_OWNER {
			sorted = append(sorted, e)
			break
		}
	}
	
	// 2. Add USER entries (sorted by ID)
	var users []posix_acl.AclElement
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_USER {
			users = append(users, e)
		}
	}
	// Sort users by ID
	for i := 0; i < len(users); i++ {
		for j := i + 1; j < len(users); j++ {
			if users[i].GetID() > users[j].GetID() {
				users[i], users[j] = users[j], users[i]
			}
		}
	}
	sorted = append(sorted, users...)
	
	// 3. Add GROUP_OWNER
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_GROUP_OWNER {
			sorted = append(sorted, e)
			break
		}
	}
	
	// 4. Add GROUP entries (sorted by ID)
	var groups []posix_acl.AclElement
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_GROUP {
			groups = append(groups, e)
		}
	}
	// Sort groups by ID
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			if groups[i].GetID() > groups[j].GetID() {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}
	sorted = append(sorted, groups...)
	
	// 5. Add MASK
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_MASK {
			sorted = append(sorted, e)
			break
		}
	}
	
	// 6. Add OTHERS
	for _, e := range list {
		if e.GetType() == posix_acl.ACL_OTHERS {
			sorted = append(sorted, e)
			break
		}
	}
	
	return sorted
}

// ensureMaskEntry ensures a mask entry exists when there are extended ACL entries
func (fs *cephmountfs) ensureMaskEntry(list []posix_acl.AclElement) []posix_acl.AclElement {
	hasMask := false
	hasExtended := false
	
	for _, entry := range list {
		t := entry.GetType()
		if t == posix_acl.ACL_MASK {
			hasMask = true
		}
		if t == posix_acl.ACL_USER || t == posix_acl.ACL_GROUP {
			hasExtended = true
		}
	}
	
	// Add mask if we have extended entries but no mask
	if hasExtended && !hasMask {
		var maskSID posix_acl.AclSID
		maskSID.SetType(posix_acl.ACL_MASK)
		list = append(list, posix_acl.AclElement{
			AclSID: maskSID,
			Perm:   0b111, // rwx by default
		})
	}
	
	return list
}

// Helper function from the original ceph implementation
func sameHolder(l1, l2 *provider.Lock) bool {
	same := true
	if l1.User != nil || l2.User != nil {
		same = utils.UserEqual(l1.User, l2.User)
	}
	if l1.AppName != "" || l2.AppName != "" {
		same = l1.AppName == l2.AppName
	}
	return same
}
