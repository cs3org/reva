// Copyright 2018-2023 CERN
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

	"github.com/cs3org/reva/v2/pkg/storage/utils/filelocks"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"gopkg.in/ini.v1"
)

var backend Backend = NullBackend{}
var errUnconfiguredError = errors.New("no xattrs backend configured. Bailing out.")

// UseXattrsBackend configures decomposedfs to use xattrs for storing file attributes
func UseXattrsBackend() {
	backend = XattrsBackend{}
}

// UseIniBackend configures decomposedfs to use ini files for storing file attributes
func UseIniBackend() {
	backend = IniBackend{}
}

// Backend defines the interface for file attribute backends
type Backend interface {
	All(path string) (map[string]string, error)
	Get(path, key string) (string, error)
	GetInt64(path, key string) (int64, error)
	List(path string) (attribs []string, err error)
	Set(path, key, val string) error
	SetMultiple(path string, attribs map[string]string) error
	Remove(path, key string) error
}

// NullBackend is the default stub backend, used to enforce the configuration of a proper backend
type NullBackend struct{}

// All reads all extended attributes for a node
func (NullBackend) All(path string) (map[string]string, error) { return nil, errUnconfiguredError }

// Get an extended attribute value for the given key
func (NullBackend) Get(path, key string) (string, error) { return "", errUnconfiguredError }

// GetInt64 reads a string as int64 from the xattrs
func (NullBackend) GetInt64(path, key string) (int64, error) { return 0, errUnconfiguredError }

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (NullBackend) List(path string) ([]string, error) { return nil, errUnconfiguredError }

// Set sets one attribute for the given path
func (NullBackend) Set(path string, key string, val string) error { return errUnconfiguredError }

// SetMultiple sets a set of attribute for the given path
func (NullBackend) SetMultiple(path string, attribs map[string]string) error {
	return errUnconfiguredError
}

// Remove an extended attribute key
func (NullBackend) Remove(path string, key string) error { return errUnconfiguredError }

// XattrsBackend stores the file attributes in extended attributes
type XattrsBackend struct{}

// Get an extended attribute value for the given key
// No file locking is involved here as reading a single xattr is
// considered to be atomic.
func (b XattrsBackend) Get(filePath, key string) (string, error) {
	v, err := xattr.Get(filePath, key)
	if err != nil {
		return "", err
	}
	val := string(v)
	return val, nil
}

// GetInt64 reads a string as int64 from the xattrs
func (b XattrsBackend) GetInt64(filePath, key string) (int64, error) {
	attr, err := b.Get(filePath, key)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(attr, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (XattrsBackend) List(filePath string) (attribs []string, err error) {
	attrs, err := xattr.List(filePath)
	if err == nil {
		return attrs, nil
	}

	// listing the attributes failed. lock the file and try again
	readLock, err := filelocks.AcquireReadLock(filePath)

	if err != nil {
		return nil, errors.Wrap(err, "xattrs: Unable to lock file for read")
	}
	defer func() {
		rerr := filelocks.ReleaseLock(readLock)

		// if err is non nil we do not overwrite that
		if err == nil {
			err = rerr
		}
	}()

	return xattr.List(filePath)
}

// All reads all extended attributes for a node, protected by a
// shared file lock
func (b XattrsBackend) All(filePath string) (attribs map[string]string, err error) {
	attrNames, err := b.List(filePath)

	if err != nil {
		return nil, err
	}

	var (
		xerrs = 0
		xerr  error
	)
	// error handling: Count if there are errors while reading all attribs.
	// if there were any, return an error.
	attribs = make(map[string]string, len(attrNames))
	for _, name := range attrNames {
		var val []byte
		if val, xerr = xattr.Get(filePath, name); xerr != nil {
			xerrs++
		} else {
			attribs[name] = string(val)
		}
	}

	if xerrs > 0 {
		err = errors.Wrap(xerr, "Failed to read all xattrs")
	}

	return attribs, err
}

// Set sets one attribute for the given path
func (XattrsBackend) Set(path string, key string, val string) (err error) {
	return xattr.Set(path, key, []byte(val))
}

// SetMultiple sets a set of attribute for the given path
func (XattrsBackend) SetMultiple(path string, attribs map[string]string) (err error) {
	// error handling: Count if there are errors while setting the attribs.
	// if there were any, return an error.
	var (
		xerrs = 0
		xerr  error
	)
	for key, val := range attribs {
		if xerr = xattr.Set(path, key, []byte(val)); xerr != nil {
			// log
			xerrs++
		}
	}
	if xerrs > 0 {
		return errors.Wrap(xerr, "Failed to set all xattrs")
	}

	return nil
}

// Remove an extended attribute key
func (XattrsBackend) Remove(filePath string, key string) (err error) {
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

	return xattr.Remove(filePath, key)
}

// IniBackend persists the attributs in INI format inside the file
type IniBackend struct{}

// All reads all extended attributes for a node
func (IniBackend) All(path string) (map[string]string, error) {
	ini, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	return ini.Section("").KeysHash(), nil
}

// Get an extended attribute value for the given key
func (IniBackend) Get(path, key string) (string, error) {
	ini, err := ini.Load(path)
	if err != nil {
		return "", err
	}
	if !ini.Section("").HasKey(key) {
		return "", &xattr.Error{Op: "xattr.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}
	return ini.Section("").Key(key).Value(), nil
}

// GetInt64 reads a string as int64 from the xattrs
func (IniBackend) GetInt64(path, key string) (int64, error) {
	ini, err := ini.Load(path)
	if err != nil {
		return 0, err
	}
	if !ini.Section("").HasKey(key) {
		return 0, &xattr.Error{Op: "xattr.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}
	return ini.Section("").Key(key).MustInt64(), nil
}

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (IniBackend) List(path string) ([]string, error) {
	ini, err := ini.Load(path)
	if err != nil {
		return nil, err
	}
	return ini.Section("").KeyStrings(), nil
}

// Set sets one attribute for the given path
func (IniBackend) Set(path, key, val string) error {
	ini, err := ini.Load(path)
	if err != nil {
		return err
	}

	ini.Section("").Key(key).SetValue(val)

	return ini.SaveTo(path)
}

// SetMultiple sets a set of attribute for the given path
func (IniBackend) SetMultiple(path string, attribs map[string]string) error {
	ini, err := ini.Load(path)
	if err != nil {
		return err
	}

	for key, val := range attribs {
		ini.Section("").Key(key).SetValue(val)
	}

	return ini.SaveTo(path)
}

// Remove an extended attribute key
func (IniBackend) Remove(path, key string) error {
	ini, err := ini.Load(path)
	if err != nil {
		return err
	}

	ini.Section("").DeleteKey(key)

	return ini.SaveTo(path)
}
