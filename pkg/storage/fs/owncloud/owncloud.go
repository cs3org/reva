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

package owncloud

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cs3org/reva/pkg/storage/fs/registry"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

const (
	// idAttribute is the name of the filesystem extended attribute that is used to store the uuid in
	idAttribute string = "user.oc.id"
)

func init() {
	registry.Register("owncloud", New)
}

type config struct {
	DataDirectory string `mapstructure:"datadirectory"`
	Scan          bool   `mapstructure:"scan"`
	Autocreate    bool   `mapstructure:"autocreate"`
	Redis         string `mapstructure:"redis"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func (c *config) init(m map[string]interface{}) {
	if c.Redis == "" {
		c.Redis = ":6379"
	}
	// default to scanning if not configured
	if _, ok := m["scan"]; !ok {
		c.Scan = true
	}
	// default to autocreate if not configured
	if _, ok := m["scan"]; !ok {
		c.Autocreate = true
	}
}

// New returns an implementation to of the storage.FS interface that talk to
// a local filesystem.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init(m)

	// c.DataDirectoryshould never end in / unless it is the root?
	c.DataDirectory = path.Clean(c.DataDirectory)

	// create root if it does not exist
	os.MkdirAll(c.DataDirectory, 0755)

	pool := &redis.Pool{

		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", c.Redis)
			if err != nil {
				return nil, err
			}
			return c, err
		},

		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &ocFS{c: c, pool: pool}, nil
}

type ocFS struct {
	c    *config
	pool *redis.Pool
}

func (fs *ocFS) Shutdown() error {
	return fs.pool.Close()
}

// scan files and add uuid to path mapping to kv store
func (fs *ocFS) scanFiles(ctx context.Context, conn redis.Conn) {
	if fs.c.Scan {
		fs.c.Scan = false // TODO ... in progress use mutex ?
		log := appctx.GetLogger(ctx)
		log.Debug().Str("path", fs.c.DataDirectory).Msg("scannig data directory")
		err := filepath.Walk(fs.c.DataDirectory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Error().Str("path", path).Err(err).Msg("error accessing path")
				return filepath.SkipDir
			}

			// reuse connection to store file ids
			id := readOrCreateID(context.Background(), path, nil)
			conn.Do("SET", id, path)

			log.Debug().Str("path", path).Str("id", id).Msg("scanned path")
			return nil
		})
		if err != nil {
			log.Error().Err(err).Str("path", fs.c.DataDirectory).Msg("error scanning data directory")
		}
	}
}

// ownloud stores files in the files subfolder
// the incoming path starts with /<username>, so we need to insert the files subfolder into the path
// and prefix the datadirectory
// TODO the path handed to a storage provider should not contain the username
func (fs *ocFS) jail(p string) string {
	// p = /<username> or
	// p = /<username>/foo/bar.txt
	parts := strings.SplitN(p, "/", 3)

	switch len(parts) {
	case 2:
		// parts = "", "<username>"
		return path.Join(fs.c.DataDirectory, parts[1], "files")
	case 3:
		// parts = "", "<username>", "foo/bar.txt"
		return path.Join(fs.c.DataDirectory, parts[1], "files", parts[2])
	default:
		return "" // TODO Must not happen?
	}

}
func (fs *ocFS) unJail(np string) string {
	// np = /data/<username>/files/foo/bar.txt
	// remove data dir
	if fs.c.DataDirectory != "/" {
		// fs.c.DataDirectory is a clean puth, so it never ends in /
		np = strings.TrimPrefix(np, fs.c.DataDirectory)
		// np = /<username>/files/foo/bar.txt
	}

	parts := strings.SplitN(np, "/", 4)
	// parts = "", "<username>", "files", "foo/bar.txt"
	switch len(parts) {
	case 1:
		return "/"
	case 2:
		return path.Join("/", parts[1])
	case 3:
		return path.Join("/", parts[1])
	default:
		return path.Join("/", parts[1], parts[3])
	}
}

// calcEtag will create an etag based on the md5 of
// - mtime,
// - inode (if available),
// - device (if available) and
// - size.
// errors are logged, but an etag will still be returned
func calcEtag(ctx context.Context, fi os.FileInfo) string {
	log := appctx.GetLogger(ctx)
	h := md5.New()
	err := binary.Write(h, binary.BigEndian, fi.ModTime().Unix())
	if err != nil {
		log.Error().Err(err).Msg("error writing mtime")
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		// take device and inode into account
		err = binary.Write(h, binary.BigEndian, stat.Ino)
		if err != nil {
			log.Error().Err(err).Msg("error writing inode")
		}
		err = binary.Write(h, binary.BigEndian, stat.Dev)
		if err != nil {
			log.Error().Err(err).Msg("error writing device")
		}
	}
	err = binary.Write(h, binary.BigEndian, fi.Size())
	if err != nil {
		log.Error().Err(err).Msg("error writing size")
	}
	return fmt.Sprintf(`"%x"`, h.Sum(nil))
}

func (fs *ocFS) normalize(ctx context.Context, fi os.FileInfo, fn string) *storage.MD {
	fn = fs.unJail(path.Join("/", fn))
	md := &storage.MD{
		ID:          "fileid-" + strings.TrimPrefix(fn, "/"),
		Path:        fn,
		IsDir:       fi.IsDir(),
		Etag:        calcEtag(ctx, fi),
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

func readOrCreateID(ctx context.Context, fn string, conn redis.Conn) string {
	log := appctx.GetLogger(ctx)

	// read extended file attribute for id
	//generate if not present
	var id []byte
	var err error
	if id, err = xattr.Get(fn, idAttribute); err != nil {
		log.Warn().Err(err).Msg("error reading file id")
		// try generating a uuid
		if uuid, err := uuid.NewV4(); err != nil {
			log.Error().Err(err).Msg("error generating fileid")
		} else {
			// store uuid
			id = uuid.Bytes()
			if err := xattr.Set(fn, idAttribute, id); err != nil {
				log.Error().Err(err).Msg("error storing file id")
			}
			// TODO cache path for uuid in redis
			// TODO reuse conn?
			if conn != nil {
				conn.Do("SET", uuid.String(), fn)
			}
		}
	}
	// todo sign metadata
	var uid uuid.UUID
	if uid, err = uuid.FromBytes(id); err != nil {
		log.Error().Err(err).Msg("error parsing uuid")
		return ""
	}
	return uid.String()
}

func (fs *ocFS) autocreate(ctx context.Context, fsfn string) error {
	log := appctx.GetLogger(ctx)
	if fs.c.Autocreate {
		err := os.MkdirAll(fsfn, 0700)
		if err != nil {
			log.Debug().
				Err(err).
				Str("fsfn", fsfn).
				Msg("could not autocreate home")
		}
	}
	return nil
}

// GetPathByID returns the path pointed by the file id
func (fs *ocFS) GetPathByID(ctx context.Context, id string) (string, error) {
	c := fs.pool.Get()
	defer c.Close()
	fs.scanFiles(ctx, c)
	s, err := redis.String(c.Do("GET", id))
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Msg("error reading fileid")
		return "", err
	}
	return s, nil
}

func (fs *ocFS) AddGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *ocFS) ListGrants(ctx context.Context, path string) ([]*storage.Grant, error) {
	return nil, notSupportedError("op not supported")
}

func (fs *ocFS) RemoveGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}
func (fs *ocFS) UpdateGrant(ctx context.Context, path string, g *storage.Grant) error {
	return notSupportedError("op not supported")
}

func (fs *ocFS) GetQuota(ctx context.Context) (int, int, error) {
	return 0, 0, nil
}

func (fs *ocFS) CreateDir(ctx context.Context, fn string) error {
	fsfn := fs.jail(fn)
	err := os.Mkdir(fsfn, 0700)
	if err != nil {
		if os.IsNotExist(err) {
			return notFoundError(fn)
		}
		// FIXME we also need already exists error, webdav expects 405 MethodNotAllowed
		return errors.Wrap(err, "ocFS: error creating dir "+fsfn)
	}
	return nil
}

func (fs *ocFS) Delete(ctx context.Context, fn string) error {
	fsfn := fs.jail(fn)
	err := os.Remove(fsfn)
	if err != nil {
		if os.IsNotExist(err) {
			return notFoundError(fn)
		}
		// try recursive delete
		err = os.RemoveAll(fsfn)
		if err != nil {
			return errors.Wrap(err, "ocFS: error deleting "+fsfn)
		}
	}
	return nil
}

func (fs *ocFS) Move(ctx context.Context, oldName, newName string) error {
	oldName = fs.jail(oldName)
	newName = fs.jail(newName)
	if err := os.Rename(oldName, newName); err != nil {
		return errors.Wrap(err, "ocFS: error moving "+oldName+" to "+newName)
	}
	return nil
}

func (fs *ocFS) GetMD(ctx context.Context, fn string) (*storage.MD, error) {
	fsfn := fs.jail(fn)
	fs.autocreate(ctx, fsfn)
	md, err := os.Stat(fsfn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "ocFS: error stating "+fsfn)
	}
	m := fs.normalize(ctx, md, fsfn)

	c := fs.pool.Get()
	defer c.Close()
	m.ID = readOrCreateID(ctx, fsfn, c)
	return m, nil
}

func (fs *ocFS) ListFolder(ctx context.Context, fn string) ([]*storage.MD, error) {
	fsfn := fs.jail(fn)
	fs.autocreate(ctx, fsfn)
	mds, err := ioutil.ReadDir(fsfn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "ocFS: error listing "+fsfn)
	}

	finfos := []*storage.MD{}
	c := fs.pool.Get()
	defer c.Close()
	for _, md := range mds {
		p := path.Join(fsfn, md.Name())
		m := fs.normalize(ctx, md, p)
		m.ID = readOrCreateID(ctx, p, c)
		finfos = append(finfos, m)
	}
	return finfos, nil
}

func (fs *ocFS) Upload(ctx context.Context, fn string, r io.ReadCloser) error {
	fsfn := fs.jail(fn)

	// we cannot rely on /tmp as it can live in another partition and we can
	// hit invalid cross-device link errors, so we create the tmp file in the same directory
	// the file is supposed to be written.
	tmp, err := ioutil.TempFile(path.Dir(fsfn), "._reva_atomic_upload")
	if err != nil {
		return errors.Wrap(err, "ocFS: error creating tmp fn at "+path.Dir(fsfn))
	}

	_, err = io.Copy(tmp, r)
	if err != nil {
		return errors.Wrap(err, "ocFS: eror writing to tmp file "+tmp.Name())
	}

	// TODO(labkode): make sure rename is atomic, missing fsync ...
	if err := os.Rename(tmp.Name(), fsfn); err != nil {
		return errors.Wrap(err, "ocFS: error renaming from "+tmp.Name()+" to "+fsfn)
	}

	return nil
}

func (fs *ocFS) Download(ctx context.Context, fn string) (io.ReadCloser, error) {
	fsfn := fs.jail(fn)
	r, err := os.Open(fsfn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFoundError(fn)
		}
		return nil, errors.Wrap(err, "ocFS: error reading "+fsfn)
	}
	return r, nil
}

func (fs *ocFS) ListRevisions(ctx context.Context, path string) ([]*storage.Revision, error) {
	return nil, notSupportedError("list revisions")
}

func (fs *ocFS) DownloadRevision(ctx context.Context, path, revisionKey string) (io.ReadCloser, error) {
	return nil, notSupportedError("download revision")
}

func (fs *ocFS) RestoreRevision(ctx context.Context, path, revisionKey string) error {
	return notSupportedError("restore revision")
}

func (fs *ocFS) EmptyRecycle(ctx context.Context, path string) error {
	return notSupportedError("empty recycle")
}

func (fs *ocFS) ListRecycle(ctx context.Context, path string) ([]*storage.RecycleItem, error) {
	return nil, notSupportedError("list recycle")
}

func (fs *ocFS) RestoreRecycleItem(ctx context.Context, fn, restoreKey string) error {
	return notSupportedError("restore recycle")
}

type notSupportedError string
type notFoundError string

func (e notSupportedError) Error() string   { return string(e) }
func (e notSupportedError) IsNotSupported() {}
func (e notFoundError) Error() string       { return string(e) }
func (e notFoundError) IsNotFound()         {}
