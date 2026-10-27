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
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))
	tok := &Token{
		Kind: KindBearer, Value: "abc",
		Expiry:   time.Now().Add(time.Hour).Round(time.Second),
		Provider: "kerberos", Subject: "einstein",
	}

	if err := c.Put(Upstream, "key", tok); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get(Upstream, "key")
	if !ok {
		t.Fatal("Get returned nothing after Put")
	}
	if got.Value != tok.Value || got.Provider != tok.Provider || got.Subject != tok.Subject {
		t.Errorf("got %+v, want %+v", got, tok)
	}
	if !got.Expiry.Equal(tok.Expiry) {
		t.Errorf("expiry = %v, want %v", got.Expiry, tok.Expiry)
	}
}

// The file holds bearer tokens.
func TestCacheIsCreatedPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no permission bits")
	}
	path := filepath.Join(t.TempDir(), "cache")
	c := NewCache(path)
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("cache permissions = %o, want 600", perm)
	}
}

func TestCacheRefusesWorldReadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no permission bits")
	}
	path := filepath.Join(t.TempDir(), "cache")
	c := NewCache(path)
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.Get(Upstream, "key"); ok {
		t.Error("a cache readable by other users must not be trusted")
	}
}

func TestCacheIgnoresCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewCache(path)
	if _, ok := c.Get(Upstream, "key"); ok {
		t.Error("a corrupt cache should read as empty")
	}
	// A corrupt cache must heal itself.
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatalf("Put over a corrupt cache: %v", err)
	}
	if _, ok := c.Get(Upstream, "key"); !ok {
		t.Error("Put should have replaced the corrupt cache")
	}
}

func TestCacheIgnoresUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	data, _ := json.Marshal(map[string]any{"version": 99, "entries": map[string]any{
		"key": map[string]any{"value": "Bearer abc"},
	}})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, ok := NewCache(path).Get(Upstream, "key"); ok {
		t.Error("a cache written by a future version should be ignored, not misread")
	}
}

func TestCachePrunesExpiredEntries(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))

	if err := c.Put(Upstream, "stale", &Token{Value: "old", Expiry: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Upstream, "fresh", &Token{Value: "new", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.Get(Upstream, "stale"); ok {
		t.Error("an expired entry should have been pruned on the next write")
	}
	if _, ok := c.Get(Upstream, "fresh"); !ok {
		t.Error("a valid entry was pruned")
	}
}

func TestCacheKeepsExpiredTokenWithRefresh(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))

	if err := c.Put(Upstream, "refreshable", &Token{
		Value: "old", Expiry: time.Now().Add(-time.Hour), RefreshToken: "renew-me",
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Upstream, "other", &Token{Value: "x", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	got, ok := c.Get(Upstream, "refreshable")
	if !ok {
		t.Fatal("an expired token with a refresh token must be kept")
	}
	if got.RefreshToken != "renew-me" {
		t.Errorf("refresh token = %q", got.RefreshToken)
	}
}

func TestCacheDelete(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(Upstream, "key"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(Upstream, "key"); ok {
		t.Error("Delete did not remove the entry")
	}
}

func TestCacheDeletePrefix(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))
	for _, key := range []string{
		"https://a.test\x00einstein", "https://a.test\x00marie", "https://b.test\x00einstein",
	} {
		if err := c.Put(Upstream, key, &Token{Value: key}); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.DeletePrefix(Upstream, "https://a.test\x00"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(Upstream, "https://a.test\x00einstein"); ok {
		t.Error("DeletePrefix left an entry behind")
	}
	if _, ok := c.Get(Upstream, "https://a.test\x00marie"); ok {
		t.Error("DeletePrefix left an entry behind")
	}
	if _, ok := c.Get(Upstream, "https://b.test\x00einstein"); !ok {
		t.Error("DeletePrefix removed an entry for a different endpoint")
	}
}

func TestCacheClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	c := NewCache(path)
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Clear did not remove the file")
	}
	if err := c.Clear(); err != nil {
		t.Errorf("Clear on a missing file: %v", err)
	}
}

func TestCacheWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(filepath.Join(dir, "cache"))
	if err := c.Put(Upstream, "key", &Token{Value: "abc"}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

func TestCacheEntries(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "cache"))
	if err := c.Put(Upstream, "a", &Token{Value: "a", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(Upstream, "b", &Token{Value: "b", Expiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got := c.Entries(Upstream); len(got) != 2 {
		t.Errorf("Entries() returned %d entries, want 2", len(got))
	}
}
