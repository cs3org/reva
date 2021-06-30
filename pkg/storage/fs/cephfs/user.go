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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"

	cephfs2 "github.com/ceph/go-ceph/cephfs"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/user"
	"github.com/pkg/errors"
)

type callBack func(cb cacheVal)

// User custom type to add functionality to current struct
type User struct {
	*userv1beta1.User
	fs  *cephfs
	ctx *context.Context
}

func (fs *cephfs) makeUser(ctx context.Context) *User {
	user := user.ContextMustGetUser(ctx)

	return &User{user, fs, &ctx}
}

func (user *User) op(cb callBack) {
	conn := user.fs.conn
	if err := conn.lock.Acquire(conn.ctx, 1); err != nil {
		return
	}
	defer conn.lock.Release(1)

	val, found := conn.cache.Get(user)
	if !found {
		val = newConn(user)
		if val != nil {
			conn.cache.Set(user, val, 1)
		} else {
			return
		}
	}

	cb(val.(cacheVal))
}

func (user *User) fileAsResourceInfo(cv cacheVal, path string, stat *cephfs2.CephStatx, mdKeys []string) (ri *provider.ResourceInfo, err error) {
	var (
		_type  provider.ResourceType
		target string
		size   uint64
		buf    []byte
		isDir  = false
	)

	switch nodeType(stat) {
	case os.ModeDir:
		_type = provider.ResourceType_RESOURCE_TYPE_CONTAINER
		if buf, err = cv.mount.GetXattr(path, "ceph.dir.rbytes"); err == nil {
			size, err = strconv.ParseUint(string(buf), 10, 64)
		}
		isDir = true
	case os.ModeSymlink:
		_type = provider.ResourceType_RESOURCE_TYPE_SYMLINK
		target, err = cv.mount.Readlink(path)
	case 0:
		_type = provider.ResourceType_RESOURCE_TYPE_FILE
		size = stat.Size
	default:
		return nil, errors.New("cephfs: unknown entry type")
	}
	if err != nil {
		return
	}

	var xattrs []string
	keys := make(map[string]bool, len(mdKeys))
	for _, key := range mdKeys {
		keys[key] = true
	}
	if keys["*"] || len(keys) == 0 {
		mdKeys = []string{}
		keys = map[string]bool{}
	}
	mx := make(map[string]string)
	if xattrs, err = cv.mount.ListXattr(path); err == nil {
		for _, xattr := range xattrs {
			if len(mdKeys) == 0 || keys[xattr] {
				if buf, err := cv.mount.GetXattr(path, xattr); err == nil {
					mx[xattr] = string(buf)
				}
			}
		}
	}

	fid, ok := mx[xattrFid]
	if !ok {
		fid = calcFID(path, stat)
		if err = cv.mount.Symlink(filepath.Join(cv.homePath, path), fid); err != nil {
			return
		}

		if err = cv.mount.SetXattr(path, xattrFid, []byte(fid), 0); err != nil {
			return
		}
	}
	id := &provider.ResourceId{OpaqueId: fid}

	var etag string
	if isDir {
		rctime, _ := cv.mount.GetXattr(path, "ceph.dir.rctime")
		etag = fid + ":" + string(rctime)
	} else {
		etag = fid + ":" + strconv.FormatInt(stat.Ctime.Sec, 10)
	}

	mtime := &typesv1beta1.Timestamp{
		Seconds: uint64(stat.Mtime.Sec),
		Nanos:   uint32(stat.Mtime.Nsec),
	}

	perms := getPermissionSet(user, stat, cv.mount, path)

	for key := range mx {
		if !strings.HasPrefix(key, xattrUserNs) {
			delete(mx, key)
		}
	}

	var checksumType provider.ResourceChecksumType
	var md5 string
	if !isDir {
		md5tsBA, err := cv.mount.GetXattr(path, xattrMd5ts)
		if err == nil {
			md5ts, _ := strconv.ParseInt(string(md5tsBA), 10, 64)
			if stat.Mtime.Sec == md5ts {
				md5BA, err := cv.mount.GetXattr(path, xattrMd5)
				if err != nil {
					md5 = calcChecksum(path, cv.mount, stat)
				} else {
					md5 = string(md5BA)
				}
			} else {
				md5 = calcChecksum(path, cv.mount, stat)
			}
		} else {
			md5 = calcChecksum(path, cv.mount, stat)
		}

		checksumType = provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5
	} else {
		checksumType = provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET
	}
	checksum := &provider.ResourceChecksum{
		Sum:  md5,
		Type: checksumType,
	}

	var owner *userv1beta1.User
	if int64(stat.Uid) != user.UidNumber {
		owner, err = user.getUserByID(fmt.Sprint(stat.Uid))
	} else {
		owner = user.User
	}

	ri = &provider.ResourceInfo{
		Type:          _type,
		Id:            id,
		Checksum:      checksum,
		Etag:          etag,
		MimeType:      mime.Detect(isDir, path),
		Mtime:         mtime,
		Path:          path,
		PermissionSet: perms,
		Size:          size,
		//TODO: xattr idp opaqueid type if owner, if not, then query from ldap based on uid gid
		Owner:             owner.Id,
		Target:            target,
		ArbitraryMetadata: &provider.ArbitraryMetadata{Metadata: mx},
	}

	return
}

func (user *User) resolveRef(ref *provider.Reference) (str string, err error) {
	if str = ref.GetPath(); str == "" {
		if ref.GetResourceId() != nil {
			user.op(func(cv cacheVal) {
				str, err = cv.mount.Readlink(ref.ResourceId.OpaqueId)
			})
		} else {
			return "", fmt.Errorf("cephfs: invalid reference %+v", ref)
		}
	}

	return
}

func (user *User) getUserByID(uid string) (*userpb.User, error) {
	if entity, found := user.fs.conn.userCache.Get(uid); found {
		return entity.(*userpb.User), nil
	}

	client, err := pool.GetGatewayServiceClient(user.fs.conf.GatewaySvc)
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUser(*user.ctx, &userpb.GetUserRequest{
		UserId: &userpb.UserId{
			OpaqueId: uid,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get user failed")
	}
	user.fs.conn.userCache.SetWithTTL(uid, getUserResp.User, 1, 24*time.Hour)

	return getUserResp.User, nil
}

func (user *User) getGroupByID(uid string) (*grouppb.Group, error) {
	if entity, found := user.fs.conn.groupCache.Get(uid); found {
		return entity.(*grouppb.Group), nil
	}

	client, err := pool.GetGatewayServiceClient(user.fs.conf.GatewaySvc)
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting gateway grpc client")
	}
	getGroupResp, err := client.GetGroup(*user.ctx, &grouppb.GetGroupRequest{
		GroupId: &grouppb.GroupId{
			OpaqueId: uid,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "cephfs: error getting user")
	}
	if getGroupResp.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "cephfs: grpc get user failed")
	}
	user.fs.conn.groupCache.SetWithTTL(uid, getGroupResp.Group, 1, 24*time.Hour)

	return getGroupResp.Group, nil
}

func in(lookup string, list []string) bool {
	for _, item := range list {
		if item == lookup {
			return true
		}
	}
	return false
}
