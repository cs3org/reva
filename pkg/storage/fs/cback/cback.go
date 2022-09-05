// Copyright 2018-2021 CERN
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

package cback

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bluele/gcache"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/cback"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type cbackfs struct {
	conf   *Config
	client *cback.Client
	cache  gcache.Cache
}

func init() {
	registry.Register("cback", New)
}

// New returns an implementation to the storage.FS interface that expose
// the snapshots in cback
func New(m map[string]interface{}) (storage.FS, error) {
	c := &Config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, errors.Wrap(err, "cback: error decoding config")
	}

	client := cback.New(
		&cback.Config{
			URL:      c.APIURL,
			Token:    c.Token,
			Insecure: c.Insecure,
			Timeout:  c.Timeout,
		},
	)

	return &cbackfs{
		conf:   c,
		client: client,
		cache:  gcache.New(c.Size).LRU().Build(),
	}, nil
}

func split(path string, backups []*cback.Backup) (string, string, string, int, bool) {
	for _, b := range backups {
		if strings.HasPrefix(path, b.Source) {
			// the path could be in this form:
			// <b.Source>/<snap_id>/<path>
			// snap_id and path are optional
			rel, _ := filepath.Rel(b.Source, path)
			if rel == "." {
				// both snap_id and path were not provided
				return b.Source, "", "", b.ID, true
			}
			split := strings.SplitN(rel, "/", 2)

			var snap, p string
			snap = split[0]
			if len(split) == 2 {
				p = split[1]
			}
			return b.Source, snap, p, b.ID, true
		}
	}
	return "", "", "", 0, false
}

func (f *cbackfs) convertToResourceInfo(r *cback.Resource, path string, resID *provider.ResourceId, owner *user.UserId) *provider.ResourceInfo {
	rtype := provider.ResourceType_RESOURCE_TYPE_FILE
	perms := permFile
	if r.IsDir() {
		rtype = provider.ResourceType_RESOURCE_TYPE_CONTAINER
		perms = permDir
	}

	return &provider.ResourceInfo{
		Type: rtype,
		Id:   resID,
		Checksum: &provider.ResourceChecksum{
			Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
		},
		Etag:     strconv.FormatUint(uint64(r.CTime), 10),
		MimeType: mime.Detect(r.IsDir(), path),
		Mtime: &types.Timestamp{
			Seconds: uint64(r.CTime),
		},
		Path:          path,
		PermissionSet: perms,
		Size:          r.Size,
		Owner:         owner,
	}
}

func encodeBackupInResourceID(backupID int, snapshotID, path string) *provider.ResourceId {
	opaque := fmt.Sprintf("%d#%s#%s", backupID, snapshotID, path)
	return &provider.ResourceId{
		StorageId: "cback",
		OpaqueId:  opaque,
	}
}

func decodeResourceID(r *provider.ResourceId) (int, string, string, bool) {
	if r == nil {
		return 0, "", "", false
	}
	split := strings.SplitN(r.OpaqueId, "#", 3)
	if len(split) != 3 {
		return 0, "", "", false
	}
	backupID, err := strconv.ParseInt(split[0], 10, 64)
	if err != nil {
		return 0, "", "", false
	}
	return int(backupID), split[1], split[2], true
}

func (f *cbackfs) placeholderResourceInfo(path string, owner *user.UserId) *provider.ResourceInfo {
	return &provider.ResourceInfo{
		Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Id: &provider.ResourceId{
			StorageId: "cback",
			OpaqueId:  path,
		},
		Checksum: &provider.ResourceChecksum{
			Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET,
		},
		Etag:     "",
		MimeType: mime.Detect(true, path),
		Mtime: &types.Timestamp{
			Seconds: 0,
		},
		Path:          path,
		PermissionSet: permDir,
		Size:          0,
		Owner:         owner,
	}
}

func hasPrefix(lst, prefix []string) bool {
	for i, p := range prefix {
		if lst[i] != p {
			return false
		}
	}
	return true
}

func (f *cbackfs) isParentOfBackup(path string, backups []*cback.Backup) bool {
	pathSplit := []string{""}
	if path != "/" {
		pathSplit = strings.Split(path, "/")
	}
	for _, b := range backups {
		backupSplit := strings.Split(b.Source, "/")
		if hasPrefix(backupSplit, pathSplit) {
			return true
		}
	}
	return false
}

