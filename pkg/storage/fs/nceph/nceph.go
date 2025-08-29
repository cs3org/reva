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

// Package nceph provides a local filesystem implementation that mimics ceph operations
package nceph

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/mime"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/fs/registry"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	xattrUserNs = "user."
	xattrLock   = xattrUserNs + "reva.lockpayload"
)

// ncephfs is a local filesystem implementation that provides a ceph-like interface
type ncephfs struct {
	conf          *Options
	cephAdminConn *CephAdminConn  // Only used for GetPathByID (defined in build-tag files)
	rootFS        *os.Root        // Chrooted filesystem root using os.Root
	threadPool    *UserThreadPool // Pool of per-user threads with dedicated UIDs
}

func init() {
	registry.Register("nceph", New)
}

// New returns an implementation of the storage.FS interface that talks to
// the local filesystem using os.Root operations instead of libcephfs.
func New(ctx context.Context, m map[string]interface{}) (fs storage.FS, err error) {
	var o Options
	if err := cfg.Decode(m, &o); err != nil {
		return nil, err
	}

	// Apply defaults
	o.ApplyDefaults()

	// Ensure root directory exists and get absolute path
	absRoot, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, errors.Wrap(err, "nceph: failed to get absolute path for root")
	}

	// Create a chrooted filesystem using os.OpenRoot to jail all operations to the root
	rootFS, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, errors.Wrap(err, "nceph: failed to create chroot jail with os.OpenRoot")
	}

	// Initialize ceph admin connection if ceph config is provided
	var cephAdminConn *CephAdminConn
	if o.CephConfig != "" && o.CephClientID != "" && o.CephKeyring != "" {
		cephAdminConn, err = newCephAdminConn(ctx, &o)
		if err != nil {
			// Log warning but continue - GetPathByID will not work but other operations will
			log := appctx.GetLogger(ctx)
			log.Warn().Err(err).Msg("nceph: failed to create ceph admin connection, GetPathByID will not work")
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
		return nil, errors.Wrap(err, "nceph: failed to initialize user thread pool")
	}

	// Log privilege verification results
	log := appctx.GetLogger(ctx)
	
	// Always log basic privilege status first
	log.Info().
		Int("current_uid", privResult.CurrentUID).
		Int("current_gid", privResult.CurrentGID).
		Int("current_fsuid", privResult.CurrentFsUID).
		Int("current_fsgid", privResult.CurrentFsGID).
		Bool("can_change_uid", privResult.CanChangeUID).
		Bool("can_change_gid", privResult.CanChangeGID).
		Msg("nceph: privilege verification status")

	// Log detailed test information
	log.Info().
		Interface("tested_uids", privResult.TestedUIDs).
		Interface("tested_gids", privResult.TestedGIDs).
		Int("target_nobody_uid", o.NobodyUID).
		Int("target_nobody_gid", o.NobodyGID).
		Msg("nceph: privilege verification details")

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
		Msg("nceph: privilege verification restoration status")
	
	if finalFsUID != privResult.CurrentFsUID {
		log.Error().
			Int("expected_fsuid", privResult.CurrentFsUID).
			Int("actual_fsuid", finalFsUID).
			Msg("nceph: CRITICAL - privilege verification failed to restore original fsuid - this will cause permission issues")
	}
	
	if finalFsGID != privResult.CurrentFsGID {
		log.Error().
			Int("expected_fsgid", privResult.CurrentFsGID).
			Int("actual_fsgid", finalFsGID).
			Msg("nceph: CRITICAL - privilege verification failed to restore original fsgid - this will cause permission issues")
	}

	if !privResult.HasSufficientPrivileges() {
		if privResult.HasPartialPrivileges() {
			log.Warn().
				Bool("can_change_uid", privResult.CanChangeUID).
				Bool("can_change_gid", privResult.CanChangeGID).
				Interface("error_messages", privResult.ErrorMessages).
				Interface("recommendations", privResult.Recommendations).
				Str("impact", "some per-user operations may not work correctly").
				Msg("nceph: partial privileges detected")
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
				Msg("nceph: insufficient privileges for setfsuid/setfsgid")
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
			Msg("nceph: sufficient privileges verified for per-user thread isolation")
	}

	return &ncephfs{
		conf:          &o,
		cephAdminConn: cephAdminConn,
		rootFS:        rootFS,
		threadPool:    threadPool,
	}, nil
}

// resolveRef converts a provider.Reference to a chroot-relative path
func (fs *ncephfs) resolveRef(ctx context.Context, ref *provider.Reference) (string, error) {
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
		return "", errors.New("nceph: invalid resource id")
	default:
		return "", errors.New("nceph: invalid reference")
	}
}

