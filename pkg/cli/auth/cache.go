// Copyright 2018-2026 CERN
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

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// A file that is not JSON is the raw token of an older reva login.
const cacheVersion = 2

// The layers of the cache, both keyed by the server. A reva token cannot be
// refreshed, but the upstream credential that it came from can.
const (
	Upstream = "upstream"
	Reva     = "reva"
)

// Cache stores tokens between invocations. It is a file readable only by the
// user.
type Cache struct {
	path string
	mu   sync.Mutex
}

type cacheFile struct {
	Version  int               `json:"version"`
	Upstream map[string]*Token `json:"upstream,omitempty"`
	Reva     map[string]*Token `json:"reva,omitempty"`
	legacy   string
}

func (f *cacheFile) layer(name string) map[string]*Token {
	if name == Reva {
		if f.Reva == nil {
			f.Reva = map[string]*Token{}
		}
		return f.Reva
	}
	if f.Upstream == nil {
		f.Upstream = map[string]*Token{}
	}
	return f.Upstream
}

// NewCache returns a cache backed by the file at path.
func NewCache(path string) *Cache {
	return &Cache{path: path}
}

// Get returns the token stored under key. A bad cache file reads as "no entry":
// signing in again is better than failing the command.
func (c *Cache) Get(layer, key string) (*Token, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.read()
	if err != nil {
		return nil, false
	}
	tok, ok := f.layer(layer)[key]
	if !ok || tok == nil {
		return nil, false
	}
	return tok, true
}

// Legacy returns the token of a file in the old format. It has no key, so it
// is for any server.
func (c *Cache) Legacy() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := c.read()
	if err != nil {
		return ""
	}
	return f.legacy
}

// Put stores a token under key and prunes the expired entries. It replaces a
// file in the old format.
func (c *Cache) Put(layer, key string, tok *Token) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.read()
	if err != nil {
		f = &cacheFile{}
	}
	f.layer(layer)[key] = tok
	pruneExpired(f.Upstream)
	pruneExpired(f.Reva)
	return c.write(f)
}

// Delete removes one entry.
func (c *Cache) Delete(layer, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.read()
	if err != nil {
		return nil
	}
	delete(f.layer(layer), key)
	return c.write(f)
}

// DeletePrefix removes every entry of layer whose key starts with prefix.
func (c *Cache) DeletePrefix(layer, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.read()
	if err != nil {
		return nil
	}
	m := f.layer(layer)
	for _, k := range slices.Collect(maps.Keys(m)) {
		if strings.HasPrefix(k, prefix) {
			delete(m, k)
		}
	}
	return c.write(f)
}

// Clear removes the cache file entirely.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := os.Remove(c.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Entries returns every cached token of layer.
func (c *Cache) Entries(layer string) map[string]*Token {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := c.read()
	if err != nil {
		return nil
	}
	return f.layer(layer)
}

func (c *Cache) read() (*cacheFile, error) {
	info, err := os.Stat(c.path)
	if err != nil {
		return nil, err
	}
	// Another user may have written the file: do not trust it.
	if !privateMode(info.Mode()) {
		return nil, fmt.Errorf("token cache %s has permissions %o, refusing to read it", c.path, info.Mode().Perm())
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] != '{' {
		return &cacheFile{legacy: string(data)}, nil
	}
	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Version != cacheVersion {
		return nil, fmt.Errorf("token cache %s has unsupported version %d", c.path, f.Version)
	}
	return &f, nil
}

// write replaces the file atomically, so a reader never sees a half-written one.
func (c *Cache) write(f *cacheFile) error {
	f.Version = cacheVersion
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(c.path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, c.path)
}

// pruneExpired drops the entries that can no longer be used. A token with a
// refresh token stays, because it can be renewed.
func pruneExpired(entries map[string]*Token) {
	now := time.Now()
	for k, t := range entries {
		if t == nil {
			delete(entries, k)
			continue
		}
		if t.RefreshToken != "" {
			continue
		}
		if !t.Expiry.IsZero() && now.After(t.Expiry) {
			delete(entries, k)
		}
	}
}
