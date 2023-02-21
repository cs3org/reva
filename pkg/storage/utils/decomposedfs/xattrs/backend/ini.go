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
	"encoding/base64"
	"os"
	"strings"
	"time"

	"github.com/bluele/gcache"
	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/xattrs/prefixes"
	"github.com/pkg/xattr"
	"github.com/rogpeppe/go-internal/lockedfile"
	"gopkg.in/ini.v1"
)

// IniBackend persists the attributes in INI format inside the file
type IniBackend struct {
	metaCache gcache.Cache
}

type cacheEntry struct {
	mtime time.Time
	meta  *ini.File
}

var encodedPrefixes = []string{prefixes.ChecksumPrefix, prefixes.MetadataPrefix, prefixes.GrantPrefix}

// NewIniBackend returns a new IniBackend instance
func NewIniBackend() IniBackend {
	return IniBackend{
		metaCache: gcache.New(1_000_000).LFU().Build(),
	}
}

// All reads all extended attributes for a node
func (b IniBackend) All(path string) (map[string]string, error) {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return nil, err
	}
	attribs := ini.Section("").KeysHash()
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

	return attribs, nil
}

// Get an extended attribute value for the given key
func (b IniBackend) Get(path, key string) (string, error) {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return "", err
	}
	if !ini.Section("").HasKey(key) {
		return "", &xattr.Error{Op: "ini.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}

	val := ini.Section("").Key(key).Value()
	for _, prefix := range encodedPrefixes {
		if strings.HasPrefix(key, prefix) {
			valBytes, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return "", err
			}
			return string(valBytes), nil
		}
	}

	return val, nil
}

// GetInt64 reads a string as int64 from the xattrs
func (b IniBackend) GetInt64(path, key string) (int64, error) {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return 0, err
	}
	if !ini.Section("").HasKey(key) {
		return 0, &xattr.Error{Op: "ini.get", Path: path, Name: key, Err: xattr.ENOATTR}
	}
	return ini.Section("").Key(key).MustInt64(), nil
}

// List retrieves a list of names of extended attributes associated with the
// given path in the file system.
func (b IniBackend) List(path string) ([]string, error) {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return nil, err
	}
	return ini.Section("").KeyStrings(), nil
}

// Set sets one attribute for the given path
func (b IniBackend) Set(path, key, val string) error {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return err
	}

	for _, prefix := range encodedPrefixes {
		if strings.HasPrefix(key, prefix) {
			val = base64.StdEncoding.EncodeToString([]byte(val))
			break
		}
	}

	ini.Section("").Key(key).SetValue(val)

	return b.saveIni(path, ini)
}

// SetMultiple sets a set of attribute for the given path
func (b IniBackend) SetMultiple(path string, attribs map[string]string) error {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return err
	}

	for key, val := range attribs {
		for _, prefix := range encodedPrefixes {
			if strings.HasPrefix(key, prefix) {
				val = base64.StdEncoding.EncodeToString([]byte(val))
				break
			}
		}
		ini.Section("").Key(key).SetValue(val)
	}

	return b.saveIni(path, ini)
}

// Remove an extended attribute key
func (b IniBackend) Remove(path, key string) error {
	path = b.MetadataPath(path)

	ini, err := b.loadIni(path)
	if err != nil {
		return err
	}

	ini.Section("").DeleteKey(key)

	return b.saveIni(path, ini)
}

func (b IniBackend) saveIni(path string, ini *ini.File) error {
	lockedFile, err := lockedfile.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer lockedFile.Close()

	_, err = ini.WriteTo(lockedFile)
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	return b.metaCache.Set(path, cacheEntry{
		mtime: fi.ModTime(),
		meta:  ini,
	})
}

func (b IniBackend) loadIni(path string) (*ini.File, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if cachedIf, err := b.metaCache.Get(path); err == nil {
		cached, ok := cachedIf.(cacheEntry)
		if ok && cached.mtime == fi.ModTime() {
			return cached.meta, nil
		}
	}

	lockedFile, err := lockedfile.Open(path)
	if err != nil {
		return nil, err
	}
	defer lockedFile.Close()

	iniFile, err := ini.Load(lockedFile)
	if err != nil {
		return nil, err
	}

	err = b.metaCache.Set(path, cacheEntry{
		mtime: fi.ModTime(),
		meta:  iniFile,
	})
	if err != nil {
		return nil, err
	}

	return iniFile, nil
}

// IsMetaFile returns whether the given path represents a meta file
func (IniBackend) IsMetaFile(path string) bool { return strings.HasSuffix(path, ".ini") }

// UsesExternalMetadataFile returns true when the backend uses external metadata files
func (IniBackend) UsesExternalMetadataFile() bool { return true }

// MetadataPath returns the path of the file holding the metadata for the given path
func (IniBackend) MetadataPath(path string) string { return path + ".ini" }
