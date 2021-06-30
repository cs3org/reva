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

//+build ceph

package cephfs

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	cephfs2 "github.com/ceph/go-ceph/cephfs"
	"github.com/ceph/go-ceph/cephfs/admin"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	xattrTrustedNs = "trusted."
	xattrFid       = xattrTrustedNs + "fid"
	//xattrFileOwner = xattrFileNs + "owner"
	xattrMd5    = xattrTrustedNs + "md5sum"
	xattrMd5ts  = xattrTrustedNs + "md5sumTS"
	xattrRef    = xattrTrustedNs + "ref"
	xattrUserNs = "user."
	snap        = ".snap"
)

type cephfs struct {
	conf         *Options
	conn         *connections
	subvolAdmin  *admin.FSAdmin
	chunkHandler *chunking.ChunkHandler
}

func init() {
	registry.Register("cephfs", New)
}

// New returns an implementation to of the storage.FS interface that talk to
// a ceph filesystem.
func New(m map[string]interface{}) (fs storage.FS, err error) {
	c := &Options{}
	if err = mapstructure.Decode(m, c); err != nil {
		return nil, errors.Wrap(err, "error decoding conf")
	}

	var cache *connections
	var fsadm *admin.FSAdmin
	if cache, err = newCache(); err != nil {
		return nil, errors.New("cephfs: can't create rCache")
	}
	if fsadm, err = admin.New(); err != nil {
		return nil, errors.New("cephfs: can't create fs admin connection")
	}

	return &cephfs{
		conf:         c,
		conn:         cache,
		subvolAdmin:  fsadm,
		chunkHandler: chunking.NewChunkHandler(c.Uploads),
	}, nil
}

func (fs *cephfs) GetHome(ctx context.Context) (string, error) {
	if fs.conf.DisableHome {
		return "", errtypes.NotSupported("cephfs: GetHome() home supported disabled")
	}

	user := fs.makeUser(ctx)

	return fs.subvolAdmin.SubVolumePath("cephfs", "reva", user.Username)
}

func (fs *cephfs) CreateHome(ctx context.Context) (err error) {
	if fs.conf.DisableHome {
		return errtypes.NotSupported("cephfs: GetHome() home supported disabled")
	}

	user := fs.makeUser(ctx)
	err = fs.subvolAdmin.CreateSubVolume("cephfs", "reva", user.Username, &admin.SubVolumeOptions{
		Size: admin.ByteCount(fs.conf.UserQuotaBytes),
		Uid:  int(user.UidNumber),
		Gid:  int(user.GidNumber),
		Mode: 0755,
	})

	return
}

func (fs *cephfs) CreateDir(ctx context.Context, fn string) (err error) {
	user := fs.makeUser(ctx)

	user.op(func(cv cacheVal) {
		err = cv.mount.MakeDir(fn, fs.conf.DirMode)
	})

	return
}

func (fs *cephfs) Delete(ctx context.Context, ref *provider.Reference) (err error) {
	var path string
	user := fs.makeUser(ctx)
	path, err = user.resolveRef(ref)
	if err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		var fid []byte
		if err = cv.mount.Unlink(path); err != nil {
			return
		}
		if fid, err = cv.mount.GetXattr(path, xattrFid); err != nil {
			return
		}
		if err = cv.mount.Unlink(string(fid)); err != nil {
			return
		}
	})

	return
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

	user.op(func(cv cacheVal) {
		var fid []byte
		if err = cv.mount.Rename(oldPath, newPath); err != nil {
			return
		}
		if fid, err = cv.mount.GetXattr(newPath, xattrFid); err != nil {
			return
		}
		if err = cv.mount.Unlink(string(fid)); err != nil {
			return
		}
		err = cv.mount.Symlink(newPath, string(fid))
	})

	return
}

func (fs *cephfs) GetMD(ctx context.Context, ref *provider.Reference, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return nil, err
	}

	user.op(func(cv cacheVal) {
		var stat Statx
		if stat, err = cv.mount.Statx(path, cephfs2.StatxBasicStats, 0); err != nil {
			return
		}
		ri, err = user.fileAsResourceInfo(cv, path, stat, mdKeys)
	})

	return
}

