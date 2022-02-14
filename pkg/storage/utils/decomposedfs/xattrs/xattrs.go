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

package xattrs

import (
	"strconv"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/gofrs/flock"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
)

// Declare a list of xattr keys
// TODO the below comment is currently copied from the owncloud driver, revisit
// Currently,extended file attributes have four separated
// namespaces (user, trusted, security and system) followed by a dot.
// A non root user can only manipulate the user. namespace, which is what
// we will use to store ownCloud specific metadata. To prevent name
// collisions with other apps We are going to introduce a sub namespace
// "user.ocis."
const (
	OcisPrefix    string = "user.ocis."
	ParentidAttr  string = OcisPrefix + "parentid"
	OwnerIDAttr   string = OcisPrefix + "owner.id"
	OwnerIDPAttr  string = OcisPrefix + "owner.idp"
	OwnerTypeAttr string = OcisPrefix + "owner.type"
	// the base name of the node
	// updated when the file is renamed or moved
	NameAttr string = OcisPrefix + "name"

	BlobIDAttr   string = OcisPrefix + "blobid"
	BlobsizeAttr string = OcisPrefix + "blobsize"

	// grantPrefix is the prefix for sharing related extended attributes
	GrantPrefix         string = OcisPrefix + "grant."
	GrantUserAcePrefix  string = OcisPrefix + "grant." + UserAcePrefix
	GrantGroupAcePrefix string = OcisPrefix + "grant." + GroupAcePrefix
	MetadataPrefix      string = OcisPrefix + "md."

	// favorite flag, per user
	FavPrefix string = OcisPrefix + "fav."

	// a temporary etag for a folder that is removed when the mtime propagation happens
	TmpEtagAttr     string = OcisPrefix + "tmp.etag"
	ReferenceAttr   string = OcisPrefix + "cs3.ref"      // arbitrary metadata
	ChecksumPrefix  string = OcisPrefix + "cs."          // followed by the algorithm, eg. ocis.cs.sha1
	TrashOriginAttr string = OcisPrefix + "trash.origin" // trash origin

	// we use a single attribute to enable or disable propagation of both: synctime and treesize
	// The propagation attribute is set to '1' at the top of the (sub)tree. Propagation will stop at
	// that node.
	PropagationAttr string = OcisPrefix + "propagation"

	// the tree modification time of the tree below this node,
	// propagated when synctime_accounting is true and
	// user.ocis.propagation=1 is set
	// stored as a readable time.RFC3339Nano
	TreeMTimeAttr string = OcisPrefix + "tmtime"

	// the size of the tree below this node,
	// propagated when treesize_accounting is true and
	// user.ocis.propagation=1 is set
	// stored as uint64, little endian
	TreesizeAttr string = OcisPrefix + "treesize"

	// the quota for the storage space / tree, regardless who accesses it
	QuotaAttr string = OcisPrefix + "quota"

	// the name given to a storage space. It should not contain any semantics as its only purpose is to be read.
	SpaceNameAttr        string = OcisPrefix + "space.name"
	SpaceDescriptionAttr string = OcisPrefix + "space.description"
	SpaceReadmeAttr      string = OcisPrefix + "space.readme"
	SpaceImageAttr       string = OcisPrefix + "space.image"

	UserAcePrefix  string = "u:"
	GroupAcePrefix string = "g:"
)

// ReferenceFromAttr returns a CS3 reference from xattr of a node.
// Supported formats are: "cs3:storageid/nodeid"
func ReferenceFromAttr(b []byte) (*provider.Reference, error) {
	return refFromCS3(b)
}

// refFromCS3 creates a CS3 reference from a set of bytes. This method should remain private
// and only be called after validation because it can potentially panic.
func refFromCS3(b []byte) (*provider.Reference, error) {
	parts := string(b[4:])
	return &provider.Reference{
		ResourceId: &provider.ResourceId{
			StorageId: strings.Split(parts, "/")[0],
			OpaqueId:  strings.Split(parts, "/")[1],
		},
	}, nil
}

// flockFile returns the flock filename for a given file name
// it returns an empty string if the input is empty
func flockFile(file string) string {
	var n string
	if len(file) > 0 {
		n = file + ".flock"
	}
	return n
}

