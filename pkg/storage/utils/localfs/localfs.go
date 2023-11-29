// Copyright 2018-2023 CERN
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

package localfs

import (
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"net/url"
	"os"
	"path"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/pkg/errors"
)

// Config holds the configuration details for the local fs.
type Config struct {
	Root                string `mapstructure:"root"`
	DataTransfersFolder string `mapstructure:"data_transfers_folder"`
	Uploads             string `mapstructure:"uploads"`
	DataDirectory       string `mapstructure:"data_directory"`
}

func (c *Config) ApplyDefaults() {
	if c.Root == "" {
		c.Root = "/var/tmp/reva"
	}

	if c.DataTransfersFolder == "" {
		c.DataTransfersFolder = "/DataTransfers"
	}

	c.DataDirectory = path.Join(c.Root, "data")
	c.Uploads = path.Join(c.Root, ".uploads")
}

type localfs struct {
	conf         *Config
	chunkHandler *chunking.ChunkHandler
}

// NewLocalFS returns a storage.FS interface implementation that controls then
// local filesystem.
func NewLocalFS(c *Config) (storage.FS, error) {
	c.ApplyDefaults()

	// create namespaces if they do not exist
	namespaces := []string{c.DataDirectory, c.Uploads}
	for _, v := range namespaces {
		if err := os.MkdirAll(v, 0755); err != nil {
			return nil, errors.Wrap(err, "could not create home dir "+v)
		}
	}

	return &localfs{
		conf:         c,
		chunkHandler: chunking.NewChunkHandler(c.Uploads),
	}, nil
}

func (fs *localfs) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *localfs) resolve(ctx context.Context, ref *provider.Reference) (p string, err error) {
	if ref.ResourceId != nil {
		if p, err = fs.GetPathByID(ctx, ref.ResourceId); err != nil {
			return "", err
		}
		return path.Join(p, path.Join("/", ref.Path)), nil
	}

	if ref.Path != "" {
		return path.Join("/", ref.Path), nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v. at least resource_id or path must be set", ref)
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "local: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (fs *localfs) wrap(ctx context.Context, p string) string {
	// This is to prevent path traversal.
	// With this p can't break out of its parent folder
	p = path.Join("/", p)
	internal := path.Join(fs.conf.DataDirectory, p)
	return internal
}

func (fs *localfs) unwrap(ctx context.Context, np string) string {
	ns := fs.getNsMatch(np, []string{fs.conf.DataDirectory})
	var external string
	external = strings.TrimPrefix(np, ns)
	if external == "" {
		external = "/"
	}
	return external
}

func (fs *localfs) getNsMatch(internal string, nss []string) string {
	var match string
	for _, ns := range nss {
		if strings.HasPrefix(internal, ns) && len(ns) > len(match) {
			match = ns
		}
	}
	if match == "" {
		panic(fmt.Sprintf("local: path is outside namespaces: path=%s namespaces=%+v", internal, nss))
	}

	return match
}

// permissionSet returns the permission set for the current user.
func (fs *localfs) permissionSet(ctx context.Context, owner *userpb.UserId) *provider.ResourcePermissions {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}
	if u.Id == nil {
		return &provider.ResourcePermissions{
			// no permissions
		}
	}
	if u.Id.OpaqueId == owner.OpaqueId && u.Id.Idp == owner.Idp {
		return &provider.ResourcePermissions{
			// owner has all permissions
			AddGrant:             true,
			CreateContainer:      true,
			Delete:               true,
			GetPath:              true,
			GetQuota:             true,
			InitiateFileDownload: true,
			InitiateFileUpload:   true,
			ListContainer:        true,
			ListFileVersions:     true,
			ListGrants:           true,
			ListRecycle:          true,
			Move:                 true,
			PurgeRecycle:         true,
			RemoveGrant:          true,
			RestoreFileVersion:   true,
			RestoreRecycleItem:   true,
			Stat:                 true,
			UpdateGrant:          true,
		}
	}
	// TODO fix permissions for share recipients by traversing reading acls up to the root? cache acls for the parent node and reuse it
	return &provider.ResourcePermissions{
		AddGrant:             true,
		CreateContainer:      true,
		Delete:               true,
		GetPath:              true,
		GetQuota:             true,
		InitiateFileDownload: true,
		InitiateFileUpload:   true,
		ListContainer:        true,
		ListFileVersions:     true,
		ListGrants:           true,
		ListRecycle:          true,
		Move:                 true,
		PurgeRecycle:         true,
		RemoveGrant:          true,
		RestoreFileVersion:   true,
		RestoreRecycleItem:   true,
		Stat:                 true,
		UpdateGrant:          true,
	}
}

