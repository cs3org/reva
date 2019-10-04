// Copyright 2018-2019 CERN
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

package local

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/fs/registry"

	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
)

func init() {
	registry.Register("local", New)
}

type config struct {
	Root string `mapstructure:"root"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// create root if it does not exist
	if err = os.MkdirAll(c.Root, 0755); err != nil {
		return nil, err
	}

	return &localFS{root: c.Root}, nil
}

func (fs *localFS) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *localFS) resolve(ctx context.Context, ref *storageproviderv0alphapb.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.addRoot(ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		fn := path.Join("/", strings.TrimPrefix(ref.GetId().OpaqueId, "fileid-"))
		fn = fs.addRoot(fn)
		return fn, nil
	}

	// reference is invalid
	return "", fmt.Errorf("invalid reference %+v", ref)
}
func (fs *localFS) addRoot(p string) string {
	np := path.Join(fs.root, p)
	return np
}

func (fs *localFS) removeRoot(np string) string {
	p := strings.TrimPrefix(np, fs.root)
	if p == "" {
		p = "/"
	}
	return p
}

type localFS struct{ root string }

func (fs *localFS) normalize(ctx context.Context, fi os.FileInfo, fn string) *storageproviderv0alphapb.ResourceInfo {
	fn = fs.removeRoot(path.Join("/", fn))
	md := &storageproviderv0alphapb.ResourceInfo{
		Id:            &storageproviderv0alphapb.ResourceId{OpaqueId: "fileid-" + strings.TrimPrefix(fn, "/")},
		Path:          fn,
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fn),
		Size:          uint64(fi.Size()),
		PermissionSet: &storageproviderv0alphapb.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &typespb.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
	}

	//logger.Println(context.Background(), "normalized: ", md)
	return md
}

func getResourceType(isDir bool) storageproviderv0alphapb.ResourceType {
	if isDir {
		return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return storageproviderv0alphapb.ResourceType_RESOURCE_TYPE_FILE
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is that path of the file without the first slash
// thus the file id always points to the filename
func (fs *localFS) GetPathByID(ctx context.Context, id *storageproviderv0alphapb.ResourceId) (string, error) {
	return path.Join("/", strings.TrimPrefix(id.OpaqueId, "fileid-")), nil
}

func (fs *localFS) AddGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error {
	return errtypes.NotSupported("op not supported")
}

func (fs *localFS) ListGrants(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.Grant, error) {
	return nil, errtypes.NotSupported("op not supported")
}

func (fs *localFS) RemoveGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error {
	return errtypes.NotSupported("op not supported")
}
func (fs *localFS) UpdateGrant(ctx context.Context, ref *storageproviderv0alphapb.Reference, g *storageproviderv0alphapb.Grant) error {
	return errtypes.NotSupported("op not supported")
}

func (fs *localFS) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *localFS) CreateDir(ctx context.Context, fn string) error {
	fn = fs.addRoot(fn)
	err := os.Mkdir(fn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		// TODO(jfd): we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "localfs: error creating dir "+fn)
	}
	return nil
}

func (fs *localFS) Delete(ctx context.Context, ref *storageproviderv0alphapb.Reference) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	err = os.Remove(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return errtypes.NotFound(fn)
		}
		// try recursive delete
		err = os.RemoveAll(fn)
		if err != nil {
			return errors.Wrap(err, "localfs: error deleting "+fn)
		}
	}
	return nil
}

func (fs *localFS) Move(ctx context.Context, oldRef, newRef *storageproviderv0alphapb.Reference) error {
	oldName, err := fs.resolve(ctx, oldRef)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	newName, err := fs.resolve(ctx, newRef)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}
	return nil
}

func (fs *localFS) GetMD(ctx context.Context, ref *storageproviderv0alphapb.Reference) (*storageproviderv0alphapb.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving ref")
	}

	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	return fs.normalize(ctx, md, fn), nil
}

func (fs *localFS) ListFolder(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.ResourceInfo, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "error resolving ref")
	}

	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error listing "+fn)
	}

	finfos := []*storageproviderv0alphapb.ResourceInfo{}
	for _, md := range mds {
		finfos = append(finfos, fs.normalize(ctx, md, path.Join(fn, md.Name())))
	}
	return finfos, nil
}

// NewUpload returns an upload id that can be used for uploads with tus
func (fs *localFS) NewUpload(ctx context.Context, ref *storageproviderv0alphapb.Reference) (uploadID string, err error) {
	return "", errtypes.NotSupported("op not supported")
}

// Upload is deprecated, handled by tus
func (fs *localFS) Upload(ctx context.Context, ref *storageproviderv0alphapb.Reference, r io.ReadCloser) error {
	return errtypes.NotSupported("op not supported")
}

func (fs *localFS) Download(ctx context.Context, ref *storageproviderv0alphapb.Reference) (io.ReadCloser, error) {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "localfs: error resolving ref")
	}

	r, err := os.Open(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errtypes.NotFound(fn)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+fn)
	}
	return r, nil
}

func (fs *localFS) ListRevisions(ctx context.Context, ref *storageproviderv0alphapb.Reference) ([]*storageproviderv0alphapb.FileVersion, error) {
	return nil, errtypes.NotSupported("list revisions")
}

func (fs *localFS) DownloadRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *localFS) RestoreRevision(ctx context.Context, ref *storageproviderv0alphapb.Reference, revisionKey string) error {
	return errtypes.NotSupported("restore revision")
}

func (fs *localFS) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("empty recycle")
}

func (fs *localFS) ListRecycle(ctx context.Context) ([]*storageproviderv0alphapb.RecycleItem, error) {
	return nil, errtypes.NotSupported("list recycle")
}

func (fs *localFS) RestoreRecycleItem(ctx context.Context, restoreKey string) error {
	return errtypes.NotSupported("restore recycle")
}