func (fs *cephfs) ListFolder(ctx context.Context, ref *provider.Reference, mdKeys []string) (files []*provider.ResourceInfo, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		var dir *cephfs2.Directory
		if dir, err = cv.mount.OpenDir(path); err != nil {
			return
		}
		defer closeDir(dir)

		var entry *cephfs2.DirEntryPlus
		var ri *provider.ResourceInfo
		for entry, err = dir.ReadDirPlus(cephfs2.StatxBasicStats, 0); entry != nil && err == nil; entry, err = dir.ReadDirPlus(cephfs2.StatxBasicStats, 0) {
			if entry.Name() == "." || entry.Name() == ".." {
				continue
			}

			ri, err = user.fileAsResourceInfo(cv, filepath.Join(path, entry.Name()), entry.Statx(), mdKeys)
			if ri == nil || err != nil {
				//TODO: Maybe try and stack the errors, or just ignore
				err = nil
				continue
			}

			files = append(files, ri)
		}
	})

	return
}

func (fs *cephfs) Download(ctx context.Context, ref *provider.Reference) (rc io.ReadCloser, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return nil, errors.Wrap(err, "cephfs: error resolving ref")
	}

	user.op(func(cv cacheVal) {
		if strings.HasPrefix(strings.TrimPrefix(path, cv.homePath), fs.conf.ShareFolder) {
			err = errtypes.PermissionDenied("cephfs: cannot download under the virtual share folder")
			return
		}
		rc, err = cv.mount.Open(path, os.O_RDONLY, filePermDefault)
	})

	return
}

func (fs *cephfs) ListRevisions(ctx context.Context, ref *provider.Reference) (fvs []*provider.FileVersion, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return nil, errors.Wrap(err, "cephfs: error resolving ref")
	}

	user.op(func(cv cacheVal) {
		if strings.HasPrefix(strings.TrimPrefix(path, cv.homePath), fs.conf.ShareFolder) {
			err = errtypes.PermissionDenied("cephfs: cannot download under the virtual share folder")
			return
		}
		var dir *cephfs2.Directory
		if dir, err = cv.mount.OpenDir(".snap"); err != nil {
			return
		}
		defer closeDir(dir)

		for d, _ := dir.ReadDir(); d != nil; d, _ = dir.ReadDir() {
			var revPath string
			var stat Statx
			var e error

			if strings.HasPrefix(d.Name(), ".") {
				continue
			}

			revPath, e = resolveRevRef(cv.mount, ref, d.Name())
			if e != nil {
				continue
			}
			stat, e = cv.mount.Statx(revPath, cephfs2.StatxMtime|cephfs2.StatxSize, 0)
			if e != nil {
				continue
			}
			fvs = append(fvs, &provider.FileVersion{
				Key:   d.Name(),
				Size:  stat.Size,
				Mtime: uint64(stat.Mtime.Sec),
			})
		}
	})

	return
}

func (fs *cephfs) DownloadRevision(ctx context.Context, ref *provider.Reference, key string) (file io.ReadCloser, err error) {
	user := fs.makeUser(ctx)

	user.op(func(cv cacheVal) {
		var revPath string
		revPath, err = resolveRevRef(cv.mount, ref, key)
		if err != nil {
			return
		}

		file, err = cv.mount.Open(revPath, os.O_RDONLY, filePermDefault)
	})

	return
}

func (fs *cephfs) RestoreRevision(ctx context.Context, ref *provider.Reference, key string) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return errors.Wrap(err, "cephfs: error resolving ref")
	}

	user.op(func(cv cacheVal) {
		var revPath string
		if revPath, err = resolveRevRef(cv.mount, ref, key); err != nil {
			err = errors.Wrap(err, "cephfs: error resolving revision ref "+ref.String())
			return
		}

		var src, dst *cephfs2.File
		if src, err = cv.mount.Open(revPath, os.O_RDONLY, filePermDefault); err != nil {
			return
		}
		defer closeFile(src)

		if dst, err = cv.mount.Open(path, os.O_WRONLY|os.O_TRUNC, filePermDefault); err != nil {
			return
		}
		defer closeFile(dst)

		_, err = io.Copy(dst, src)
	})

	return
}

