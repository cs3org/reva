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
	"path/filepath"

	goceph "github.com/ceph/go-ceph/cephfs"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Mount type
type Mount = *goceph.MountInfo

// Statx type
type Statx = *goceph.CephStatx

var dirPermFull = uint32(0777)
var dirPermDefault = uint32(0700)
var filePermDefault = uint32(0640)

func closeDir(directory *goceph.Directory) {
	if directory != nil {
		_ = directory.Close()
	}
}

func closeFile(file *goceph.File) {
	if file != nil {
		_ = file.Close()
	}
}

func destroyCephConn(mt Mount, perm *goceph.UserPerm) *cacheVal {
	if perm != nil {
		perm.Destroy()
	}
	if mt != nil {
		_ = mt.Release()
	}
	return nil
}

func deleteFile(mount *goceph.MountInfo, path string) {
	_ = mount.Unlink(path)
}

func isDir(t provider.ResourceType) bool {
	return t == provider.ResourceType_RESOURCE_TYPE_CONTAINER
}

func removeLeadingSlash(path string) string {
	return filepath.Join(".", path)
}

func addLeadingSlash(path string) string {
	return filepath.Join("/", path)
}

func in(lookup string, list []string) bool {
	for _, item := range list {
		if item == lookup {
			return true
		}
	}
	return false
}

func pathGenerator(path string, reverse bool, str chan string) {
	if reverse {
		str <- path
		for i := range path {
			if path[len(path)-i-1] == filepath.Separator {
				str <- path[:len(path)-i-1]
			}
		}
	} else {
		for i := range path {
			if path[i] == filepath.Separator {
				str <- path[:i]
			}
		}
		str <- path
	}

	close(str)
}

func walkPath(path string, f func(string) error, reverse bool) (err error) {
	paths := make(chan string)
	// TODO(labkode): carefully review this, a race could happen if pathGenerator gorouting is slow
	go pathGenerator(path, reverse, paths)
	for path := range paths {
		if path == "" {
			continue
		}
		if err = f(path); err != nil && err.Error() != errFileExists && err.Error() != errNotFound {
			break
		} else {
			err = nil
		}
	}

	return
}
