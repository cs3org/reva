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
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cs3org/reva/v2/pkg/storage/cache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/options"
	"github.com/pkg/xattr"
	"github.com/rogpeppe/go-internal/lockedfile"
	"gopkg.in/ini.v1"
)

func init() {
	ini.PrettyFormat = false // Disable alignment of values for performance reasons
}

// IniBackend persists the attributes in INI format inside the file
type IniBackend struct {
	rootPath  string
	metaCache cache.FileMetadataCache
}

type readWriteCloseSeekTruncater interface {
	io.ReadWriteCloser
	io.Seeker
	Truncate(int64) error
}

// NewIniBackend returns a new IniBackend instance
func NewIniBackend(rootPath string, o options.CacheOptions) IniBackend {
	ini.PrettyFormat = false
	return IniBackend{
		rootPath:  filepath.Clean(rootPath),
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

	// Invalidate cache early
	_ = b.metaCache.RemoveMetadata(path)

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
	iniAttribs, err := decodeAttribs(iniFile)
	if err != nil {
		return err
	}
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
	for key, val := range encodeAttribs(iniAttribs) {
		ini.Section("").Key(key).SetValue(val)
	}
	_, err = ini.WriteTo(f)
	if err != nil {
		return err
	}

	return b.metaCache.PushToCache(b.cacheKey(path), iniAttribs)
}

func (b IniBackend) loadMeta(path string) (map[string]string, error) {
	var attribs map[string]string
	err := b.metaCache.PullFromCache(b.cacheKey(path), &attribs)
	if err == nil {
		return attribs, err
	}

	lockedFile, err := lockedfile.Open(path)

	// // No cached entry found. Read from storage and store in cache
	if err != nil {
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
	}
	defer lockedFile.Close()

	iniFile, err := ini.Load(lockedFile)
	if err != nil {
		return nil, err
	}

	attribs, err = decodeAttribs(iniFile)
	if err != nil {
		return nil, err
	}

	err = b.metaCache.PushToCache(b.cacheKey(path), attribs)
	if err != nil {
		return nil, err
	}

	return attribs, nil
}

// IsMetaFile returns whether the given path represents a meta file
func (IniBackend) IsMetaFile(path string) bool { return strings.HasSuffix(path, ".ini") }

// Purge purges the data of a given path
func (b IniBackend) Purge(path string) error {
	if err := b.metaCache.RemoveMetadata(b.cacheKey(path)); err != nil {
		return err
	}
	return os.Remove(b.MetadataPath(path))
}

// Rename moves the data for a given path to a new path
func (b IniBackend) Rename(oldPath, newPath string) error {
	data := map[string]string{}
	_ = b.metaCache.PullFromCache(b.cacheKey(oldPath), &data)
	err := b.metaCache.RemoveMetadata(b.cacheKey(oldPath))
	if err != nil {
		return err
	}
	err = b.metaCache.PushToCache(b.cacheKey(newPath), data)
	if err != nil {
		return err
	}

	return os.Rename(b.MetadataPath(oldPath), b.MetadataPath(newPath))
}

// MetadataPath returns the path of the file holding the metadata for the given path
func (IniBackend) MetadataPath(path string) string { return path + ".ini" }

func (b IniBackend) cacheKey(path string) string {
	// rootPath is guaranteed to have no trailing slash
	// the cache key shouldn't begin with a slash as some stores drop it which can cause
	// confusion
	return strings.TrimPrefix(path, b.rootPath+"/")
}

func needsEncoding(s []byte) bool {
	if len(s) == 0 {
		return false
	}

	if s[0] == '\'' || s[0] == '"' || s[0] == '`' {
		return true
	}

	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] >= unicode.MaxASCII { // ASCII 127 = Del - we don't want that
			return true
		}
	}
	return false
}

func encodeAttribs(attribs map[string]string) map[string]string {
	encAttribs := map[string]string{}
	for key, val := range attribs {
		if needsEncoding([]byte(val)) {
			encAttribs["base64:"+key] = base64.StdEncoding.EncodeToString([]byte(val))
		} else {
			encAttribs[key] = val
		}
	}
	return encAttribs
}

func decodeAttribs(iniFile *ini.File) (map[string]string, error) {
	decodedAttributes := map[string]string{}
	for key, val := range iniFile.Section("").KeysHash() {
		if strings.HasPrefix(key, "base64:") {
			key = strings.TrimPrefix(key, "base64:")
			valBytes, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return nil, err
			}
			val = string(valBytes)
		}
		decodedAttributes[key] = val
	}
	return decodedAttributes, nil
}