// fileAsResourceInfo converts file info to ResourceInfo without user context
func (fs *ncephfs) fileAsResourceInfo(path string, info os.FileInfo, mdKeys []string) (*provider.ResourceInfo, error) {
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
			absPath = fs.conf.Root
		} else {
			absPath = filepath.Join(fs.conf.Root, path)
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
		StorageId: "nceph",
		OpaqueId:  strconv.FormatUint(stat.Ino, 10),
	}

	ri := &provider.ResourceInfo{
		Type:     resourceType,
		Id:       resourceId,
		Checksum: &provider.ResourceChecksum{},
		Size:     size,
		Mtime:    &typepb.Timestamp{Seconds: uint64(info.ModTime().Unix())},
		Path:     fs.fromChroot(path),                   // Convert chroot path back to external path
		Owner:    &userv1beta1.UserId{OpaqueId: "root"}, // Default owner
		PermissionSet: &provider.ResourcePermissions{
			AddGrant:             true,
			CreateContainer:      true,
			Delete:               true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
			ListContainer:        true,
			ListFileVersions:     false,
			ListGrants:           true,
			ListRecycle:          false,
			Move:                 true,
			PurgeRecycle:         false,
			RemoveGrant:          true,
			RestoreFileVersion:   false,
			RestoreRecycleItem:   false,
			Stat:                 true,
			UpdateGrant:          true,
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
func (fs *ncephfs) toChroot(externalPath string) string {
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
func (fs *ncephfs) fromChroot(chrootPath string) string {
	if chrootPath == "." {
		return "/"
	}
	// Ensure the returned path starts with /
	if strings.HasPrefix(chrootPath, "/") {
		return chrootPath
	}
	return "/" + chrootPath
}

func (fs *ncephfs) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("nceph: GetHome not implemented")
}

func (fs *ncephfs) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("nceph: CreateHome not implemented")
}

func (fs *ncephfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "CreateDir", path)

	// Execute directory creation on user's thread with correct UID
	err = fs.createDirectoryAsUser(ctx, path, os.FileMode(fs.conf.DirPerms))
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to create directory")
		fs.logOperationError(ctx, "CreateDir", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *ncephfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "Delete", path)

	// Execute stat and delete operations on user's thread with correct UID
	info, err := fs.statAsUser(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		wrappedErr := errors.Wrap(err, "nceph: failed to stat file for deletion")
		fs.logOperationError(ctx, "Delete", path, wrappedErr)
		return wrappedErr
	}

	if info.IsDir() {
		err = fs.removeAllAsUser(ctx, path)
	} else {
		err = fs.removeAsUser(ctx, path)
	}

	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to delete")
		fs.logOperationError(ctx, "Delete", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *ncephfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	oldPath, err := fs.resolveRef(ctx, oldRef)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve old path")
		fs.logOperationError(ctx, "Move", "", wrappedErr)
		return wrappedErr
	}
	newPath, err := fs.resolveRef(ctx, newRef)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve new path")
		fs.logOperationError(ctx, "Move", oldPath, wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "Move", fmt.Sprintf("%s -> %s", oldPath, newPath))

	// oldPath and newPath are already chroot-relative from resolveRef
	// Create parent directory if needed and execute move on user's thread with correct UID
	parentPath := path.Dir(newPath)
	if parentPath != "." {
		if err := fs.createDirectoryAsUser(ctx, parentPath, os.FileMode(fs.conf.DirPerms)); err != nil {
			wrappedErr := errors.Wrap(err, "nceph: failed to create parent directory for move")
			fs.logOperationError(ctx, "Move", fmt.Sprintf("%s -> %s", oldPath, newPath), wrappedErr)
			return wrappedErr
		}
	}

	err = fs.renameAsUser(ctx, oldPath, newPath)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to move file")
		fs.logOperationError(ctx, "Move", fmt.Sprintf("%s -> %s", oldPath, newPath), wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *ncephfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	if ref == nil {
		wrappedErr := errors.New("error: ref is nil")
		fs.logOperationError(ctx, "GetMD", "", wrappedErr)
		return nil, wrappedErr
	}

	log := appctx.GetLogger(ctx)
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "GetMD", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "GetMD", path)

	// path is already chroot-relative from resolveRef
	// Execute stat operation on user's thread with correct UID
	info, err := fs.statAsUser(ctx, path)
	if err != nil {
		if os.IsNotExist(err) {
			wrappedErr := errtypes.NotFound("file not found")
			fs.logOperationError(ctx, "GetMD", path, wrappedErr)
			return nil, wrappedErr
		}
		wrappedErr := errors.Wrap(err, "nceph: failed to stat file")
		fs.logOperationError(ctx, "GetMD", path, wrappedErr)
		return nil, wrappedErr
	}

	ri, err = fs.fileAsResourceInfo(path, info, mdKeys)
	if err != nil {
		log.Debug().Any("resourceInfo", ri).Err(err).Msg("fileAsResourceInfo returned error")
		wrappedErr := errors.Wrap(err, "nceph: failed to convert file to resource info")
		fs.logOperationError(ctx, "GetMD", path, wrappedErr)
		return nil, wrappedErr
	}

	return ri, nil
}