func (fs *cephfs) ListRecycle(ctx context.Context) (ri []*provider.RecycleItem, err error) {
	user := fs.makeUser(ctx)
	user.op(func(cv cacheVal) {
		var snapdir, basicDir *cephfs2.Directory
		var cEntry *cephfs2.DirEntry

		snapdir, err = cv.mount.OpenDir(".snap")
		if err != nil {
			return
		}
		defer closeDir(snapdir)

		basicDir, err = cv.mount.OpenDir(".")
		if err != nil {
			return
		}
		defer closeDir(basicDir)

		// Skip . and  ..
		for cEntry, err = basicDir.ReadDir(); err == nil && cEntry != nil && strings.HasPrefix(cEntry.Name(), "."); cEntry, err = basicDir.ReadDir() {
		}
		if err != nil {
			return
		}

		for snap, e := snapdir.ReadDirPlus(cephfs2.StatxBasicStats, 0); snap != nil && e == nil; snap, e = snapdir.ReadDirPlus(cephfs2.StatxBasicStats, 0) {
			if strings.HasPrefix(snap.Name(), ".") {
				continue
			}

		}
	})

	return
}

func (fs *cephfs) RestoreRecycleItem(ctx context.Context, key string, ref *provider.Reference) error {
	panic("implement me")
}

func (fs *cephfs) PurgeRecycleItem(ctx context.Context, key string) error {
	return errors.New("cephfs: Recycled items can't be purged, they are handled by snapshots, which are read-only")
}

func (fs *cephfs) EmptyRecycle(ctx context.Context) error {
	return errors.New("cephfs: recycle is based on snapshots and can't be edited")
}

func (fs *cephfs) GetPathByID(ctx context.Context, id *provider.ResourceId) (str string, err error) {
	user := fs.makeUser(ctx)

	user.op(func(cv cacheVal) {
		str, err = cv.mount.Readlink(id.OpaqueId)
	})

	return
}

func (fs *cephfs) AddGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		err = changePerms(cv.mount, g, path, updateGrant)
	})

	return
}

func (fs *cephfs) RemoveGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		err = changePerms(cv.mount, g, path, removeGrant)
	})

	return
}

func (fs *cephfs) UpdateGrant(ctx context.Context, ref *provider.Reference, g *provider.Grant) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		err = changePerms(cv.mount, g, path, updateGrant)
	})

	return
}

func (fs *cephfs) ListGrants(ctx context.Context, ref *provider.Reference) (glist []*provider.Grant, err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		glist = getFullPermissionSet(cv.mount, path)

		if glist == nil {
			err = errors.New("cephfs: error listing grants on " + path)
		}
	})

	return
}

func (fs *cephfs) GetQuota(ctx context.Context) (total uint64, used uint64, err error) {
	user := fs.makeUser(ctx)

	user.op(func(cv cacheVal) {
		var buf []byte
		buf, err = cv.mount.GetXattr("/", "ceph.quota.max_bytes")
		if err != nil {
			total = 0
		} else {
			total, _ = strconv.ParseUint(string(buf), 10, 64)
		}

		buf, err = cv.mount.GetXattr("/", "ceph.dir.rbytes")
		if err == nil {
			used, err = strconv.ParseUint(string(buf), 10, 64)
		}
	})

	return
}

func (fs *cephfs) CreateReference(ctx context.Context, path string, targetURI *url.URL) (err error) {
	user := fs.makeUser(ctx)

	user.op(func(cv cacheVal) {
		if !strings.HasPrefix(strings.TrimPrefix(path, cv.homePath), fs.conf.ShareFolder) {
			err = errors.New("cephfs: can't create reference outside a share folder")
		} else {
			err = cv.mount.MakeDir(path, fs.conf.DirMode)
		}
	})
	if err != nil {
		return
	}

	user.op(func(cv cacheVal) {
		err = cv.mount.SetXattr(path, xattrRef, []byte(targetURI.String()), 0)
	})

	return
}

func (fs *cephfs) Shutdown(ctx context.Context) (err error) {
	ctx.Done()
	fs.conn.clearCache()

	return
}

func (fs *cephfs) SetArbitraryMetadata(ctx context.Context, ref *provider.Reference, md *provider.ArbitraryMetadata) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return err
	}

	user.op(func(cv cacheVal) {
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

	return
}

func (fs *cephfs) UnsetArbitraryMetadata(ctx context.Context, ref *provider.Reference, keys []string) (err error) {
	var path string
	user := fs.makeUser(ctx)
	if path, err = user.resolveRef(ref); err != nil {
		return err
	}

	user.op(func(cv cacheVal) {
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

	return
}
