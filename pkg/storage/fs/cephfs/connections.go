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
	"fmt"
	"time"

	"github.com/ceph/go-ceph/cephfs/admin"
	rados2 "github.com/ceph/go-ceph/rados"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"

	goceph "github.com/ceph/go-ceph/cephfs"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/semaphore"
)

type cacheVal struct {
	perm  *goceph.UserPerm
	mount *goceph.MountInfo
}

//TODO: Add to cephfs obj

type connections struct {
	cache      *ristretto.Cache
	lock       *semaphore.Weighted
	ctx        context.Context
	userCache  *ristretto.Cache
	groupCache *ristretto.Cache
}

// TODO: make configurable/add to options
var usrLimit int64 = 1e4

func newCache() (c *connections, err error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     usrLimit,
		BufferItems: 64,
		OnEvict: func(item *ristretto.Item) {
			if v, ok := item.Value.(*cacheVal); ok {
				v.perm.Destroy()
				_ = v.mount.Unmount()
				_ = v.mount.Release()
			}
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

type adminConn struct {
	// indexPoolName string
	subvolAdmin *admin.FSAdmin
	adminMount  Mount
	radosConn   *rados2.Conn
	// radosIO       *rados2.IOContext
}

func newAdminConn(conf *Options) (*adminConn, error) {
	rados, err := rados2.NewConnWithUser(conf.ClientID)
	if err != nil {
		return nil, errors.Wrap(err, "error creating connection with user for client id: "+conf.ClientID)
	}
	if err = rados.ReadConfigFile(conf.Config); err != nil {
		return nil, errors.Wrapf(err, "error reading config file %s", conf.Config)
	}

	if err = rados.SetConfigOption("keyring", conf.Keyring); err != nil {
		return nil, errors.Wrapf(err, "error setting keyring conf: %s", conf.Keyring)
	}

	if err = rados.Connect(); err != nil {
		return nil, errors.Wrap(err, "error connecting to rados")
	}

	// TODO: May use later for file ids
	/*
		pools, err := rados.ListPools()
		if err != nil {
			rados.Shutdown()
			return nil
		}

		var radosIO *rados2.IOContext
		poolName := conf.IndexPool
		if in(poolName, pools) {
			radosIO, err = rados.OpenIOContext(poolName)
			if err != nil {
				rados.Shutdown()
				return nil
			}
		} else {
			err = rados.MakePool(poolName)
			if err != nil {
				rados.Shutdown()
				return nil
			}
			radosIO, err = rados.OpenIOContext(poolName)
			if err != nil {
				rados.Shutdown()
				return nil
			}
		}
	*/

	mount, err := goceph.CreateFromRados(rados)
	if err != nil {
		rados.Shutdown()
		return nil, errors.Wrap(err, "error calling CreateFromRados")
	}

	if err = mount.MountWithRoot(conf.Root); err != nil {
		rados.Shutdown()
		destroyCephConn(mount, nil)
		return nil, errors.Wrapf(err, "error mounting with root %s", conf.Root)
	}

	return &adminConn{
		// poolName,
		admin.NewFromConn(rados),
		mount,
		rados,
		// radosIO,
	}, nil
}

func newConn(user *User) *cacheVal {
	var perm *goceph.UserPerm
	mount, err := goceph.CreateMountWithId(user.fs.conf.ClientID)
	if err != nil {
		return destroyCephConn(mount, perm)
	}
	if err = mount.ReadConfigFile(user.fs.conf.Config); err != nil {
		return destroyCephConn(mount, perm)
	}

	if err = mount.SetConfigOption("keyring", user.fs.conf.Keyring); err != nil {
		return destroyCephConn(mount, perm)
	}

	if err = mount.Init(); err != nil {
		return destroyCephConn(mount, perm)
	}

	if user != nil { //nil creates admin conn
		// TODO(lopresti) here we may need to impersonate a different user in order to support ACLs!
		perm = goceph.NewUserPerm(int(user.UidNumber), int(user.GidNumber), []int{})
		if err = mount.SetMountPerms(perm); err != nil {
			return destroyCephConn(mount, perm)
		}
	}

	if err = mount.MountWithRoot(user.fs.conf.Root); err != nil {
		return destroyCephConn(mount, perm)
	}

	// TODO(labkode): we leave the mount on the fs root
	/*
		if user != nil && !user.fs.conf.DisableHome {
			if err = mount.ChangeDir(user.fs.conf.Root); err != nil {
				return destroyCephConn(mount, perm)
			}
		}
	*/

	return &cacheVal{
		perm:  perm,
		mount: mount,
	}
}

func (fs *cephfs) getUserByID(ctx context.Context, uid string) (*userpb.User, error) {
	if entity, found := fs.conn.userCache.Get(uid); found {
		return entity.(*userpb.User), nil
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim: "uid",
		Value: uid,
	})

	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get user failed")
	}
	fs.conn.userCache.SetWithTTL(uid, getUserResp.User, 1, 24*time.Hour)
	fs.conn.userCache.SetWithTTL(getUserResp.User.Id.OpaqueId, getUserResp.User, 1, 24*time.Hour)

	return getUserResp.User, nil
}

func (fs *cephfs) getUserByOpaqueID(ctx context.Context, oid string) (*userpb.User, error) {
	if entity, found := fs.conn.userCache.Get(oid); found {
		return entity.(*userpb.User), nil
	}
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{
			OpaqueId: oid,
		},
	})

	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get user failed")
	}
	fs.conn.userCache.SetWithTTL(fmt.Sprint(getUserResp.User.UidNumber), getUserResp.User, 1, 24*time.Hour)
	fs.conn.userCache.SetWithTTL(oid, getUserResp.User, 1, 24*time.Hour)

	return getUserResp.User, nil
}

func (fs *cephfs) getGroupByID(ctx context.Context, gid string) (*grouppb.Group, error) {
	if entity, found := fs.conn.groupCache.Get(gid); found {
		return entity.(*grouppb.Group), nil
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getGroupResp, err := client.GetGroupByClaim(ctx, &grouppb.GetGroupByClaimRequest{
		Claim: "gid",
		Value: gid,
	})
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting group")
	}
	if getGroupResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get group failed")
	}
	fs.conn.groupCache.SetWithTTL(gid, getGroupResp.Group, 1, 24*time.Hour)
	fs.conn.groupCache.SetWithTTL(getGroupResp.Group.Id.OpaqueId, getGroupResp.Group, 1, 24*time.Hour)

	return getGroupResp.Group, nil
}

func (fs *cephfs) getGroupByOpaqueID(ctx context.Context, oid string) (*grouppb.Group, error) {
	if entity, found := fs.conn.groupCache.Get(oid); found {
		return entity.(*grouppb.Group), nil
	}
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getGroupResp, err := client.GetGroup(ctx, &grouppb.GetGroupRequest{
		GroupId: &grouppb.GroupId{
			OpaqueId: oid,
		},
	})

	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting group")
	}
	if getGroupResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get group failed")
	}
	fs.conn.userCache.SetWithTTL(fmt.Sprint(getGroupResp.Group.GidNumber), getGroupResp.Group, 1, 24*time.Hour)
	fs.conn.userCache.SetWithTTL(oid, getGroupResp.Group, 1, 24*time.Hour)

	return getGroupResp.Group, nil
}
