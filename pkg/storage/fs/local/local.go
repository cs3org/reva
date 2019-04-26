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
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"syscall"

	"github.com/cernbox/reva/pkg/storage/fs/registry"

	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/mime"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

var logger = log.New("storage-provider-local")

func init() {
	registry.Register("local", New)
}

type config struct {
	Root string `mapstructure:"root"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
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
	os.MkdirAll(c.Root, 0755)

	return &localFS{root: c.Root}, nil
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

// calcEtag will create an etag based on the md5 of
// - mtime,
// - inode (if available),
// - device (if available) and
// - size.
// errors are logged, but an etag will still be returned
func calcEtag(fi os.FileInfo) string {
	h := md5.New()
	err := binary.Write(h, binary.BigEndian, fi.ModTime().Unix())
	if err != nil {
		logger.Error(context.Background(), err)
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		// take device and inode into account
		err = binary.Write(h, binary.BigEndian, stat.Ino)
		if err != nil {
			logger.Error(context.Background(), err)
		}
		err = binary.Write(h, binary.BigEndian, stat.Dev)
		if err != nil {
			logger.Error(context.Background(), err)
		}
	}
	err = binary.Write(h, binary.BigEndian, fi.Size())
	if err != nil {
		logger.Error(context.Background(), err)
	}
	return fmt.Sprintf(`"%x"`, h.Sum(nil))
}

func (fs *localFS) normalize(fi os.FileInfo, fn string) *storage.MD {
	fn = fs.removeRoot(path.Join("/", fn))
	md := &storage.MD{
		ID:          "fileid-" + strings.TrimPrefix(fn, "/"),
		Path:        fn,
		IsDir:       fi.IsDir(),
		Etag:        calcEtag(fi),
		Mime:        mime.Detect(fi.IsDir(), fn),
		Size:        uint64(fi.Size()),
		Permissions: &storage.PermissionSet{ListContainer: true, CreateContainer: true},
		Mtime: &storage.Timestamp{
			Seconds: uint64(fi.ModTime().Unix()),
		},
	}
	//logger.Println(context.Background(), "normalized: ", md)
	return md
}

// GetPathByID returns the path pointed by the file id
// In this implementation the file id is that path of the file without the first slash
// thus the file id always points to the filename
func (fs *localFS) GetPathByID(ctx context.Context, id string) (string, error) {
	return path.Join("/", strings.TrimPrefix(id, "fileid-")), nil
}

func (fs *localFS) AddGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *localFS) ListGrants(ctx context.Context, path string) ([]*storage.Grant, error) {
	return nil, notSupportedError("op not supported")
}

func (fs *localFS) RemoveGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}
func (fs *localFS) UpdateGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *localFS) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *localFS) CreateDir(ctx context.Context, fn string) error {
	fn = fs.addRoot(fn)
	err := os.Mkdir(fn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return notFoundError(fn)
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "localfs: error creating dir "+fn)
	}
	return nil
}

func (fs *localFS) Delete(ctx context.Context, fn string) error {
	fn = fs.addRoot(fn)
	err := os.Remove(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return notFoundError(fn)
		}
		// try recursive delete
		err = os.RemoveAll(fn)
		if err != nil {
			return errors.Wrap(err, "localfs: error deleting "+fn)
		}
	}
	return nil
}

func (fs *localFS) Move(ctx context.Context, oldName, newName string) error {
	oldName = fs.addRoot(oldName)
	newName = fs.addRoot(newName)
	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "localfs: error moving "+oldName+" to "+newName)
	}
	return nil
}

func (fs *localFS) GetMD(ctx context.Context, fn string) (*storage.MD, error) {
	fn = fs.addRoot(fn)
	md, err := os.Stat(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "localfs: error stating "+fn)
	}

	return fs.normalize(md, fn), nil
}

func (fs *localFS) ListFolder(ctx context.Context, fn string) ([]*storage.MD, error) {
	fn = fs.addRoot(fn)
	mds, err := ioutil.ReadDir(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "localfs: error listing "+fn)
	}

	finfos := []*storage.MD{}
	for _, md := range mds {
		finfos = append(finfos, fs.normalize(md, path.Join(fn, md.Name())))
	}
	return finfos, nil
}

func (fs *localFS) Upload(ctx context.Context, fn string, r io.ReadCloser) error {
	fn = fs.addRoot(fn)

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

	logger.Println(ctx, "renaming ", tmp.Name(), " to ", fn)
	// TODO(labkode): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), fn); err != nil {
		return errors.Wrap(err, "localfs: error renaming from "+tmp.Name()+" to "+fn)
	}

	return nil
}

func (fs *localFS) Download(ctx context.Context, fn string) (io.ReadCloser, error) {
	fn = fs.addRoot(fn)
	r, err := os.Open(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "localfs: error reading "+fn)
	}
	return r, nil
}

func (fs *localFS) ListRevisions(ctx context.Context, path string) ([]*storage.Revision, error) {
	return nil, notSupportedError("list revisions")
}

func (fs *localFS) DownloadRevision(ctx context.Context, path, revisionKey string) (io.ReadCloser, error) {
	return nil, notSupportedError("download revision")
}

func (fs *localFS) RestoreRevision(ctx context.Context, path, revisionKey string) error {
	return notSupportedError("restore revision")
}

func (fs *localFS) EmptyRecycle(ctx context.Context, path string) error {
	return notSupportedError("empty recycle")
}

func (fs *localFS) ListRecycle(ctx context.Context, path string) ([]*storage.RecycleItem, error) {
	return nil, notSupportedError("list recycle")
}

func (fs *localFS) RestoreRecycleItem(ctx context.Context, fn, restoreKey string) error {
	return notSupportedError("restore recycle")
}

type notSupportedError string
type notFoundError string

func (e notSupportedError) Error() string   { return string(e) }
func (e notSupportedError) IsNotSupported() {}
func (e notFoundError) Error() string       { return string(e) }
func (e notFoundError) IsNotFound()         {}
