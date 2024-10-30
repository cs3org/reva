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

//go:build ceph
// +build ceph

package cephfs

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	goceph "github.com/ceph/go-ceph/cephfs"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
)

const (
	xattrUserNs = "user."
	xattrLock   = xattrUserNs + "reva.lockpayload"
)

type cephfs struct {
	conf         *Options
	conn         *connections
	adminConn    *adminConn
	chunkHandler *ChunkHandler
}

func init() {
	registry.Register("cephfs", New)
}

// New returns an implementation to of the storage.FS interface that talks to
// a ceph filesystem.
func New(ctx context.Context, m map[string]interface{}) (fs storage.FS, err error) {
	var o Options
	if err := cfg.Decode(m, &o); err != nil {
		return nil, err
	}

	var cache *connections
	if cache, err = newCache(); err != nil {
		return nil, errors.New("cephfs: can't create caches")
	}

	adminConn, err := newAdminConn(&o)
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: couldn't create admin connections")
	}

	return &cephfs{
		conf:      &o,
		conn:      cache,
		adminConn: adminConn,
	}, nil
}

func (fs *cephfs) GetHome(ctx context.Context) (string, error) {
	log := appctx.GetLogger(ctx)
	user := fs.makeUser(ctx)
	log.Debug().Interface("user", user).Msg("GetHome for user")
	return user.home, nil
}

func (fs *cephfs) CreateHome(ctx context.Context) (err error) {
	log := appctx.GetLogger(ctx)

	user := fs.makeUser(ctx)
	log.Debug().Interface("user", user).Msg("CreateHome for user")

	// Skip home creation if the directory already exists.
	// We do not check for all necessary attributes, only for the existence of the directory.
	stat, err := fs.adminConn.adminMount.Statx(user.home, goceph.StatxMode, 0)
	if err != nil {
		return errors.Wrap(err, "error stating user home when trying to create it")
	}

	log.Debug().Interface("stat", stat).Msgf("home is %s")

	// TODO(labkode): for now we always try to create the home directory even if it exists.
	// One needs to check for "no such of file or directory" error to short-cut.

	err = walkPath(user.home, func(path string) error {
		return fs.adminConn.adminMount.MakeDir(path, fs.conf.DirPerms)
	}, false)
	if err != nil {
		return getRevaError(ctx, err)
	}

	err = fs.adminConn.adminMount.Chown(user.home, uint32(user.UidNumber), uint32(user.GidNumber))
	if err != nil {
		return getRevaError(ctx, err)
	}

	err = fs.adminConn.adminMount.SetXattr(user.home, "ceph.quota.max_bytes", []byte(fmt.Sprint(fs.conf.UserQuotaBytes)), 0)
	if err != nil {
		return getRevaError(ctx, err)
	}

	return nil
}

func (fs *cephfs) CreateDir(ctx context.Context, ref *provider.Reference) error {
	user := fs.makeUser(ctx)
	path, err := user.resolveRef(ref)
	if err != nil {
		return getRevaError(ctx, err)
	}

	user.op(func(cv *cacheVal) {
		if err = cv.mount.MakeDir(path, fs.conf.DirPerms); err != nil {
			return
		}
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var path string
	user := fs.makeUser(ctx)
	path, err = user.resolveRef(ref)
	if err != nil {
		return err
	}

	user.op(func(cv *cacheVal) {
		if err = cv.mount.Unlink(path); err != nil && err.Error() == errIsADirectory {
			err = cv.mount.RemoveDir(path)
		}
	})

	//has already been deleted by direct mount
	if err != nil && err.Error() == errNotFound {
		return nil
	}

	return getRevaError(ctx, err)
}

func (fs *cephfs) Move(ctx context.Context, oldRef, newRef *provider.Reference) (err error) {
	var oldPath, newPath string
	user := fs.makeUser(ctx)
	if oldPath, err = user.resolveRef(oldRef); err != nil {
		return
	}
	if newPath, err = user.resolveRef(newRef); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		if err = cv.mount.Rename(oldPath, newPath); err != nil {
			return
		}
	})

	// has already been moved by direct mount
	if err != nil && err.Error() == errNotFound {
		return nil
	}

	return getRevaError(ctx, err)
}

func (fs *cephfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	if ref == nil {
		return nil, errors.New("error: ref is nil")
	}

	var path string
	user := fs.makeUser(ctx)

	if path, err = user.resolveRef(ref); err != nil {
		return nil, err
	}

	user.op(func(cv *cacheVal) {
		var stat Statx
		if stat, err = cv.mount.Statx(path, goceph.StatxBasicStats, 0); err != nil {
			return
		}
		ri, err = user.fileAsResourceInfo(cv, path, stat, mdKeys)
	})

	return ri, getRevaError(ctx, err)
}

