// Copyright 2018-2020 CERN
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
	"net/url"
	"os"
	"path"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/templates"
	"github.com/cs3org/reva/pkg/user"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("local", New)
}

type config struct {
	Root       string `mapstructure:"root"`
	EnableHome bool   `mapstructure:"enable_home"`
	UserLayout string `mapstructure:"user_layout"`
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

	// defaults for Root
	if c.Root == "" {
		c.Root = "/var/tmp/reva/"
	}

	if c.UserLayout == "" {
		c.UserLayout = "{{.Username}}"
	}

	// create namespace if it does not exist
	if err = os.MkdirAll(c.Root, 0755); err != nil {
		return nil, errors.Wrap(err, "local: could not create namespace dir")
	}

	return &localfs{root: c.Root, conf: c}, nil
}

func (fs *localfs) Shutdown(ctx context.Context) error {
	return nil
}

func (fs *localfs) resolve(ctx context.Context, ref *provider.Reference) (string, error) {
	if ref.GetPath() != "" {
		return fs.wrap(ctx, ref.GetPath()), nil
	}

	if ref.GetId() != nil {
		fn := path.Join("/", strings.TrimPrefix(ref.GetId().OpaqueId, "fileid-"))
		fn = fs.wrap(ctx, fn)
		return fn, nil
	}

	// reference is invalid
	return "", fmt.Errorf("local: invalid reference %+v", ref)
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "local: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

func (fs *localfs) wrap(ctx context.Context, p string) (internal string) {
	if fs.conf.EnableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		internal = path.Join(fs.conf.Root, layout, p)
	} else {
		internal = path.Join(fs.conf.Root, p)
	}
	return
}

func (fs *localfs) unwrap(ctx context.Context, np string) (external string) {
	if fs.conf.EnableHome {
		layout, err := fs.GetHome(ctx)
		if err != nil {
			panic(err)
		}
		trim := path.Join(fs.conf.Root, layout)
		external = strings.TrimPrefix(np, trim)
	} else {
		external = strings.TrimPrefix(np, fs.conf.Root)
		if external == "" {
			external = "/"
		}
	}
	return
}

type localfs struct {
	root string
	conf *config
}

func (fs *localfs) normalize(ctx context.Context, fi os.FileInfo, fn string) *provider.ResourceInfo {
	fn = fs.unwrap(ctx, path.Join("/", fn))
	md := &provider.ResourceInfo{
		Id:            &provider.ResourceId{OpaqueId: "fileid-" + strings.TrimPrefix(fn, "/")},
		Path:          fn,
		Type:          getResourceType(fi.IsDir()),
		Etag:          calcEtag(ctx, fi),
		MimeType:      mime.Detect(fi.IsDir(), fn),
		Size:          uint64(fi.Size()),
		PermissionSet: &provider.ResourcePermissions{ListContainer: true, CreateContainer: true},
		Mtime: &types.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
	}

	//logger.Println(context.Background(), "normalized: ", md)
	return md
}

func getResourceType(isDir bool) provider.ResourceType {
	if isDir {
		return provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return provider.ResourceType_RESOURCE_TYPE_FILE
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is that path of the file without the first slash
// thus the file id always points to the filename
func (fs *localfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (string, error) {
	return path.Join("/", strings.TrimPrefix(id.OpaqueId, "fileid-")), nil
}

func (fs *localfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) ListGrants(ctx context.Context, ref *provider.Reference) ([]*provider.Grant, error) {
	return nil, errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("local: operation not supported")
}
func (fs *localfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}
func (fs *localfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) error {
	return errtypes.NotSupported("local: operation not supported")
}

func (fs *localfs) GetHome(ctx context.Context) (string, error) {
	if !fs.conf.EnableHome {
		return "", errtypes.NotSupported("local: get home not supported")
	}

	u, err := getUser(ctx)
	if err != nil {
		err = errors.Wrap(err, "local: wrap: no user in ctx and home is enabled")
		return "", err
	}
	relativeHome := templates.WithUser(u, fs.conf.UserLayout)

	return relativeHome, nil
}

func (fs *localfs) CreateHome(ctx context.Context) error {
	if !fs.conf.EnableHome {
		return errtypes.NotSupported("eos: create home not supported")
	}

	home := fs.wrap(ctx, "/")

	_, err := os.Stat(home)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "local: error stating home:"+home)
		}
	}

	err = os.MkdirAll(home, 0700)
	if err != nil {
		return errors.Wrap(err, "local: error creating home dir:"+home)
	}
	return nil
}

func (fs *localfs) CreateDir(ctx context.Context, fn string) error {
	fn = fs.wrap(ctx, fn)
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

func (fs *localfs) Delete(ctx context.Context, ref *provider.Reference) error {
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

func (fs *localfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) error {
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

func (fs *localfs) GetMD(ctx context.Context, ref *provider.Reference) (*provider.ResourceInfo, error) {
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

func (fs *localfs) ListFolder(ctx context.Context, ref *provider.Reference) ([]*provider.ResourceInfo, error) {
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

	finfos := []*provider.ResourceInfo{}
	for _, md := range mds {
		finfos = append(finfos, fs.normalize(ctx, md, path.Join(fn, md.Name())))
	}
	return finfos, nil
}

func (fs *localfs) Upload(ctx context.Context, ref *provider.Reference, r io.ReadCloser) error {
	fn, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "error resolving ref")
	}

	// we cannot rely on /tmp as it can live in another partition and we can
	// hit invalid cross-device link errors, so we create the tmp file in the same directory
	// the file is supposed to be written.
	tmp, err := ioutil.TempFile(path.Dir(fn), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "localfs: error creating tmp fn at "+path.Dir(fn))
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "localfs: eror writing to tmp file "+tmp.Name())
	}

	// TODO(labkode): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), fn); err != nil {
		return errors.Wrap(err, "localfs: error renaming from "+tmp.Name()+" to "+fn)
	}

	return nil
}

func (fs *localfs) Download(ctx context.Context, ref *provider.Reference) (io.ReadCloser, error) {
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

func (fs *localfs) ListRevisions(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	return nil, errtypes.NotSupported("list revisions")
}

func (fs *localfs) DownloadRevision(ctx context.Context, ref *provider.Reference, revisionKey string) (io.ReadCloser, error) {
	return nil, errtypes.NotSupported("download revision")
}

func (fs *localfs) RestoreRevision(ctx context.Context, ref *provider.Reference, revisionKey string) error {
	return errtypes.NotSupported("restore revision")
}

func (fs *localfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errtypes.NotSupported("purge recycle item")
}

func (fs *localfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("empty recycle")
}

func (fs *localfs) ListRecycle(ctx context.Context) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("list recycle")
}

func (fs *localfs) RestoreRecycleItem(ctx context.Context, restoreKey string) error {
	return errtypes.NotSupported("restore recycle")
}