func (f *cbackfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("cback: user not found in context")
	}

	backups, err := f.listBackups(ctx, user.Username)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error listing backups")
	}

	source, snapshot, path, id, ok := split(ref.Path, backups)
	if ok {
		if snapshot != "" && path != "" {
			// the path from the user is something like /eos/home-g/gdelmont/<snapshot_id>/rest/of/path
			// in this case the method has to return the stat of the file /eos/home-g/gdelmont/rest/of/path
			// in the snapshot <snapshot_id>
			res, err := f.client.Stat(ctx, user.Username, id, snapshot, filepath.Join(source, path))
			if err != nil {
				return nil, err
			}
			return f.convertToResourceInfo(
				res,
				filepath.Join(source, snapshot, path),
				encodeBackupInResourceID(id, snapshot, filepath.Join(source, path)),
				user.Id,
			), nil
		} else if snapshot != "" && path == "" {
			// the path from the user is something like /eos/home-g/gdelmont/<snapshot_id>
			return f.placeholderResourceInfo(filepath.Join(source, snapshot), user.Id), nil
		}
		// the path from the user is something like /eos/home-g/gdelmont
		return f.placeholderResourceInfo(source, user.Id), nil
	}

	// the path is not one of the backup. There is a situation in which
	// the user's path is a parent folder of some of the backups

	if f.isParentOfBackup(ref.Path, backups) {
		return f.placeholderResourceInfo(ref.Path, user.Id), nil
	}

	return nil, errtypes.NotFound(fmt.Sprintf("path %s does not exist", ref.Path))
}

func (f *cbackfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("cback: user not found in context")
	}

	backups, err := f.listBackups(ctx, user.Username)
	if err != nil {
		return nil, errors.Wrapf(err, "cback: error listing backups")
	}

	source, snapshot, path, id, ok := split(ref.Path, backups)
	if ok {
		if snapshot != "" {
			// the path from the user is something like /eos/home-g/gdelmont/<snapshot_id>/(rest/of/path)
			// in this case the method has to return the content of the folder /eos/home-g/gdelmont/(rest/of/path)
			// in the snapshot <snapshot_id>
			content, err := f.client.ListFolder(ctx, user.Username, id, snapshot, filepath.Join(source, path))
			if err != nil {
				return nil, err
			}
			res := make([]*provider.ResourceInfo, 0, len(content))
			for _, info := range content {
				base := filepath.Base(info.Name)
				res = append(res, f.convertToResourceInfo(
					info,
					filepath.Join(source, snapshot, path, base),
					encodeBackupInResourceID(id, snapshot, filepath.Join(source, path, base)),
					user.Id,
				))
			}
			return res, nil
		}
		// the path from the user is something like /eos/home-g/gdelmont
		// the method needs to return the list of snapshots as folders
		snapshots, err := f.client.ListSnapshots(ctx, user.Username, id)
		if err != nil {
			return nil, err
		}
		res := make([]*provider.ResourceInfo, 0, len(snapshots))
		for _, s := range snapshots {
			res = append(res, f.placeholderResourceInfo(filepath.Join(source, s.ID), user.Id))
		}
		return res, nil
	}

	// the path is not one of the backup. Can happen that the
	// user's path is a parent folder of some of the backups
	resSet := make(map[string]struct{}) // used to discard duplicates
	var resources []*provider.ResourceInfo

	sourceSplit := []string{""}
	if ref.Path != "/" {
		sourceSplit = strings.Split(ref.Path, "/")
	}
	for _, b := range backups {
		backupSplit := strings.Split(b.Source, "/")
		if hasPrefix(backupSplit, sourceSplit) {
			base := backupSplit[len(sourceSplit)]
			path := filepath.Join(source, base)

			if _, ok := resSet[path]; !ok {
				resources = append(resources, f.placeholderResourceInfo(path, user.Id))
				resSet[path] = struct{}{}
			}
		}
	}

	if len(resources) != 0 {
		return resources, nil
	}

	return nil, errtypes.NotFound(fmt.Sprintf("path %s does not exist", ref.Path))

}

func (f *cbackfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	user, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, errtypes.UserRequired("cback: user not found in context")
	}

	stat, err := f.GetMD(ctx, ref, nil)
	if err != nil {
		return nil, errors.Wrap(err, "cback: error statting resource")
	}

	if stat.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		return nil, errtypes.BadRequest("cback: can only download files")
	}

	id, snapshot, path, ok := decodeResourceID(stat.Id)
	if !ok {
		return nil, errtypes.BadRequest("cback: can only download files")
	}

	return f.client.Download(ctx, user.Username, id, snapshot, path)
}

func (f *cbackfs) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) CreateHome(ctx context.Context) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) TouchFile(ctx context.Context, ref *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (str string, err error) {
	return "", errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, used uint64, err error) {
	return 0, 0, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) Shutdown(ctx context.Context) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (r *provider.CreateStorageSpaceResponse, err error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) ListRecycle(ctx context.Context, basePath, key, relativePath string) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("Operation Not Permitted")
}

func (f *cbackfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	return errtypes.NotSupported("Operation Not Permitted")

}

func (f *cbackfs) InitiateUpload(ctx context.Context, ref *provider.Reference, uploadLength int64, metadata map[string]string) (map[string]string, error) {
	return nil, errtypes.NotSupported("Operation Not Permitted")

}
