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

package metadata

import (
	"encoding/base64"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v2/pkg/storage/cache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/metadata/prefixes"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/pkg/xattr"
	"github.com/rogpeppe/go-internal/lockedfile"
	"gopkg.in/ini.v1"
)

// IniBackend persists the attributes in INI format inside the file
type IniBackend struct {
	metaCache cache.FileMetadataCache
}

type readWriteCloseSeekTruncater interface {
	io.ReadWriteCloser
	io.Seeker
	Truncate(int64) error
}

var encodedPrefixes = []string{prefixes.ChecksumPrefix, prefixes.MetadataPrefix, prefixes.GrantPrefix}

// NewIniBackend returns a new IniBackend instance
func NewIniBackend(o options.CacheOptions) IniBackend {
	return IniBackend{
		metaCache: cache.GetFileMetadataCache(o.CacheStore, o.CacheNodes, o.CacheDatabase, "filemetadata", 24*time.Hour),
	}
}

// All reads all extended attributes for a node
func (b IniBackend) All(path string) (map[string]string, error) {
	path = b.MetadataPath(path)

	return b.loadMeta(path)
}

// Get an extended attribute value for the given key
func (b IniBackend) Get(path, key string) (string, error) {
	path = b.MetadataPath(path)

	attribs, err := b.loadMeta(path)
	if err != nil {
		return "", err
	}
	val, ok := attribs[key]
	if !ok {
		return "", &xattr.Error{Op: "ini.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}
	return val, nil
}

// GetInt64 reads a string as int64 from the xattrs
func (b IniBackend) GetInt64(path, key string) (int64, error) {
	path = b.MetadataPath(path)

	attribs, err := b.loadMeta(path)
	if err != nil {
		return 0, err
	}
	val, ok := attribs[key]
	if !ok {
		return 0, &xattr.Error{Op: "ini.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (b IniBackend) List(path string) ([]string, error) {
	path = b.MetadataPath(path)

	attribs, err := b.loadMeta(path)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for k := range attribs {
		keys = append(keys, k)
	}
	return keys, nil
}

// Set sets one attribute for the given path
func (b IniBackend) Set(path, key, val string) error {
	return b.SetMultiple(path, map[string]string{key: val}, true)
}

// SetMultiple sets a set of attribute for the given path
func (b IniBackend) SetMultiple(path string, attribs map[string]string, acquireLock bool) error {
	return b.saveIni(path, attribs, nil, acquireLock)
}

// Remove an extended attribute key
func (b IniBackend) Remove(path, key string) error {
	return b.saveIni(path, nil, []string{key}, true)
}

func (b IniBackend) saveIni(path string, setAttribs map[string]string, deleteAttribs []string, acquireLock bool) error {
	var (
		f   readWriteCloseSeekTruncater
		err error
	)
	path = b.MetadataPath(path)
	if acquireLock {
		f, err = lockedfile.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	} else {
		f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	// Read current state
	iniBytes, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	iniFile, err := ini.Load(iniBytes)
	if err != nil {
		return err
	}

	// Prepare new metadata
	iniAttribs := iniFile.Section("").KeysHash()
	for key, val := range setAttribs {
		iniAttribs[key] = val
	}
	for _, key := range deleteAttribs {
		delete(iniAttribs, key)
	}

	// Truncate file
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	err = f.Truncate(0)
	if err != nil {
		return err
	}

	// Write new metadata to file
	ini, err := ini.Load([]byte{})
	if err != nil {
		return err
	}
	for key, val := range iniAttribs {
		for _, prefix := range encodedPrefixes {
			if strings.HasPrefix(key, prefix) {
				val = base64.StdEncoding.EncodeToString([]byte(val))
				break
			}
		}
		ini.Section("").Key(key).SetValue(val)
	}
	_, err = ini.WriteTo(f)
	if err != nil {
		return err
	}

	return b.metaCache.PushToCache(path, iniAttribs)
}

func (b IniBackend) loadMeta(path string) (map[string]string, error) {
	var attribs map[string]string
	err := b.metaCache.PullFromCache(path, &attribs)
	if err == nil {
		return attribs, err
	}

	var iniFile *ini.File
	f, err := os.ReadFile(path)
	length := len(f)

	// Try to read the file without getting a lock first. We will either
	// get the old or the new state or an empty byte array when the file
	// was just truncated by a writer.
	if err == nil && length > 0 {
		iniFile, err = ini.Load(f)
		if err != nil {
			return nil, err
		}
	} else {
		if os.IsNotExist(err) {
			// some of the caller rely on ENOTEXISTS to be returned when the
			// actual file (not the metafile) does not exist in order to
			// determine whether a node exists or not -> stat the actual node
			_, err := os.Stat(strings.TrimSuffix(path, ".ini"))
			if err != nil {
				return nil, err
			}
			return attribs, nil // no attributes set yet
		}

		// // No cached entry found. Read from storage and store in cache
		lockedFile, err := lockedfile.Open(path)
		if err != nil {
			return nil, err
		}
		defer lockedFile.Close()

		iniFile, err = ini.Load(lockedFile)
		if err != nil {
			return nil, err
		}
	}

	attribs = iniFile.Section("").KeysHash()
	for key, val := range attribs {
		for _, prefix := range encodedPrefixes {
			if strings.HasPrefix(key, prefix) {
				valBytes, err := base64.StdEncoding.DecodeString(val)
				if err != nil {
					return nil, err
				}
				attribs[key] = string(valBytes)
				break
			}
		}
	}

	err = b.metaCache.PushToCache(path, attribs)
	if err != nil {
		return nil, err
	}

	return attribs, nil
}

// IsMetaFile returns whether the given path represents a meta file
func (IniBackend) IsMetaFile(path string) bool { return strings.HasSuffix(path, ".ini") }

// Purge purges the data of a given path
func (b IniBackend) Purge(path string) error {
	if err := b.metaCache.Delete(path); err != nil {
		return err
	}
	return os.Remove(b.MetadataPath(path))
}

// Rename moves the data for a given path to a new path
func (b IniBackend) Rename(oldPath, newPath string) error {
	data := map[string]string{}
	_ = b.metaCache.PullFromCache(oldPath, &data)
	err := b.metaCache.Delete(oldPath)
	if err != nil {
		return err
	}
	err = b.metaCache.PushToCache(newPath, data)
	if err != nil {
		return err
	}

	return os.Rename(b.MetadataPath(oldPath), b.MetadataPath(newPath))
}

// MetadataPath returns the path of the file holding the metadata for the given path
func (IniBackend) MetadataPath(path string) string { return path + ".ini" }
