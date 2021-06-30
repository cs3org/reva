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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	cephfs2 "github.com/ceph/go-ceph/cephfs"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Mount type
type Mount = *cephfs2.MountInfo

// Statx type
type Statx = *cephfs2.CephStatx

var filePermDefault = uint32(0664)

func nodeType(stat Statx) os.FileMode {
	return os.FileMode(stat.Mode) & os.ModeType
}

/*
func firstEntry(dir *cephfs2.Directory) *cephfs2.DirEntry {
	for dirEntry, err := dir.ReadDir(); dirEntry != nil; dirEntry, err = dir.ReadDir() {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(dirEntry.Name(), ".") {
			continue
		}
		return dirEntry
	}

	return nil
}
*/

func closeDir(directory *cephfs2.Directory) {
	_ = directory.Close()
}

func closeFile(file *cephfs2.File) {
	_ = file.Close()
}

func finishConn(mt Mount) *cacheVal {
	_ = mt.Release()
	return nil
}

/*
func dirCmp(currPath string, curr *cephfs2.Directory, prev *cephfs2.Directory) (ri []*provider.RecycleItem, err error) {
	cEntry := firstEntry(curr)
	pEntry := firstEntry(prev)

	for true {
		if cEntry.Name() < pEntry.Name() {
			cEntry, err = curr.ReadDir(); if err != nil { return }
		} else if cEntry.Name() == pEntry.Name() {
			if cEntry.DType() == cephfs2.DTypeDir && cEntry
		}
	}

	return
}
*/

func calcFID(pathname string, stat Statx) string {
	hash := md5.New()
	return url.QueryEscape(hex.EncodeToString(hash.Sum([]byte(pathname)))) +
		strconv.FormatUint(uint64(stat.Inode), 10)
}

func calcChecksum(filepath string, mt Mount, stat Statx) (checksum string) {
	file, err := mt.Open(filepath, 'r', filePermDefault)
	defer closeFile(file)
	if err != nil {
		return
	}
	hash := md5.New()
	if _, err = io.Copy(hash, file); err != nil {
		return
	}
	checksum = hex.EncodeToString(hash.Sum(nil))
	_ = mt.SetXattr(filepath, xattrMd5ts, []byte(strconv.FormatInt(stat.Mtime.Sec, 10)), 0)
	_ = mt.SetXattr(filepath, xattrMd5, []byte(checksum), 0)

	return
}

func resolveRevRef(mt Mount, ref *provider.Reference, revKey string) (str string, err error) {
	if ref.GetResourceId() != nil {
		str, err = mt.Readlink(filepath.Join(snap, revKey, ref.ResourceId.OpaqueId))
		if err != nil {
			return "", fmt.Errorf("cephfs: invalid reference %+v", ref)
		}
	} else if str = ref.GetPath(); str != "" {
		buf, err := mt.GetXattr(str, xattrFid)
		if err != nil {
			return
		}
		str, err = mt.Readlink(filepath.Join(snap, revKey, string(buf)))
		if err != nil {
			return
		}
	} else {
		return "", fmt.Errorf("cephfs: empty reference %+v", ref)
	}

	return filepath.Join(snap, revKey, str), err
}