func (fs *ncephfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (files []*provider.ResourceInfo, err error) {
	if ref == nil {
		wrappedErr := errors.New("error: ref is nil")
		fs.logOperationError(ctx, "ListFolder", "", wrappedErr)
		return nil, wrappedErr
	}

	log := appctx.GetLogger(ctx)
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "ListFolder", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "ListFolder", path)

	// Execute directory listing on user's thread with correct UID
	entries, err := fs.readDirectoryAsUser(ctx, path)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to read directory")
		fs.logOperationError(ctx, "ListFolder", path, wrappedErr)
		return nil, wrappedErr
	}

	for _, entry := range entries {
		if fs.conf.HiddenDirs[entry.Name()] {
			continue
		}

		ri, err := fs.fileAsResourceInfo(filepath.Join(path, entry.Name()), entry, mdKeys)
		if ri == nil || err != nil {
			if err != nil {
				log.Debug().Any("resourceInfo", ri).Err(err).Msg("fileAsResourceInfo returned error")
			}
			continue
		}

		files = append(files, ri)
	}

	return files, nil
}

func (fs *ncephfs) Download(ctx context.Context, ref *provider.Reference) (rc io.ReadCloser, err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: error resolving ref")
		fs.logOperationError(ctx, "Download", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "Download", path)

	// Execute file open on user's thread with correct UID
	file, err := fs.openFileAsUser(ctx, path)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to open file for download")
		fs.logOperationError(ctx, "Download", path, wrappedErr)
		return nil, wrappedErr
	}

	return file, nil
}

// Upload handles file uploads to the local filesystem
func (fs *ncephfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser, metadata map[string]string) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: error resolving reference")
		fs.logOperationError(ctx, "Upload", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "Upload", path)

	// Create parent directory if needed and execute upload on user's thread with correct UID
	parentDir := filepath.Dir(path)
	if parentDir != "." {
		if err := fs.createDirectoryAsUser(ctx, parentDir, os.FileMode(fs.conf.DirPerms)); err != nil {
			wrappedErr := errors.Wrap(err, "nceph: failed to create parent directory for upload")
			fs.logOperationError(ctx, "Upload", path, wrappedErr)
			return wrappedErr
		}
	}

	// Create and upload the file on user's thread
	err = fs.uploadFileAsUser(ctx, path, r, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: error uploading file")
		fs.logOperationError(ctx, "Upload", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *ncephfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: error resolving reference")
		fs.logOperationError(ctx, "InitiateUpload", "", wrappedErr)
		return nil, wrappedErr
	}

	fs.logOperation(ctx, "InitiateUpload", fmt.Sprintf("%s (length: %d)", path, uploadLength))

	return map[string]string{
		"simple": path,
	}, nil
}

