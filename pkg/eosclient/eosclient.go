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

package eosclient

import (
	"context"
	"io"

	"github.com/cs3org/reva/pkg/storage/utils/acl"
)

// EOSClient is the interface which enables access to EOS instances through various interfaces.
type EOSClient interface {
	AddACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error
	RemoveACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error
	UpdateACL(ctx context.Context, uid, gid, rootUID, rootGID, path string, a *acl.Entry) error
	GetACL(ctx context.Context, uid, gid, path, aclType, target string) (*acl.Entry, error)
	ListACLs(ctx context.Context, uid, gid, path string) ([]*acl.Entry, error)
	GetFileInfoByInode(ctx context.Context, uid, gid string, inode uint64) (*FileInfo, error)
	GetFileInfoByFXID(ctx context.Context, uid, gid string, fxid string) (*FileInfo, error)
	GetFileInfoByPath(ctx context.Context, uid, gid, path string) (*FileInfo, error)
	SetAttr(ctx context.Context, uid, gid string, attr *Attribute, recursive bool, path string) error
	UnsetAttr(ctx context.Context, uid, gid string, attr *Attribute, path string) error
	GetQuota(ctx context.Context, username, rootUID, rootGID, path string) (*QuotaInfo, error)
	SetQuota(ctx context.Context, rootUID, rootGID string, info *SetQuotaInfo) error
	Touch(ctx context.Context, uid, gid, path string) error
	Chown(ctx context.Context, uid, gid, chownUID, chownGID, path string) error
	Chmod(ctx context.Context, uid, gid, mode, path string) error
	CreateDir(ctx context.Context, uid, gid, path string) error
	Remove(ctx context.Context, uid, gid, path string) error
	Rename(ctx context.Context, uid, gid, oldPath, newPath string) error
	List(ctx context.Context, uid, gid, path string) ([]*FileInfo, error)
	Read(ctx context.Context, uid, gid, path string) (io.ReadCloser, error)
	Write(ctx context.Context, uid, gid, path string, stream io.ReadCloser) error
	WriteFile(ctx context.Context, uid, gid, path, source string) error
	ListDeletedEntries(ctx context.Context, uid, gid string) ([]*DeletedEntry, error)
	RestoreDeletedEntry(ctx context.Context, uid, gid, key string) error
	PurgeDeletedEntries(ctx context.Context, uid, gid string) error
	ListVersions(ctx context.Context, uid, gid, p string) ([]*FileInfo, error)
	RollbackToVersion(ctx context.Context, uid, gid, path, version string) error
	ReadVersion(ctx context.Context, uid, gid, p, version string) (io.ReadCloser, error)
}

// AttrType is the type of extended attribute,
// either system (sys) or user (user).
type AttrType uint32

// Attribute represents an EOS extended attribute.
type Attribute struct {
	Type     AttrType
	Key, Val string
}

// FileInfo represents the metadata information returned by querying the EOS namespace.
type FileInfo struct {
	IsDir      bool
	MTimeNanos uint32
	Inode      uint64            `json:"inode"`
	FID        uint64            `json:"fid"`
	UID        uint64            `json:"uid"`
	GID        uint64            `json:"gid"`
	TreeSize   uint64            `json:"tree_size"`
	MTimeSec   uint64            `json:"mtime_sec"`
	Size       uint64            `json:"size"`
	TreeCount  uint64            `json:"tree_count"`
	File       string            `json:"eos_file"`
	ETag       string            `json:"etag"`
	Instance   string            `json:"instance"`
	SysACL     *acl.ACLs         `json:"sys_acl"`
	Attrs      map[string]string `json:"attrs"`
}

// DeletedEntry represents an entry from the trashbin.
type DeletedEntry struct {
	RestorePath   string
	RestoreKey    string
	Size          uint64
	DeletionMTime uint64
	IsDir         bool
}

// QuotaInfo reports the available bytes and inodes for a particular user.
// eos reports all quota values are unsigned long, see https://github.com/cern-eos/eos/blob/93515df8c0d5a858982853d960bec98f983c1285/mgm/Quota.hh#L135
type QuotaInfo struct {
	AvailableBytes, UsedBytes   uint64
	AvailableInodes, UsedInodes uint64
}

// SetQuotaInfo encapsulates the information needed to
// create a quota space in EOS for a user
type SetQuotaInfo struct {
	Username  string
	QuotaNode string
	MaxBytes  uint64
	MaxFiles  uint64
}