// acquireWriteLog acquires a lock on a file or directory.
// if the parameter write is true, it gets an exclusive write lock, otherwise a shared read lock.
// The function returns a Flock object, unlocking has to be done in the calling function.
func acquireLock(file string, write bool) (*flock.Flock, error) {
	var err error

	// Create the a file to carry the log
	n := flockFile(file)
	if len(n) == 0 {
		return nil, errors.New("Path empty")
	}
	// Acquire the write log on the target node first.
	lock := flock.New(n)

	if write {
		_, err = lock.TryLock()
	} else {
		_, err = lock.TryRLock()
	}

	if err != nil {
		return nil, err
	}
	return lock, nil
}

// CopyMetadata copies all extended attributes from source to target.
// The optional filter function can be used to filter by attribute name, e.g. by checking a prefix
// For the source file, a shared lock is acquired. For the target, an exclusive
// write lock is acquired.
func CopyMetadata(src, target string, filter func(attributeName string) bool) error {
	var err error
	var writeLock, readLock *flock.Flock

	// Acquire the write log on the target node first.
	writeLock, err = acquireLock(target, true)

	if err != nil {
		return errors.Wrap(err, "xattrs: Unable to lock target to write")
	}
	defer func() {
		err = writeLock.Unlock()
	}()

	// now try to get a shared lock on the source
	readLock, err = acquireLock(src, false)

	if err != nil {
		return errors.Wrap(err, "xattrs: Unable to lock file for read")
	}
	defer func() {
		err = readLock.Unlock()
	}()

	// both locks are established. Copy.
	var attrNameList []string
	if attrNameList, err = xattr.List(src); err != nil {
		return errors.Wrap(err, "Can not get xattr listing on src")
	}

	// FIXME: this loop assumes that all reads and writes work. The case that one val
	// is copied, but any others not is not handled properly.
	for idx := range attrNameList {
		attrName := attrNameList[idx]
		if filter == nil || filter(attrName) {
			var attrVal []byte
			if attrVal, err = xattr.Get(src, attrName); err != nil {
				return errors.Wrapf(err, "Can not read src attribute named %s", attrName)
			}
			if err = xattr.Set(target, attrName, attrVal); err != nil {
				return errors.Wrapf(err, "Can not write target attribute named %s", attrName)
			}
		}
	}

	return nil
}

// Set an extended attribute key to the given value
// No file locking is involved here as writing a single xattr is
// considered to be atomic.
func Set(filePath string, key string, val string) error {

	if err := xattr.Set(filePath, key, []byte(val)); err != nil {
		return err
	}
	return nil
}

// SetMultiple allows setting multiple key value pairs at once
// the changes are protected with an file lock
// If the file lock can not be acquired the function returns a
// lock error.
func SetMultiple(filePath string, attribs map[string]string) error {

	// h, err := lockedfile.OpenFile(filePath, os.O_WRONLY, 0) // 0? Open File only workn for files ... but we want to lock dirs ... or symlinks
	// or we append .lock to the file and use https://github.com/gofrs/flock
	fileLock, err := acquireLock(filePath, true)

	if err != nil {
		return errors.Wrap(err, "xattrs: Can not acquired write log")
	}
	defer func() {
		err = fileLock.Unlock()
	}()

	for key, val := range attribs {
		if err := xattr.Set(filePath, key, []byte(val)); err != nil {
			return err
		}
	}
	return nil
}

// Get an extended attribute value for the given key
// No file locking is involved here as reading a single xattr is
// considered to be atomic.
func Get(filePath, key string) (string, error) {
	v, err := xattr.Get(filePath, key)
	if err != nil {
		return "", err
	}
	val := string(v)
	return val, nil
}

// GetInt64 reads a string as int64 from the xattrs
func GetInt64(filePath, key string) (int64, error) {
	attr, err := Get(filePath, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(attr, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// All reads all extended attributes for a node, protected by a
// shared file lock
func All(filePath string) (map[string]string, error) {
	fileLock, err := acquireLock(filePath, false)

	if err != nil {
		return nil, errors.Wrap(err, "xattrs: Unable to lock file for read")
	}
	defer func() {
		err = fileLock.Unlock()
	}()

	attrNames, err := xattr.List(filePath)
	if err != nil {
		return nil, err
	}

	attribs := make(map[string]string, len(attrNames))
	for _, name := range attrNames {
		val, err := xattr.Get(filePath, name)
		if err != nil {
			return nil, err
		}
		attribs[name] = string(val)
	}

	return attribs, nil
}