func (fs *cephfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (files []*provider.ResourceInfo, err error) {
	if ref == nil {
		return nil, errors.New("error: ref is nil")
	}

	log := appctx.GetLogger(ctx)
	log.Debug().Interface("ref", ref)
	user := fs.makeUser(ctx)

	var path string
	if path, err = user.resolveRef(ref); err != nil {
		return nil, err
	}

	user.op(func(cv *cacheVal) {
		var dir *goceph.Directory
		if dir, err = cv.mount.OpenDir(path); err != nil {
			return
		}
		defer closeDir(dir)

		var entry *goceph.DirEntryPlus
		var ri *provider.ResourceInfo

		for entry, err = dir.ReadDirPlus(goceph.StatxBasicStats, 0); entry != nil && err == nil; entry, err = dir.ReadDirPlus(goceph.StatxBasicStats, 0) {
			if fs.conf.HiddenDirs[entry.Name()] {
				continue
			}

			ri, err = user.fileAsResourceInfo(cv, filepath.Join(path, entry.Name()), entry.Statx(), mdKeys)
			if ri == nil || err != nil {
				if err != nil {
					log := appctx.GetLogger(ctx)
					log.Err(err).Msg("cephfs: error in file as resource info")
				}
				err = nil
				continue
			}

			files = append(files, ri)
		}
	})

	return files, getRevaError(ctx, err)
}

func (fs *cephfs) Download(ctx context.Context, ref *provider.Reference) (rc io.ReadCloser, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return nil, errors.Wrap(err, "cephfs: error resolving ref")
	}

	user.op(func(cv *cacheVal) {
		rc, err = cv.mount.Open(path, os.O_RDONLY, 0)
	})

	return rc, getRevaError(ctx, err)
}

func (fs *cephfs) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	return nil, errtypes.NotSupported("cephfs:  RestoreRevision not supported")
}

func (fs *cephfs) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	return nil, errtypes.NotSupported("cephfs:  RestoreRevision not supported")
}

func (fs *cephfs) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	return errtypes.NotSupported("cephfs:  RestoreRevision not supported")
}

func (fs *cephfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (str string, err error) {
	return "", errtypes.NotSupported("cephfs: ids currently not supported")
}

func (fs *cephfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		err = fs.changePerms(ctx, cv.mount, g, path, updateGrant)
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		err = fs.changePerms(ctx, cv.mount, g, path, removeGrant)
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		err = fs.changePerms(ctx, cv.mount, g, path, updateGrant)
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) DenyGrant(ctx context.Context, ref *provider.Reference, g *provider.Grantee) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		grant := &provider.Grant{Grantee: g} //nil perms will remove the whole grant
		err = fs.changePerms(ctx, cv.mount, grant, path, removeGrant)
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv *cacheVal) {
		glist = fs.getFullPermissionSet(ctx, cv.mount, path)

		if glist == nil {
			err = errors.New("cephfs: error listing grants on " + path)
		}
	})

	return glist, getRevaError(ctx, err)
}

func (fs *cephfs) GetQuota(ctx context.Context, ref *provider.Reference) (total uint64, used uint64, err error) {
	user := fs.makeUser(ctx)

	log := appctx.GetLogger(ctx)
	user.op(func(cv *cacheVal) {
		var buf []byte
		buf, err = cv.mount.GetXattr(".", "ceph.quota.max_bytes")
		if err != nil {
			log.Warn().Msg("cephfs: user quota bytes not set")
			total = fs.conf.UserQuotaBytes
		} else {
			total, _ = strconv.ParseUint(string(buf), 10, 64)
		}

		buf, err = cv.mount.GetXattr(".", "ceph.dir.rbytes")
		if err == nil {
			used, err = strconv.ParseUint(string(buf), 10, 64)
		}
	})

	return total, used, getRevaError(ctx, err)
}

func (fs *cephfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	return errors.New("error: CreateReference not implemented")
}

func (fs *cephfs) Shutdown(ctx context.Context) (err error) {
	ctx.Done()
	fs.conn.clearCache()
	_ = fs.adminConn.adminMount.Unmount()
	_ = fs.adminConn.adminMount.Release()
	fs.adminConn.radosConn.Shutdown()

	return
}

