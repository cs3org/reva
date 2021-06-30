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
	"strconv"

	cephfs2 "github.com/ceph/go-ceph/cephfs"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/semaphore"
)

type cacheVal struct {
	perm     *cephfs2.UserPerm
	mount    *cephfs2.MountInfo
	homeIno  string
	homePath string
}

//TODO: Add to cephfs obj

type connections struct {
	cache      *ristretto.Cache
	lock       *semaphore.Weighted
	ctx        context.Context
	userCache  *ristretto.Cache
	groupCache *ristretto.Cache
}

//TODO: make configurable/add to options
var usrLimit int64 = 1e4

func newCache() (c *connections, err error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     usrLimit,
		BufferItems: 64,
		OnEvict: func(key, conflict uint64, value interface{}, cost int64) {
			v := value.(cacheVal)
			v.perm.Destroy()
			_ = v.mount.Unmount()
			_ = v.mount.Release()
		},
	})
	if err != nil {
		return
	}

	ucache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     10 * usrLimit,
		BufferItems: 64,
	})
	if err != nil {
		return
	}

	gcache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     10 * usrLimit,
		BufferItems: 64,
	})
	if err != nil {
		return
	}

	c = &connections{
		cache:      cache,
		lock:       semaphore.NewWeighted(usrLimit),
		ctx:        context.Background(),
		userCache:  ucache,
		groupCache: gcache,
	}

	return
}

func (c *connections) clearCache() {
	c.cache.Clear()
	c.cache.Close()
}

func newConn(user *User) *cacheVal {
	var homePath = "/"
	var perm *cephfs2.UserPerm
	var stat *cephfs2.CephStatx
	mount, err := cephfs2.CreateMount()
	if err != nil {
		return finishConn(mount)
	}
	if err = mount.ReadDefaultConfigFile(); err != nil {
		return finishConn(mount)
	}
	if err = mount.Init(); err != nil {
		return finishConn(mount)
	}

	if user != nil {
		if homePath, err = user.fs.subvolAdmin.SubVolumePath("cephfs", "reva", user.Username); err != nil {
			return finishConn(mount)
		}
		perm = cephfs2.NewUserPerm(int(user.UidNumber), int(user.GidNumber), []int{})
		if err = mount.SetMountPerms(perm); err != nil {
			perm.Destroy()
			return finishConn(mount)
		}
	}
	if err = mount.MountWithRoot("/"); err != nil {
		return finishConn(mount)
	}
	stat, err = mount.Statx(homePath, cephfs2.StatxIno, 0)
	if err != nil {
		return finishConn(mount)
	}

	return &cacheVal{
		perm:     perm,
		mount:    mount,
		homeIno:  strconv.FormatUint(uint64(stat.Inode), 10),
		homePath: homePath,
	}
}
