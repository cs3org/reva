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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	cephfs2 "github.com/ceph/go-ceph/cephfs"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/maxymania/go-system/posix_acl"
)

//TODO: assign the permissions to each of the 3 categories
var perms = map[rune][]int{
	'r': {3, 4, 5, 7, 8, 9, 10, 16, 18},
	'w': {1, 2, 6, 11, 13, 14, 15, 19},
	'x': {0, 12, 17},
}

var pIndex = make(map[int]rune)

func init() {
	for k, arr := range perms {
		for _, v := range arr {
			pIndex[v] = k
		}
	}
}

const (
	aclXattr = "system.posix_acl_access"
)

func getPermissionSet(user *User, stat *cephfs2.CephStatx, mount Mount, path string) (perm *provider.ResourcePermissions) {
	if int64(stat.Uid) == user.UidNumber || int64(stat.Gid) == user.GidNumber {
		updatePerms(perm, "rwx", false)
		return
	}

	acls := &posix_acl.Acl{}
	var xattr []byte
	var err error
	if xattr, err = mount.GetXattr(path, aclXattr); err != nil {
		return
	}
	acls.Decode(xattr)

	group, err := user.getGroupByID(fmt.Sprint(stat.Gid))

	for _, acl := range acls.List {
		rwx := strings.Split(acl.String(), ":")[2]
		switch acl.GetType() {
		case posix_acl.ACL_USER:
			if int64(acl.GetID()) == user.UidNumber {
				updatePerms(perm, rwx, false)
			}
		case posix_acl.ACL_GROUP:
			if int64(acl.GetID()) == user.GidNumber || in(group.GroupName, user.Groups) {
				updatePerms(perm, rwx, false)
			}
		case posix_acl.ACL_MASK:
			updatePerms(perm, rwx, true)
		case posix_acl.ACL_OTHERS:
			updatePerms(perm, rwx, false)
		}
	}

	return
}

func getFullPermissionSet(mount Mount, path string) (permList []*provider.Grant) {
	acls := &posix_acl.Acl{}
	var xattr []byte
	var err error
	if xattr, err = mount.GetXattr(path, aclXattr); err != nil {
		return nil
	}
	acls.Decode(xattr)

	permMap := make(map[uint32]*provider.Grant)
	for _, acl := range acls.List {
		rwx := strings.Split(acl.String(), ":")[2]
		switch acl.GetType() {
		case posix_acl.ACL_USER:
			permMap[acl.GetID()] = &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_USER,
					Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{Idp: strconv.Itoa(int(acl.GetID()))}},
				},
				Permissions: &provider.ResourcePermissions{},
			}
			updatePerms(permMap[acl.GetID()].Permissions, rwx, false)
		case posix_acl.ACL_GROUP:
			permMap[acl.GetID()] = &provider.Grant{
				Grantee: &provider.Grantee{
					Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
					Id:   &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{Idp: strconv.Itoa(int(acl.GetID()))}},
				},
				Permissions: &provider.ResourcePermissions{},
			}
			updatePerms(permMap[acl.GetID()].Permissions, rwx, false)
		}
	}

	for _, value := range permMap {
		permList = append(permList, value)
	}

	return
}

func permToInt(p *provider.ResourcePermissions) (result uint16) {
	item := reflect.ValueOf(p).Elem()
	fields := len(pIndex)
	rwx := uint16(4 | 2 | 1)
	for i := 0; i < fields; i++ {
		if item.Field(i).Bool() {
			switch pIndex[i] {
			case 'r':
				result |= 4
			case 'w':
				result |= 2
			case 'x':
				result |= 1
			}
		}

		if result == rwx {
			return
		}
	}

	return
}

const (
	updateGrant = iota
	removeGrant
)

func changePerms(mt Mount, grant *provider.Grant, path string, method int) (e error) {
	buf, e := mt.GetXattr(path, aclXattr)
	if e != nil {
		return
	}
	acls := &posix_acl.Acl{}
	acls.Decode(buf)
	var id uint64
	var sid posix_acl.AclSID

	switch grant.Grantee.Type {
	case provider.GranteeType_GRANTEE_TYPE_USER:
		id, e = strconv.ParseUint(grant.Grantee.GetUserId().OpaqueId, 10, 32)
		if e != nil {
			return e
		}
		sid.SetUid(uint32(id))
	case provider.GranteeType_GRANTEE_TYPE_GROUP:
		id, e = strconv.ParseUint(grant.Grantee.GetGroupId().OpaqueId, 10, 32)
		if e != nil {
			return e
		}
		sid.SetGid(uint32(id))
	default:
		return errors.New("cephfs: invalid grantee type")
	}

	var found = false
	var i int
	for i = range acls.List {
		if acls.List[i].AclSID == sid {
			found = true
		}
	}

	if method == updateGrant {
		if found {
			acls.List[i].Perm |= permToInt(grant.Permissions)
		} else {
			acls.List = append(acls.List, posix_acl.AclElement{
				AclSID: sid,
				Perm:   permToInt(grant.Permissions),
			})
		}
	} else {
		if found {
			acls.List[i].Perm &^= permToInt(grant.Permissions)
			if acls.List[i].Perm == 0 { // remove empty grant
				acls.List = append(acls.List[:i], acls.List[i+1:]...)
			}
		}
	}

	e = mt.SetXattr(path, aclXattr, acls.Encode(), 0)

	return
}

func updatePerms(rp *provider.ResourcePermissions, acl string, unset bool) {
	for _, t := range "rwx" {
		if strings.ContainsRune(acl, t) {
			for _, i := range perms[t] {
				reflect.ValueOf(rp).Elem().Field(i).SetBool(true)
			}
		} else if unset {
			for _, i := range perms[t] {
				reflect.ValueOf(rp).Elem().Field(i).SetBool(false)
			}
		}
	}
}