func (fs *ncephfs) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	wrappedErr := errtypes.NotSupported("nceph: ListRevisions not supported")
	fs.logOperationError(ctx, "ListRevisions", "", wrappedErr)
	return nil, wrappedErr
}

func (fs *ncephfs) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	wrappedErr := errtypes.NotSupported("nceph: DownloadRevision not supported")
	fs.logOperationError(ctx, "DownloadRevision", "", wrappedErr)
	return nil, wrappedErr
}

func (fs *ncephfs) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	wrappedErr := errtypes.NotSupported("nceph: RestoreRevision not supported")
	fs.logOperationError(ctx, "RestoreRevision", "", wrappedErr)
	return wrappedErr
}

func (fs *ncephfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "AddGrant", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "AddGrant", path)

	// Store grant information as extended attributes using user's thread
	grantData, err := json.Marshal(g)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to marshal grant")
		fs.logOperationError(ctx, "AddGrant", path, wrappedErr)
		return wrappedErr
	}

	grantKey := fmt.Sprintf("user.grant.%s", g.Grantee.GetUserId().GetOpaqueId())
	err = fs.setXattrAsUser(ctx, path, grantKey, grantData)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to set grant xattr")
		fs.logOperationError(ctx, "AddGrant", path, wrappedErr)
		return wrappedErr
	}

	return nil
}

func (fs *ncephfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "RemoveGrant", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "RemoveGrant", path)

	grantKey := fmt.Sprintf("user.grant.%s", g.Grantee.GetUserId().GetOpaqueId())
	err = fs.removeXattrAsUser(ctx, path, grantKey)
	if err != nil {
		// Ignore if the attribute doesn't exist
		if !strings.Contains(err.Error(), "no such attribute") {
			wrappedErr := errors.Wrap(err, "nceph: failed to remove grant xattr")
			fs.logOperationError(ctx, "RemoveGrant", path, wrappedErr)
			return wrappedErr
		}
	}

	return nil
}

func (fs *ncephfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	// For simplicity, update is the same as add
	return fs.AddGrant(ctx, ref, g)
}

func (fs *ncephfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "DenyGrant", path)

	grantKey := fmt.Sprintf("user.grant.%s", g.GetUserId().GetOpaqueId())
	err = fs.removeXattrAsUser(ctx, path, grantKey)
	if err != nil {
		// Ignore if the attribute doesn't exist
		if !strings.Contains(err.Error(), "no such attribute") {
			return errors.Wrap(err, "nceph: failed to deny grant")
		}
	}

	return nil
}

func (fs *ncephfs) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	fs.logOperation(ctx, "ListGrants", path)

	// List all grant-related extended attributes on user's thread
	attrs, err := fs.listXattrsAsUser(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "nceph: failed to list xattrs")
	}

	for _, attr := range attrs {
		if strings.HasPrefix(attr, "user.grant.") {
			data, err := fs.getXattrAsUser(ctx, path, attr)
			if err != nil {
				continue
			}

			var grant provider.Grant
			if err := json.Unmarshal(data, &grant); err != nil {
				continue
			}

			glist = append(glist, &grant)
		}
	}

	return glist, nil
}

func (fs *ncephfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, used uint64, err error) {
	log := appctx.GetLogger(ctx)

	// Get user home path for quota check
	homePath, err := fs.resolveRef(ctx, &provider.Reference{Path: "."})
	if err != nil {
		return 0, 0, errors.Wrap(err, "nceph: error resolving home path")
	}

	// Get quota from extended attributes or use default
	quotaData, err := xattr.Get(homePath, "user.quota.max_bytes")
	if err != nil {
		log.Debug().Msg("nceph: user quota bytes not set, using default")
		total = fs.conf.UserQuotaBytes
	} else {
		total, _ = strconv.ParseUint(string(quotaData), 10, 64)
	}

	// Calculate used space by walking the directory
	used, err = fs.calculateDirectorySize(homePath)
	if err != nil {
		log.Debug().Err(err).Msg("failed to calculate directory size")
		used = 0
	}

	return total, used, nil
}