func (fs *localfs) normalize(ctx context.Context, fi os.FileInfo, fn string, mdKeys []string) (*provider.ResourceInfo, error) {
	fp := fs.unwrap(ctx, path.Join("/", fn))
	owner, err := getUser(ctx)
	if err != nil {
		return nil, err
	}
	metadata, err := fs.retrieveArbitraryMetadata(ctx, fn, mdKeys)
	if err != nil {
		return nil, err
	}

	var layout string

	// A fileid is constructed like `fileid-url_encoded_path`. See GetPathByID for the inverse conversion
	md := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: "fileid-" + url.QueryEscape(path.Join(layout, fp))},
		Path:          fp,
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fp),
		Size:          uint64(fi.Size()),
		PermissionSet: fs.permissionSet(ctx, owner.Id),
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
		Owner:             owner.Id,
		ArbitraryMetadata: metadata,
	}

	return md, nil
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

func (fs *localfs) retrieveArbitraryMetadata(ctx context.Context, fn string, mdKeys []string) (*provider.ArbitraryMetadata, error) {
	return nil, errtypes.NotSupported("localfs: not supported")
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is in the form `fileid-url_encoded_path`.
func (fs *localfs) GetPathByID(ctx context.Context, ref *provider.ResourceId) (string, error) {
	var layout string
	return url.QueryUnescape(strings.TrimPrefix(ref.OpaqueId, "fileid-"+layout))
}

func (fs *localfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) error {
	return errtypes.NotSupported("localfs: deny grant not supported")
}

func (fs *localfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("localfs: deny grant not supported")
}

func (fs *localfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	return nil, errtypes.NotSupported("localfs: deny grant not supported")
}

func (fs *localfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("localfs: deny grant not supported")
}

func (fs *localfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("localfs: deny grant not supported")
}

func (fs *localfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("localfs: not supported")
}

// CreateStorageSpace creates a storage space.
func (fs *localfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, fmt.Errorf("unimplemented: CreateStorageSpace")
}

func (fs *localfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("localfs: not supported")
}

// GetLock returns an existing lock on the given reference.
func (fs *localfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

// SetLock puts a lock on the given reference.
func (fs *localfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("unimplemented")
}

// RefreshLock refreshes an existing lock on the given reference.
func (fs *localfs) RefreshLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock, existingLockID string) error {
	return errtypes.NotSupported("unimplemented")
}

// Unlock removes an existing lock from the given reference.
func (fs *localfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *localfs) GetHome(ctx context.Context) (string, error) {
	return "", errtypes.NotSupported("local: get home not supported")
}

func (fs *localfs) CreateHome(ctx context.Context) error {
	return errtypes.NotSupported("localfs: create home not supported")
}

func (fs *localfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil
	}

	fn = fs.wrap(ctx, fn)
	if _, err := os.Stat(fn); err == nil {
		return errtypes.AlreadyExists(fn)
	}
	err = os.Mkdir(fn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		return errors.Wrap(err, "localfs: error creating dir "+fn)
	}

	return nil
}

// TouchFile as defined in the storage.FS interface.
func (fs *localfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	return fmt.Errorf("unimplemented: TouchFile")
}

func (fs *localfs) Delete(ctx context.Context, ref *provider.Reference) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	var fp string
	_, err = os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		return errors.Wrap(err, "localfs: error stating "+fp)
	}

	if err := os.RemoveAll(fp); err != nil {
		return errors.Wrap(err, "localfs: could not delete item")
	}

	return nil
}

func (fs *localfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
	oldName, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	newName, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "localfs: error resolving ref")
	}

	oldName = fs.wrap(ctx, oldName)
	newName = fs.wrap(ctx, newName)

	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}

	return nil
}

func (fs *localfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	fn = fs.wrap(ctx, fn)
	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	return fs.normalize(ctx, md, fn, mdKeys)
}

func (fs *localfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) ([]*provider.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	if fn == "/" {
		homeFiles, err := fs.listFolder(ctx, fn, mdKeys)
		if err != nil {
			return nil, err
		}
		return homeFiles, nil
	}

	return fs.listFolder(ctx, fn, mdKeys)
}

func (fs *localfs) listFolder(ctx context.Context, fn string, mdKeys []string) ([]*provider.ResourceInfo, error) {
	fn = fs.wrap(ctx, fn)

	entries, err := os.ReadDir(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error listing "+fn)
	}

	mds := make([]iofs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		mds = append(mds, info)
	}

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		info, err := fs.normalize(ctx, md, path.Join(fn, md.Name()), mdKeys)
		if err == nil {
			finfos = append(finfos, info)
		}
	}
	return finfos, nil
}

func (fs *localfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	fn = fs.wrap(ctx, fn)
	r, err := os.Open(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+fn)
	}
	return r, nil
}

func (fs *localfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) ListRecycle(ctx context.Context, basePath, key, relativePath string) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("localfs: not supported")
}

func (fs *localfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("localfs: not supported")
}

// UpdateStorageSpace updates a storage space.
func (fs *localfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("update storage space")
}