func (fs *cephfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return err
	}

	user.op(func(cv *cacheVal) {
		for k, v := range md.Metadata {
			if !strings.HasPrefix(k, xattrUserNs) {
				k = xattrUserNs + k
			}
			if e := cv.mount.SetXattr(path, k, []byte(v), 0); e != nil {
				err = errors.Wrap(err, e.Error())
				return
			}
		}
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return err
	}

	user.op(func(cv *cacheVal) {
		for _, key := range keys {
			if !strings.HasPrefix(key, xattrUserNs) {
				key = xattrUserNs + key
			}
			if e := cv.mount.RemoveXattr(path, key); e != nil {
				err = errors.Wrap(err, e.Error())
				return
			}
		}
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) TouchFile(ctx context.Context, ref *provider.Reference) error {
	user := fs.makeUser(ctx)
	path, err := user.resolveRef(ref)
	if err != nil {
		return getRevaError(ctx, err)
	}

	user.op(func(cv *cacheVal) {
		var file *goceph.File
		defer closeFile(file)
		if file, err = cv.mount.Open(path, os.O_CREATE|os.O_WRONLY, fs.conf.FilePerms); err != nil {
			return
		}
	})

	return getRevaError(ctx, err)
}

func (fs *cephfs) EmptyRecycle(ctx context.Context) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (r *provider.CreateStorageSpaceResponse, err error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) PurgeRecycleItem(ctx context.Context, basePath, key, relativePath string) error {
	return errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) ListStorageSpaces(ctx context.Context, filter []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

func (fs *cephfs) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errtypes.NotSupported("unimplemented")
}

var fnvHash = fnv.New32a()

func getHash(s string) uint64 {
	fnvHash.Write([]byte(s))
	defer fnvHash.Reset()
	return uint64(fnvHash.Sum32())
}

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

func (fs *cephfs) SetLock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	user := fs.makeUser(ctx)
	path, err := user.resolveRef(ref)
	if err != nil {
		return getRevaError(ctx, err)
	}

	op := goceph.LockEX
	if lock.Type == provider.LockType_LOCK_TYPE_SHARED {
		op = goceph.LockSH
	}

	user.op(func(cv *cacheVal) {
		var file *goceph.File
		defer closeFile(file)
		if file, err = cv.mount.Open(path, os.O_RDWR, fs.conf.FilePerms); err != nil {
			return
		}

		err = file.Flock(op|goceph.LockNB, getHash(lock.AppName))
	})

	if err == nil {
		// ok, we got the flock, now also store the related lock metadata
		md := &provider.ArbitraryMetadata{
			Metadata: map[string]string{
				xattrLock: encodeLock(lock),
			},
		}
		return fs.SetArbitraryMetadata(ctx, ref, md)
	}

	return getRevaError(ctx, err)
}

func (fs *cephfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	user := fs.makeUser(ctx)
	path, err := user.resolveRef(ref)
	if err != nil {
		return nil, getRevaError(ctx, err)
	}

	var l *provider.Lock
	user.op(func(cv *cacheVal) {
		buf, errXattr := cv.mount.GetXattr(path, xattrLock)
		if errXattr == nil {
			if l, err = decodeLock(string(buf)); err != nil {
				err = errors.Wrap(err, "malformed lock payload")
				return
			}
		}

		var file *goceph.File
		defer closeFile(file)
		if file, err = cv.mount.Open(path, os.O_RDWR, fs.conf.FilePerms); err != nil {
			// try and open with read-only permissions: if this succeeds, we just return
			// the metadata as is, otherwise we return the error on Open()
			if file, err = cv.mount.Open(path, os.O_RDONLY, fs.conf.FilePerms); err != nil {
				l = nil
			}
			return
		}

		if err = file.Flock(goceph.LockEX|goceph.LockNB, 0); err == nil {
			// success means the file was not locked, drop related metadata if present
			if l != nil {
				fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
				l = nil
			}
			file.Flock(goceph.LockUN|goceph.LockNB, 0)
			err = errtypes.NotFound("file was not locked")
			return
		}

		if errXattr != nil {
			// error here means we have a "foreign" flock with no CS3 metadata
			err = nil
			l = new(provider.Lock)
			l.AppName = "External"
			return
		}

		if time.Unix(int64(l.Expiration.Seconds), 0).After(time.Now()) {
			// the lock expired, drop
			fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
			file.Flock(goceph.LockUN|goceph.LockNB, getHash(l.AppName))
			err = errtypes.NotFound("file was not locked")
			l = nil
		}
		return
	})

	return l, getRevaError(ctx, err)
}

// TODO(lopresti) part of this logic is duplicated from eosfs.go, should be factored out
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

func (fs *cephfs) RefreshLock(ctx context.Context, ref *provider.Reference, newLock *provider.Lock, existingLockID string) error {
	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	// check if the holder is the same of the new lock
	if !sameHolder(oldLock, newLock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	if existingLockID != "" && oldLock.LockId != existingLockID {
		return errtypes.BadRequest("lock id does not match")
	}

	return fs.SetLock(ctx, ref, newLock)
}

func (fs *cephfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	user := fs.makeUser(ctx)
	path, err := user.resolveRef(ref)
	if err != nil {
		return getRevaError(ctx, err)
	}

	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file not found or not locked")
		default:
			return err
		}
	}

	// check if the lock id of the lock corresponds to the stored lock
	if oldLock.LockId != lock.LockId {
		return errtypes.BadRequest("lock id does not match")
	}

	if !sameHolder(oldLock, lock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	user.op(func(cv *cacheVal) {
		var file *goceph.File
		defer closeFile(file)
		if file, err = cv.mount.Open(path, os.O_RDWR, fs.conf.FilePerms); err != nil {
			return
		}

		err = file.Flock(goceph.LockUN|goceph.LockNB, getHash(lock.AppName))
	})

	if err == nil {
		return fs.UnsetArbitraryMetadata(ctx, ref, []string{xattrLock})
	}

	return getRevaError(ctx, err)
}