func (fs *ncephfs) calculateDirectorySize(root string) (uint64, error) {
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

func (fs *ncephfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	return errors.New("error: CreateReference not implemented")
}

func (fs *ncephfs) Shutdown(ctx context.Context) (err error) {
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

func (fs *ncephfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "SetArbitraryMetadata", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "SetArbitraryMetadata", path)

	for k, v := range md.Metadata {
		if !strings.HasPrefix(k, xattrUserNs) {
			k = xattrUserNs + k
		}
		if err := xattr.Set(path, k, []byte(v)); err != nil {
			wrappedErr := errors.Wrap(err, "nceph: failed to set xattr")
			fs.logOperationError(ctx, "SetArbitraryMetadata", path, wrappedErr)
			return wrappedErr
		}
	}

	return nil
}

func (fs *ncephfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		wrappedErr := errors.Wrap(err, "nceph: failed to resolve reference")
		fs.logOperationError(ctx, "UnsetArbitraryMetadata", "", wrappedErr)
		return wrappedErr
	}

	fs.logOperation(ctx, "UnsetArbitraryMetadata", path)

	for _, key := range keys {
		if !strings.HasPrefix(key, xattrUserNs) {
			key = xattrUserNs + key
		}
		if err := xattr.Remove(path, key); err != nil {
			// Ignore if the attribute doesn't exist
			if !strings.Contains(err.Error(), "no such attribute") {
				wrappedErr := errors.Wrap(err, "nceph: failed to remove xattr")
				fs.logOperationError(ctx, "UnsetArbitraryMetadata", path, wrappedErr)
				return wrappedErr
			}
		}
	}

	return nil
}

func (fs *ncephfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "TouchFile", path)

	// Create parent directory if needed using chrooted operations
	parentDir := filepath.Dir(path)
	if parentDir != "." {
		if err := fs.rootFS.MkdirAll(parentDir, os.FileMode(fs.conf.DirPerms)); err != nil {
			return errors.Wrap(err, "nceph: failed to create parent directory")
		}
	}

	// Use chrooted file operations
	file, err := fs.rootFS.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		return errors.Wrap(err, "nceph: failed to touch file")
	}
	defer file.Close()

	return nil
}

func (fs *ncephfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (r *provider.CreateStorageSpaceResponse, err error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *ncephfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
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

func (fs *ncephfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return err
	}

	fs.logOperation(ctx, "SetLock", path)

	// Open the file for locking
	file, err := os.OpenFile(path, os.O_RDWR, os.FileMode(fs.conf.FilePerms))
	if err != nil {
		return errors.Wrap(err, "nceph: failed to open file for locking")
	}
	defer file.Close()

	// Try to acquire a file lock
	lockType := syscall.LOCK_EX
	if lock.Type == provider.LockType_LOCK_TYPE_SHARED {
		lockType = syscall.LOCK_SH
	}

	if err := syscall.Flock(int(file.Fd()), lockType|syscall.LOCK_NB); err != nil {
		return errors.Wrap(err, "nceph: failed to acquire file lock")
	}

	// Store lock metadata as extended attribute
	md := &provider.ArbitraryMetadata{
		Metadata: map[string]string{
			xattrLock: encodeLock(lock),
		},
	}
	return fs.SetArbitraryMetadata(ctx, ref, md)
}

func (fs *ncephfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	path, err := fs.resolveRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	fs.logOperation(ctx, "GetLock", path)

	// Try to read lock metadata
	buf, err := xattr.Get(path, xattrLock)
	if err != nil {
		if strings.Contains(err.Error(), "no such attribute") {
			return nil, errtypes.NotFound("file was not locked")
		}
		return nil, errors.Wrap(err, "nceph: failed to get lock xattr")
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

func (fs *ncephfs) RefreshLock(ctx context.Context, ref *provider.Reference, newLock *provider.Lock, existingLockID string) error {
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

func (fs *ncephfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
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
		return errors.Wrap(err, "nceph: failed to open file for unlocking")
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		return errors.Wrap(err, "nceph: failed to release file lock")
	}

	// Remove lock metadata
	return fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
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
