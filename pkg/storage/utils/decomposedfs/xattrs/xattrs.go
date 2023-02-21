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
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/gofrs/flock"
	"github.com/pkg/errors"
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

// CopyMetadata copies all extended attributes from source to target.
// The optional filter function can be used to filter by attribute name, e.g. by checking a prefix
// For the source file, a shared lock is acquired.
// NOTE: target resource is not locked! You need to acquire a write lock on the target additionally
func CopyMetadata(src, target string, filter func(attributeName string) bool) (err error) {
	var readLock *flock.Flock

	// Acquire a read log on the source node
	readLock, err = filelocks.AcquireReadLock(src)

	if err != nil {
		return errors.Wrap(err, "xattrs: Unable to lock source to read")
	}
	defer func() {
		rerr := filelocks.ReleaseLock(readLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	return CopyMetadataWithSourceLock(src, target, filter, readLock)
}

// CopyMetadataWithSourceLock copies all extended attributes from source to target.
// The optional filter function can be used to filter by attribute name, e.g. by checking a prefix
// For the source file, a shared lock is acquired.
// NOTE: target resource is not locked! You need to acquire a write lock on the target additionally
func CopyMetadataWithSourceLock(src, target string, filter func(attributeName string) bool, readLock *flock.Flock) (err error) {
	switch {
	case readLock == nil:
		return errors.New("no lock provided")
	case readLock.Path() != filelocks.FlockFile(src):
		return errors.New("lockpath does not match filepath")
	case !readLock.Locked() && !readLock.RLocked(): // we need either a read or a write lock
		return errors.New("not locked")
	}

	// both locks are established. Copy.
	var attrNameList []string
	if attrNameList, err = backend.List(src); err != nil {
		return errors.Wrap(err, "Can not get xattr listing on src")
	}

	// error handling: We count errors of reads or writes of xattrs.
	// if there were any read or write errors an error is returned.
	var (
		xerrs = 0
		xerr  error
	)
	for idx := range attrNameList {
		attrName := attrNameList[idx]
		if filter == nil || filter(attrName) {
			var attrVal string
			if attrVal, xerr = backend.Get(src, attrName); xerr != nil {
				xerrs++
			}
			if xerr = backend.Set(target, attrName, attrVal); xerr != nil {
				xerrs++
			}
		}
	}
	if xerrs > 0 {
		err = errors.Wrap(xerr, "failed to copy all xattrs, last error returned")
	}

	return err
}

// Set an extended attribute key to the given value
func Set(filePath string, key string, val string) (err error) {
	fileLock, err := filelocks.AcquireWriteLock(filePath)
	if err != nil {
		return errors.Wrap(err, "xattrs: Can not acquire write log")
	}
	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	return SetWithLock(filePath, key, val, fileLock)
}

// SetWithLock an extended attribute key to the given value with an existing lock
func SetWithLock(filePath string, key string, val string, fileLock *flock.Flock) (err error) {
	// check the file is write locked
	switch {
	case fileLock == nil:
		return errors.New("no lock provided")
	case fileLock.Path() != filelocks.FlockFile(filePath):
		return errors.New("lockpath does not match filepath")
	case !fileLock.Locked():
		return errors.New("not write locked")
	}

	return backend.Set(filePath, key, val)
}

// Remove an extended attribute key
func Remove(filePath string, key string) (err error) {
	return backend.Remove(filePath, key)
}

// SetMultiple allows setting multiple key value pairs at once
// the changes are protected with an file lock
// If the file lock can not be acquired the function returns a
// lock error.
func SetMultiple(filePath string, attribs map[string]string) (err error) {
	var fileLock *flock.Flock
	fileLock, err = filelocks.AcquireWriteLock(filePath)

	if err != nil {
		return errors.Wrap(err, "xattrs: Can not acquire write log")
	}
	defer func() {
		rerr := filelocks.ReleaseLock(fileLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	return SetMultipleWithLock(filePath, attribs, fileLock)
}

// SetMultipleWithLock allows setting multiple key value pairs at once with an existing lock
func SetMultipleWithLock(filePath string, attribs map[string]string, fileLock *flock.Flock) (err error) {
	switch {
	case fileLock == nil:
		return errors.New("no lock provided")
	case fileLock.Path() != filelocks.FlockFile(filePath):
		return errors.New("lockpath does not match filepath")
	case !fileLock.Locked():
		return errors.New("not locked")
	}

	return backend.SetMultiple(filePath, attribs)
}

// All reads all extended attributes for a node
func All(path string) (map[string]string, error) {
	return backend.All(path)
}

// Get an extended attribute value for the given key
func Get(path, key string) (string, error) {
	return backend.Get(path, key)
}

// GetInt64 reads a string as int64 from the xattrs
func GetInt64(path, key string) (int64, error) {
	return backend.GetInt64(path, key)
}

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func List(path string) ([]string, error) {
	return backend.List(path)
}

// MetadataPath returns the path of the file holding the metadata for the given path
func MetadataPath(path string) string {
	return backend.MetadataPath(path)
}

// UsesExternalMetadataFile returns true when the backend uses external metadata files
func UsesExternalMetadataFile() bool {
	return backend.UsesExternalMetadataFile()
}

// IsMetaFile returns whether the given path represents a meta file
func IsMetaFile(path string) bool {
	return backend.IsMetaFile(path)
}
