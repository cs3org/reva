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

package backend

import (
	"context"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
	"github.com/go-redis/cache/v9"
	"github.com/pkg/xattr"
	"github.com/redis/go-redis/v9"
	"github.com/rogpeppe/go-internal/lockedfile"
	"gopkg.in/ini.v1"
)

// IniBackend persists the attributes in INI format inside the file
type IniBackend struct {
	metaCache *cache.Cache
}

type ReadWriteCloseSeekTruncater interface {
	io.ReadWriteCloser
	io.Seeker
	Truncate(int64) error
}

var encodedPrefixes = []string{prefixes.ChecksumPrefix, prefixes.MetadataPrefix, prefixes.GrantPrefix}

// NewIniBackend returns a new IniBackend instance
func NewIniBackend() IniBackend {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	c := cache.New(&cache.Options{
		Redis: rdb,
	})

	return IniBackend{
		metaCache: c,
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
	for k, _ := range attribs {
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
	return b.saveIni(path, attribs, acquireLock)
}

// Remove an extended attribute key
func (b IniBackend) Remove(path, key string) error {
	return b.saveIni(path, map[string]string{key: ""}, true)
}

func (b IniBackend) saveIni(path string, attribs map[string]string, acquireLock bool) error {
	var (
		f   ReadWriteCloseSeekTruncater
		err error
	)
	path = b.MetadataPath(path)
	if acquireLock {
		f, err = lockedfile.OpenFile(path, os.O_RDWR, 0600)
	} else {
		f, err = os.OpenFile(path, os.O_RDWR, 0600)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	// Read current state
	iniBytes, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	iniFile, err := ini.Load(iniBytes)
	if err != nil {
		return err
	}

	// Prepare new metadata
	iniAttribs := iniFile.Section("").KeysHash()
	for key, val := range attribs {
		if val == "" {
			delete(iniAttribs, key)
		} else {
			iniAttribs[key] = val
		}
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

	return b.metaCache.Set(&cache.Item{
		Key:   path,
		Value: iniAttribs,
		TTL:   24 * time.Hour,
	})
}

func (b IniBackend) loadMeta(path string) (map[string]string, error) {
	var attribs map[string]string
	err := b.metaCache.Get(context.Background(), path, &attribs)
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

	err = b.metaCache.Set(&cache.Item{
		Key:   path,
		Value: attribs,
		TTL:   24 * time.Hour,
	})
	if err != nil {
		return nil, err
	}

	return attribs, nil
}

// IsMetaFile returns whether the given path represents a meta file
func (IniBackend) IsMetaFile(path string) bool { return strings.HasSuffix(path, ".ini") }

// UsesExternalMetadataFile returns true when the backend uses external metadata files
func (IniBackend) UsesExternalMetadataFile() bool { return true }

// MetadataPath returns the path of the file holding the metadata for the given path
func (IniBackend) MetadataPath(path string) string { return path + ".ini" }
